package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/def"
	"github.com/chenxyzl/grain/actor/provider"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/fun"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"log/slog"
	"strconv"
)

type System struct {
	config *def.Config
	//
	logger          *slog.Logger
	registry        *Registry
	clusterProvider provider.Provider
	router          *ActorRef
}

func NewSystem[Provider provider.Provider](config *def.Config) *System {
	system := &System{}
	system.logger = slog.With()
	system.config = config
	system.clusterProvider = fun.Zero[Provider]()
	system.registry = newRegistry(system)
	return system
}

func (x *System) Start() error {
	//register to cluster
	if err := x.clusterProvider.Start(x, def.NodeState{}, x.config, x); err != nil {
		return err
	}
	//overwrite logger
	x.logger = slog.With("system", x.clusterProvider.SelfAddr())
	//create router
	x.router = x.Spawn(func(name *ActorRef) IProcess {
		return newStreamRouter(name, x)
	})
	return nil
}

func (x *System) ClusterErr() {
	slog.Error("cluster provider error.")
	x.Stop()
}

func (x *System) InitGlobalUuid(nodeId uint64) {
	//update uuid node
	if err := uuid.Init(nodeId); err != nil {
		panic(err)
	}
	slog.Warn("uuid init success.", "nodeId", nodeId)
}

func (x *System) NodesChanged() {

}

func (x *System) Stop() {
	if err := x.clusterProvider.Stop(); err != nil {
		x.Logger().Error("cluster provider stop err.", "err", err)
	}
}

func (x *System) GetConfig() *def.Config {
	return x.config
}

func (x *System) GetRegistry() *Registry {
	return x.registry
}

func (x *System) Logger() *slog.Logger {
	return x.logger
}

func (x *System) Spawn(p func(name *ActorRef) IProcess) *ActorRef {
	actorRef := NewActorRef(x.clusterProvider.SelfAddr(), strconv.Itoa(int(uuid.Generate())))
	return x.registry.add(p(actorRef)).Self()
}

func (x *System) SpawnNamed(p func(name *ActorRef) IProcess, name string) *ActorRef {
	actorRef := NewActorRef(x.clusterProvider.SelfAddr(), name)
	return x.registry.add(p(actorRef)).Self()
}

func (x *System) sendToLocal(request *Request) {
	id := request.GetTarget().GetId()
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
	ctx := newContext(proc, request.GetSender(), msg, context.Background())
	proc.receive(ctx)
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
	//send to local
	if target.GetAddress() == x.clusterProvider.SelfAddr() {
		x.sendToLocal(&Request{
			Header:  nil,
			Sender:  sender,
			Target:  target,
			MsgId:   0,
			MsgType: string(msg.ProtoReflect().Descriptor().FullName()),
			Content: content,
		})
		return
	}
	//send to remote

}
