package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/helper"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
)

type System struct {
	config *Config
	//
	logger          *slog.Logger
	registry        *Registry
	clusterProvider Provider
	router          *ActorRef
	closeChan       chan bool
	requestId       uint64
}

func NewSystem[P Provider](config *Config) *System {
	system := &System{}
	system.logger = slog.Default()
	system.config = config
	system.clusterProvider = newProvider[P]()
	system.registry = newRegistry(system)
	system.closeChan = make(chan bool, 1)
	return system
}

func (x *System) Start() error {
	//register to cluster
	if err := x.clusterProvider.Start(x, x.config); err != nil {
		return err
	}
	//overwrite logger
	x.logger = slog.With("system", x.clusterProvider.SelfAddr(), "node", x.config.state.NodeId)
	//create router
	x.router = x.Spawn(func() IActor {
		return newStreamRouter(NewActorRef(x.clusterProvider.SelfAddr(), "stream_router"), x)
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
	case <-x.closeChan:
		x.Logger().Warn("system will exit by closeChan")
	}
	//todo stop somethings
}

func (x *System) ClusterErr() {
	if x == nil {
		return
	}
	x.Logger().Error("cluster provider error. will stop system")
	x.Stop()
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

func (x *System) Stop() {
	x.closeChan <- true
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
	opts = append(opts, withSelf(x.clusterProvider.SelfAddr(), name))
	options := NewOpts(p, opts...)
	//
	proc := newProcessor(x, options)
	//
	x.registry.add(proc)
	//1
	if err := proc.start(); err != nil {
		x.Logger().Info("spawn actor error.", "actor", proc.self(), "err", err)
		x.registry.remove(proc.self())
		panic(err)
	}
	return proc.self()
}

func (x *System) sendToLocal(request *Envelope) {
	proc := x.registry.get(request.GetTarget())
	if proc == nil {
		x.Logger().Error("get actor failed", "actor", request.GetTarget(), "msgName", request.MsgName)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(request.MsgName))
	if err != nil {
		x.Logger().Error("unregister msg type", "msgName", request.MsgName, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(request.Content, msg)
	if err != nil {
		x.Logger().Error("msg unmarshal err", "msgName", request.MsgName, "err", err)
		return
	}
	//build ctx
	proc.send(newContext(proc.self(), request.GetSender(), msg, request.GetRequestId(), context.Background()))
}

func (x *System) sendToRemote(request *Envelope) {
	proc := x.registry.get(x.router)
	if proc == nil {
		x.Logger().Error("get router failed", "router", x.router, "msgName", request.MsgName)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(request.MsgName))
	if err != nil {
		x.Logger().Error("unregister msg type", "msgName", request.MsgName, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(request.Content, msg)
	if err != nil {
		x.Logger().Error("msg unmarshal err", "msgName", request.MsgName, "err", err)
		return
	}
	//build ctx
	ctx := newContext(proc.self(), request.GetSender(), msg, request.GetRequestId(), context.Background())
	proc.send(ctx)
}

func (x *System) getNextRequestId() uint64 {
	return atomic.AddUint64(&x.requestId, 1)
}

func (x *System) sendWithSender(target *ActorRef, msg proto.Message, requestId uint64, sender *ActorRef) {
	//check
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	//check send target
	if target.GetAddress() == x.clusterProvider.SelfAddr() {
		//to local
		proc := x.registry.get(target)
		if proc == nil {
			x.Logger().Error("get actor failed", "actor", target, "msgName", msg.ProtoReflect().Descriptor().FullName())
			return
		}
		proc.send(newContext(proc.self(), sender, msg, requestId, context.Background()))
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
			Header:    nil,
			Sender:    sender,
			Target:    target,
			RequestId: requestId,
			MsgName:   string(msg.ProtoReflect().Descriptor().FullName()),
			Content:   content,
		}
		x.sendToRemote(envelope)
	}
}

func (x *System) Poison(ref *ActorRef) {
	Send(x, ref, messageDef.poison)
}

// Send
// msg to target
// warning don't change msg value when send, may data race
func Send(system *System, target *ActorRef, msg proto.Message) {
	system.sendWithSender(target, msg, 0, nil)
}

// SyncRequestE sync request mean's not allowed re-entry
// wanted system.SyncRequestE[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func SyncRequestE[T proto.Message](system *System, target *ActorRef, req proto.Message) (T, error) {
	reply := newProcessorReplay[T](system, system.GetConfig().requestTimeout)
	system.registry.add(reply)
	system.sendWithSender(target, req, system.getNextRequestId(), reply.self())
	ret, err := reply.Result()
	if err != nil {
		system.Logger().Error("request result err", "target", target, "reply", reply.self(), "err", err)
		return helper.Zero[T](), err
	}
	return ret, nil
}
