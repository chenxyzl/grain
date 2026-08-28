package grain

import (
	"log/slog"
	"strconv"
	"sync/atomic"

	"github.com/chenxyzl/grain/al/safemap"
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
	addrHash        *AddrHash

	//
	forceCloseChan chan bool
	logger         *slog.Logger
	eventStream    ActorRef
	askId          uint64
	// addr caches rpcService.Addr() after Start(); it is immutable once the grpc
	// server is listening, so the per-Send getAddr() avoids an interface dispatch.
	addr string
	// pending holds in-flight Ask correlation channels keyed by msg snId. A reply
	// (local or routed back from a remote node) is delivered by snId, so a reply
	// no longer needs to be a registered actor.
	pending safemap.ConcurrentMap[uint64, chan proto.Message]
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
	sys.clusterProvider = &providerEtcd{}
	sys.forceCloseChan = make(chan bool, 1)
	sys.timerSchedule = newTimerSchedule(sys)
	sys.addrHash = newAddrHash()
	sys.pending = safemap.NewIntC[uint64, chan proto.Message]()
	//
	return sys
}
func (x *system) getAddr() string          { return x.addr }
func (x *system) getConfig() *config       { return x.config }
func (x *system) GetProvider() iProvider   { return x.clusterProvider }
func (x *system) getRegistry() iRegistry   { return x.registry }
func (x *system) nextSnId() uint64         { return atomic.AddUint64(&x.askId, 1) }
func (x *system) GetScheduler() iScheduler { return x }
func (x *system) getAddrHash() *AddrHash   { return x.addrHash }

func (x *system) Logger() *slog.Logger { return x.logger }

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

func (x *system) GetClusterActorRef(kind string, name string) ActorRef {
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
			cacheAddr, _ := target.getRemoteAddrCache()
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
	x.tellWithSender(target, msg, nil, x.nextSnId())
}

func (x *system) sendToLocal(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	// reply target: deliver to the waiting Ask via the pending table (by snId),
	// not through the registry — a reply is a correlation-id future, not an actor.
	if target.isAsk() {
		x.deliverReply(target.askSnId(), msg)
		return
	}
	//to local
	proc := x.registry.get(target)
	if proc == nil {
		//
		if _, ok := msg.(*message.Poison); ok {
			//ignore poison msg if proc not found
			return
		}
		if sender != nil && sender.isAsk() {
			sender.Tell(errActorNotFound)
		}
		x.Logger().Error("send, get actor failed", "actor", target, "msgName", proto.MessageName(msg))
		return
	}
	//
	proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
}

func (x *system) sendToCluster(targetAddress string, target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	//remote addr
	writeStreamActorRef := newDirectActorRef(defaultWriteStreamKind, targetAddress, x.getAddr(), x)
	//get proc, or spawn idempotently (multiple senders may race here; a
	//duplicate-id panic would crash the process, so use the get-or-create path).
	proc := x.registry.get(writeStreamActorRef)
	if proc == nil {
		opts := newOpts(func() IActor {
			return newStreamWriterActor(writeStreamActorRef, targetAddress, x.getConfig().dialOptions, x.getConfig().callOptions)
		}, WithOptsKindName(writeStreamActorRef.GetKind()), WithOptsPoisonFirstOnQuit(false), withOptsDirectSelf(writeStreamActorRef.GetName(), x.getAddr(), x))
		proc = newProcessorOrGet(x, opts)
	}
	proc.send(newContext(target, sender, msg, msgSnId, x))
}

func (x *system) Poison(ref ActorRef) {
	// local actor: use the non-blocking control signal (never blocks on a full
	// mailbox, safe under registry locks). remote actor: send poison as a normal
	// message over the network.
	if proc := x.registry.get(ref); proc != nil {
		proc.poison()
		return
	}
	x.tell(ref, poison)
}

// registerAsk allocates a correlation channel for an in-flight Ask and stores it
// under snId. The channel is buffered (cap 1) so deliverReply never blocks.
func (x *system) registerAsk(snId uint64) chan proto.Message {
	ch := make(chan proto.Message, 1)
	x.pending.Set(snId, ch)
	return ch
}

// deliverReply routes a reply to the waiting Ask by snId. It atomically removes
// the entry (Pop), so at most one of {reply, timeout, shutdown} ever writes the
// channel; a late reply whose Ask already finished finds nothing and is dropped.
func (x *system) deliverReply(snId uint64, msg proto.Message) {
	if ch, ok := x.pending.Pop(snId); ok {
		ch <- msg // cap-1 buffered, single writer wins the Pop, never blocks
	}
}

// cancelAsk removes the pending entry (Ask finished or timed out). Idempotent:
// if a reply already Popped it, this is a no-op.
func (x *system) cancelAsk(snId uint64) {
	x.pending.Remove(snId)
}

