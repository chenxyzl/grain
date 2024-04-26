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
	proc.send(newContext(proc.self(), request.GetSender(), msg, context.Background()))
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
	ctx := newContext(proc.self(), request.GetSender(), msg, context.Background())
	proc.send(ctx)
}

func (x *System) Send(target *ActorRef, msg proto.Message, senders ...*ActorRef) {
	//check
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	//get sender
	var sender *ActorRef
	l := len(senders)
	if l > 0 {
		sender = senders[0]
		if l > 1 {
			x.Logger().Warn("sender must length 0 or 1", "senders", senders)
		}
	}
	//check send target
	if target.GetAddress() == x.clusterProvider.SelfAddr() {
		//to local
		proc := x.registry.get(target)
		if proc == nil {
			x.Logger().Error("get actor failed", "actor", target, "msgName", msg.ProtoReflect().Descriptor().FullName())
			return
		}
		proc.send(newContext(proc.self(), sender, msg, context.Background()))
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
			RequestId: 0,
			MsgName:   string(msg.ProtoReflect().Descriptor().FullName()),
			Content:   content,
		}
		x.sendToRemote(envelope)
	}
}

func (x *System) Poison(ref *ActorRef) {
	x.Send(ref, Message.poison)
}

// Request
// wanted system.Request[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func Request[T proto.Message](system *System, target *ActorRef, req proto.Message) T {
	reply := newProcessorReplay[T](system, system.GetConfig().requestTimeout)
	system.registry.add(reply)
	system.Send(target, req, reply.self())
	ret, err := reply.Result()
	if err != nil {
		system.Logger().Error("request result err", "target", target, "reply", reply.self(), "err", err)
		return helper.Zero[T]()
	}
	return ret
}
