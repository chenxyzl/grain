package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
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
	router          *ActorRef
	forceCloseChan  chan bool
	requestId       uint64
}

func NewSystem[P Provider](config *Config) *System {
	system := &System{}
	system.logger = slog.Default()
	system.config = config
	system.clusterProvider = newProvider[P]()
	system.registry = newRegistry(system)
	system.forceCloseChan = make(chan bool, 1)
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
	//create router
	x.router = x.Spawn(func() IActor {
		return newStreamRouter(NewActorRefLocal(x.clusterProvider.addr(), "stream_router"), x)
	})
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
	for {
		var left []*ActorRef
		times := 0
		x.registry.lookup.IterCb(func(key string, v iProcess) {
			if v.self().GetAddress() == x.clusterProvider.addr() && v.self().GetKind() != defaultReplyKindName {
				x.send(v.self(), messageDef.poison, x.getNextSnId())
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

func (x *System) NodesChanged() {

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

func (x *System) sendToLocal(envelope *Envelope) {
	proc := x.registry.get(envelope.GetTarget())
	if proc == nil {
		x.Logger().Error("get actor failed", "actor", envelope.GetTarget(), "msgName", envelope.MsgName)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(envelope.MsgName))
	if err != nil {
		x.Logger().Error("unregister msg type", "msgName", envelope.MsgName, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(envelope.Content, msg)
	if err != nil {
		x.Logger().Error("msg unmarshal err", "msgName", envelope.MsgName, "err", err)
		return
	}
	//build ctx
	proc.send(newContext(proc.self(), envelope.GetSender(), msg, envelope.GetMsgSnId(), context.Background()))
}

func (x *System) sendToRemote(envelope *Envelope) {
	proc := x.registry.get(x.router)
	if proc == nil {
		x.Logger().Error("get router failed", "router", x.router, "msgName", envelope.MsgName)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(envelope.MsgName))
	if err != nil {
		x.Logger().Error("unregister msg type", "msgName", envelope.MsgName, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(envelope.Content, msg)
	if err != nil {
		x.Logger().Error("msg unmarshal err", "msgName", envelope.MsgName, "err", err)
		return
	}
	//build ctx
	proc.send(newContext(proc.self(), envelope.GetSender(), msg, envelope.GetMsgSnId(), context.Background()))
}

func (x *System) getNextSnId() uint64 {
	return atomic.AddUint64(&x.requestId, 1)
}

func (x *System) send(target *ActorRef, msg proto.Message, msgSnId uint64, senders ...*ActorRef) {
	//check
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	var sender *ActorRef
	if len(senders) > 0 {
		sender = senders[0]
		if len(senders) > 1 {
			x.Logger().Warn("senders len bigger than 1, please check")
		}
	}
	//check send target
	if target.GetAddress() == x.clusterProvider.addr() {
		//to local
		proc := x.registry.get(target)
		if proc == nil {
			x.Logger().Error("get actor failed", "actor", target, "msgName", msg.ProtoReflect().Descriptor().FullName())
			return
		}
		proc.send(newContext(proc.self(), sender, msg, msgSnId, context.Background()))
		//x.sendToLocal(envelope)
	} else {
		//to remote
		//marshal
		content, err := proto.Marshal(msg)
		if err != nil {
			x.logger.Error("proto marshal err", "err", err, "msg", msg)
			return
		}
		envelope := &Envelope{
			Header:  nil,
			Sender:  sender,
			Target:  target,
			MsgSnId: msgSnId,
			MsgName: string(msg.ProtoReflect().Descriptor().FullName()),
			Content: content,
		}
		x.sendToRemote(envelope)
	}
}

func request[T proto.Message](system *System, target *ActorRef, req proto.Message, msgSnId uint64) (T, error) {
	//
	reply := newProcessorReplay[T](system, system.GetConfig().requestTimeout)
	//
	system.send(target, req, msgSnId, reply.self())
	//
	return reply.Result()
}

func (x *System) Poison(ref *ActorRef) {
	x.send(ref, messageDef.poison, x.getNextSnId())
}

// NoEntrySend
// msg to target
// warning don't change msg value when send, may data race
func NoEntrySend(system *System, target *ActorRef, msg proto.Message) {
	system.send(target, msg, system.getNextSnId())
}

// NoEntryRequestE sync request mean's not allowed re-entry
// wanted system.NoEntryRequestE[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func NoEntryRequestE[T proto.Message](system *System, target *ActorRef, req proto.Message) (T, error) {
	return request[T](system, target, req, system.getNextSnId())
}
