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
)

type System struct {
	config          *def.Config
	registry        *Registry
	clusterProvider provider.Provider
	rpcService      *RPCService
}

func NewSystem[Provider provider.Provider](config *def.Config) *System {
	system := &System{}
	system.config = config
	system.clusterProvider = fun.Zero[Provider]()
	system.registry = newRegistry(system)
	system.rpcService = &RPCService{}
	return system
}

func (x *System) Start() error {
	//start grpc
	if err := x.rpcService.start(); err != nil {
		return err
	}
	//register to cluster
	if err := x.clusterProvider.Start(def.NodeState{}, x, x.config); err != nil {
		return err
	}
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
		slog.Error("cluster provider stop err.", "err", err)
	}
}

func (x *System) GetConfig() *def.Config {
	return x.config
}

func (x *System) GetRegistry() *Registry {
	return x.registry
}

func (x *System) SendToLocal(request *Request) {
	id := request.GetTarget().GetId()
	proc := x.registry.getByID(id)
	if proc == nil {
		slog.Error("get actor by id fail", "id", id, "msg_type", request.MsgType)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(request.MsgType))
	if err != nil {
		proc.Logger().Error("unregister msg type", "msg_type", request.MsgType, "err", err)
		return
	}
	msg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(request.Content, msg)
	if err != nil {
		proc.Logger().Error("msg unmarshal err", "msg_type", request.MsgType, "err", err)
		return
	}
	//build ctx
	ctx := newContext(proc, request.GetSender(), msg, context.Background())
	proc.receive(ctx)
}
