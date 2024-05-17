package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/internal"
	"github.com/chenxyzl/grain/actor/uuid"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

type System struct {
	config *Config
	//
	logger          *slog.Logger
	registry        *Registry
	clusterProvider Provider
	forceCloseChan  chan bool
	timerSchedule   *timerSchedule
	requestId       uint64
	eventStream     *ActorRef
}

func NewSystem[P Provider](config *Config) *System {
	system := &System{}
	system.logger = slog.Default()
	system.config = config
	system.clusterProvider = newProvider[P]()
	system.registry = newRegistry(system)
	system.forceCloseChan = make(chan bool, 1)
	system.timerSchedule = newTimerSchedule(system.sendWithoutSender)
	return system
}

func (x *System) Start() error {
	//lock config
	x.config.markRunning()
	//register to cluster
	if err := x.clusterProvider.start(x, x.config); err != nil {
		return err
	}
	//overwrite logger
	x.logger = slog.With("system", x.clusterProvider.addr(), "node", x.config.state.NodeId)
	return nil
}
func (x *System) WaitStopSignal() {
	// signal.Notify的ch信道是阻塞的(signal.Notify不会阻塞发送信号), 需要设置缓冲
	signals := make(chan os.Signal, 1)
	// It is not possible to block SIGKILL or syscall.SIGSTOP
	signal.Notify(signals, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	select {
	case sig := <-signals:
		x.Logger().Warn("system will exit by signal", "signal", sig.String())
	case <-x.forceCloseChan:
		x.Logger().Warn("system will exit by forceCloseChan")
	}
	x.stopActors()
	x.clusterProvider.stop()
}

func (x *System) ForceStop() {
	x.forceCloseChan <- true
}

func (x *System) stopActors() {
	x.Logger().Info("begin stop all actors")
	for {
		var left []*ActorRef
		times := 0
		x.registry.lookup.IterCb(func(key string, v iProcess) {
			if v.self().GetAddress() == x.clusterProvider.addr() &&
				v.self().GetKind() != defaultReplyKind {
				x.sendWithoutSender(v.self(), messageDef.poison)
				left = append(left, v.self())
			}
		})
		time.Sleep(time.Second)
		times++
		if len(left) == 0 {
			x.Logger().Warn("waiting actors stop success", "times", times)
			break
		} else if times >= defaultStopWaitTimeSecond {
			x.Logger().Warn("waiting stop timeout", "left", len(left), "times", times, "actors", left)
			break
		} else {
			x.Logger().Info("waiting actors stop ..., ", "count", len(left), "times", times)
		}
	}
}

func (x *System) ClusterErr() {
	if x == nil {
		return
	}
	x.Logger().Error("cluster provider error. will stop system")
	x.ForceStop()
}

func (x *System) InitGlobalUuid(nodeId uint64) {
	//update uuid node
	if err := uuid.Init(nodeId); err != nil {
		panic(err)
	}
	x.requestId = uuid.GetBeginRequestId()
	x.Logger().Warn("uuid init success", "nodeId", nodeId)
}

func (x *System) GetConfig() *Config {
	return x.config
}

func (x *System) GetRegistry() *Registry {
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
	addr := x.clusterProvider.getAddressByKind7Id(kind, name)
	if addr == "" {
		return nil
	}
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
				x.clusterProvider.ensureRemoteKindActorExist(target)
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
		address := target.GetAddress()
		remoteActorRef := newActorRefWithKind(x.clusterProvider.addr(), writeStreamKind, address)
		proc := x.registry.get(remoteActorRef)
		if proc == nil {
			x.SpawnNamed(func() IActor {
				return newStreamWriterActor(remoteActorRef, address, x.GetConfig().dialOptions, x.GetConfig().callOptions)
			}, address, WithKindName(writeStreamKind))
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
