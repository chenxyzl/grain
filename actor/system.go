package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/internal"
	"github.com/chenxyzl/grain/actor/uuid"
	"google.golang.org/protobuf/proto"
	"hash"
	"hash/fnv"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
)

type System struct {
	config *Config
	//
	logger          *slog.Logger
	registry        *registry
	clusterProvider Provider
	forceCloseChan  chan bool
	timerSchedule   *timerSchedule
	//hash
	hasher     hash.Hash32
	hasherLock sync.Mutex
	//
	eventStream *ActorRef
	//
	requestId uint64
}

func NewSystem[P Provider](config *Config) *System {
	system := &System{}
	system.config = config
	//
	system.logger = slog.Default()
	system.registry = newRegistry(system)
	system.clusterProvider = newProvider[P]()
	system.forceCloseChan = make(chan bool, 1)
	system.timerSchedule = newTimerSchedule(system.sendWithoutSender)
	//
	system.hasher = fnv.New32a()
	//
	return system
}

func (x *System) GetConfig() *Config {
	return x.config
}

func (x *System) getRegistry() *registry {
	return x.registry
}

func (x *System) Logger() *slog.Logger {
	return x.logger
}

func (x *System) Spawn(p Producer, opts ...OptFunc) *ActorRef {
	return x.SpawnNamed(p, strconv.Itoa(int(uuid.Generate())), opts...)
}

func (x *System) SpawnNamed(p Producer, name string, opts ...OptFunc) *ActorRef {
	//
	opts = append(opts, withSelf(x.clusterProvider.addr(), name))
	options := NewOpts(p, opts...)
	//
	return newProcessor(x, options).self()
}

func (x *System) GetRemoteActorRef(kind string, name string) *ActorRef {
	addr := ""
	if kind == defaultLocalKind {
		addr = x.clusterProvider.addr()
	} else {
		addr = x.calcAddressByKind7Id(x.clusterProvider.getNodes(), kind, name)
	}
	//
	if addr == "" {
		return nil
	}
	//
	return newActorRefWithKind(addr, kind, name)
}
func (x *System) GetLocalActorRef(kind string, name string) *ActorRef {
	return newActorRefWithKind(x.clusterProvider.addr(), kind, name)
}

func (x *System) getNextSnId() uint64 {
	return atomic.AddUint64(&x.requestId, 1)
}

func (x *System) sendWithoutSender(target *ActorRef, msg proto.Message, msgSnId ...uint64) {
	x.sendWithSender(target, msg, nil, msgSnId...)
}

func (x *System) sendWithSender(target *ActorRef, msg proto.Message, sender *ActorRef, msgSnIds ...uint64) {
	//check
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	//messageId
	var msgSnId uint64
	if len(msgSnIds) > 0 {
		msgSnId = msgSnIds[0]
	} else {
		msgSnId = x.getNextSnId()
	}

	//check send target
	if target.GetAddress() == x.clusterProvider.addr() {
		//to local
		proc := x.registry.get(target)
		if proc == nil {
			if _, ok := msg.(*internal.Poison); ok {
				//ignore poison msg if proc not found
				return
			} else {
				//ensure remote kind actor
				x.ensureRemoteKindActorExist(target)
				proc = x.registry.get(target)
			}
		}
		if proc == nil {
			x.Logger().Error("send, get actor failed", "actor", target, "msgName", msg.ProtoReflect().Descriptor().FullName())
			return
		}
		proc.send(newContext(proc.self(), sender, msg, msgSnId, context.Background(), x))
		//x.sendToLocal(envelope)
	} else {
		targetAddress := target.GetAddress()
		remoteActorRef := newActorRefWithKind(x.clusterProvider.addr(), defaultSystemKind, writeStreamNamePrefix+targetAddress)
		proc := x.registry.get(remoteActorRef)
		if proc == nil {
			x.SpawnNamed(func() IActor {
				return newStreamWriterActor(remoteActorRef, targetAddress, x.GetConfig().dialOptions, x.GetConfig().callOptions)
			}, remoteActorRef.GetName(), withKindName(remoteActorRef.GetKind()))
		}
		//to remote
		proc = x.registry.get(remoteActorRef)
		if proc == nil {
			x.Logger().Error("get remote failed", "remote", remoteActorRef, "msgName", msg.ProtoReflect().Descriptor().FullName())
			return
		}
		proc.send(newContext(target, sender, msg, msgSnId, context.Background(), x))
	}
}

func request[T proto.Message](system *System, target *ActorRef, req proto.Message, msgSnId uint64) (T, error) {
	//
	reply := newProcessorReplay[T](system, system.GetConfig().requestTimeout)
	//
	system.sendWithSender(target, req, reply.self(), msgSnId)
	//
	return reply.Result()
}

func (x *System) Poison(ref *ActorRef) {
	x.sendWithoutSender(ref, messageDef.poison)
}
