package grain

import (
	"hash"
	"hash/fnv"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
)

type system struct {
	config *config
	//
	registry        *registry
	rpcService      iRpcServer
	clusterProvider iProvider
	timerSchedule   *timerSchedule
	//hash
	hasher     hash.Hash32
	hasherLock sync.Mutex
	//
	forceCloseChan chan bool
	logger         *slog.Logger
	eventStream    ActorRef
	askId          uint64
}

// NewSystem ...
// @param clusterName mean etcd root
// @param clusterUrls mean etcd urls
func NewSystem(clusterName string, version string, clusterUrls []string, opts ...ConfigOptFunc) ISystem {
	sys := &system{}
	sys.config = newConfig(clusterName, version, clusterUrls, opts...)
	//
	sys.logger = slog.Default()
	sys.registry = newRegistry(sys.Logger())
	sys.clusterProvider = newProvider[*providerEtcd]()
	sys.forceCloseChan = make(chan bool, 1)
	sys.timerSchedule = newTimerSchedule(sys)
	//
	sys.hasher = fnv.New32a()
	//
	return sys
}

func (x *system) getAddr() string        { return x.rpcService.Addr() }
func (x *system) getConfig() *config     { return x.config }
func (x *system) GetProvider() iProvider { return x.clusterProvider }
func (x *system) getRegistry() iRegistry {
	return x.registry
}
func (x *system) GetNextAskId() uint64     { return atomic.AddUint64(&x.askId, 1) }
func (x *system) GetScheduler() iScheduler { return x }

func (x *system) Logger() *slog.Logger {
	return x.logger
}

func (x *system) Spawn(p iProducer, opts ...KindOptFunc) ActorRef {
	return x.SpawnNamed(p, strconv.Itoa(int(uuid.Generate())), opts...)
}

func (x *system) SpawnNamed(p iProducer, name string, opts ...KindOptFunc) ActorRef {
	//
	opts = append(opts, withOptsDirectSelf(name, x.getAddr(), x))
	options := newOpts(p, opts...)
	//
	return newProcessor(x, options).self()
}

func (x *system) SpawnClusterName(p iProducer, opts ...KindOptFunc) ActorRef {
	//
	options := newOpts(p, opts...)
	//
	return newProcessor(x, options).self()
}

func (x *system) GetClusterActorRef(kind string, name string) ActorRef {
	////
	//nodes, _ := x.clusterProvider.getNodes()
	//addr := x.calcAddressByKind8Id(nodes, kind, name)
	////
	//if addr == "" {
	//	return nil
	//}
	//
	return newClusterActorRef(kind, name, x)
}

func (x *system) getSender() iSender {
	return x
}

func (x *system) tellWithSender(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	//check
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	//check actor type
	if target.isDirect() {
		targetAddr := target.GetDirectAddr()
		//direct actor
		if targetAddr == x.getAddr() {
			x.sendToLocal(target, msg, sender, msgSnId)
		} else {
			x.sendToCluster(targetAddr, target, msg, sender, msgSnId)
		}
	} else {
		//for performance op
		if proc := x.registry.get(target); proc != nil {
			proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
		} else {
			//cluster actor
			//need calc each time?
			nodes, version := x.clusterProvider.getNodes()
			cacheAddr, cacheVersion := target.getRemoteAddrCache()
			if cacheVersion != version {
				cacheAddr = x.calcAddressByKind8Id(nodes, target.GetKind(), target.GetName())
				if cacheAddr != "" {
					target.setRemoteAddrCache(cacheAddr, version)
				}
			}
			if cacheAddr == "" {
				x.Logger().Error("actor kind not in cluster")
				return
			}
			//
			if cacheAddr == x.getAddr() {
				//ensure cluster kind actor must exist
				x.ensureClusterKindActorExist(target)
				//
				x.sendToLocal(target, msg, sender, msgSnId)
			} else {
				x.sendToCluster(cacheAddr, target, msg, sender, msgSnId)
			}
		}
	}
}

func (x *system) tell(target ActorRef, msg proto.Message) {
	x.tellWithSender(target, msg, nil, x.GetNextAskId())
}

func (x *system) sendToLocal(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	//to local
	proc := x.registry.get(target)
	if proc == nil {
		//
		if _, ok := msg.(*message.Poison); ok {
			//ignore poison msg if proc not found
			return
		}
		x.Logger().Error("send, get actor failed", "actor", target, "msgName", msg.ProtoReflect().Descriptor().FullName())
		return
	}
	//
	proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
}

func (x *system) sendToCluster(targetAddress string, target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	//remote addr
	writeStreamActorRef := newDirectActorRef(defaultWriteStreamKind, targetAddress, x.getAddr(), x)
	//get proc
	proc := x.registry.get(writeStreamActorRef)
	if proc == nil {
		// spawn if not found
		x.SpawnNamed(func() IActor {
			return newStreamWriterActor(writeStreamActorRef, targetAddress, x.getConfig().dialOptions, x.getConfig().callOptions)
		}, writeStreamActorRef.GetName(), WithOptsKindName(writeStreamActorRef.GetKind()))
	}
	//must
	proc = x.registry.get(writeStreamActorRef)
	proc.send(newContext(target, sender, msg, msgSnId, x))
}

func (x *system) Poison(ref ActorRef) {
	x.tell(ref, poison)
}
