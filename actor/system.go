package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/helper"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"log/slog"
	"strconv"
)

type System struct {
	config *Config
	//
	logger          *slog.Logger
	registry        *Registry
	clusterProvider Provider
	router          *ActorRef
}

func NewSystem[P Provider](config *Config) *System {
	system := &System{}
	system.logger = slog.With()
	system.config = config
	system.clusterProvider = helper.New[P]()
	system.registry = newRegistry(system)
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

func (x *System) ClusterErr() {
	if x == nil {
		return
	}
	x.Logger().Error("cluster provider error.")
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
	if err := x.clusterProvider.Stop(); err != nil {
		x.Logger().Error("cluster provider stop err.", "err", err)
	}
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
	//
	if err := proc.start(); err != nil {
		x.Logger().Info("spawn actor error.", "actor", proc.self(), "err", err)
		x.registry.remove(proc.self())
		panic(err)
	}
	return proc.self()
}

func (x *System) sendToLocal(request *Envelope) {
	id := request.GetTarget().GetIdentifier()
	proc := x.registry.getByID(id)
	if proc == nil {
		x.Logger().Error("get actor by id fail", "id", id, "msg_type", request.MsgType)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(request.MsgType))
	if err != nil {
		x.Logger().Error("unregister msg type", "msg_type", request.MsgType, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(request.Content, msg)
	if err != nil {
		x.Logger().Error("msg unmarshal err", "msg_type", request.MsgType, "err", err)
		return
	}
	//build ctx
	ctx := newContext(proc.self(), request.GetSender(), msg, context.Background())
	proc.send(ctx)
}

func (x *System) sendToRemote(request *Envelope) {
	routerId := x.router.GetIdentifier()
	proc := x.registry.getByID(routerId)
	if proc == nil {
		x.Logger().Error("get actor by id fail", "id", routerId, "msg_type", request.MsgType)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(request.MsgType))
	if err != nil {
		x.Logger().Error("unregister msg type", "msg_type", request.MsgType, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(request.Content, msg)
	if err != nil {
		x.Logger().Error("msg unmarshal err", "msg_type", request.MsgType, "err", err)
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
	//marshal
	content, err := proto.Marshal(msg)
	if err != nil {
		x.logger.Error("proto marshal err", "err", err, "msg", msg)
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
	envelope := &Envelope{
		Header:    nil,
		Sender:    sender,
		Target:    target,
		RequestId: 0,
		MsgType:   string(msg.ProtoReflect().Descriptor().FullName()),
		Content:   content,
	}

	//send to local
	if target.GetAddress() == x.clusterProvider.SelfAddr() {
		x.sendToLocal(envelope)
	} else {
		x.sendToRemote(envelope)
	}
}

func (x *System) Poison(ref *ActorRef) {
	x.Send(ref, Message.poison)
}

// Request
// wanted system.Request[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func Request[T proto.Message](system *System, target *ActorRef, req proto.Message) {
	reply := newProcessorReplay[T](system, system.GetConfig().requestTimeout)
	system.registry.add(reply)
}

// Send api like Request
func Send(system *System, target *ActorRef, msg proto.Message) {
	system.Send(target, msg)
}
