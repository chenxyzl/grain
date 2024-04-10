package actor

import (
	"github.com/chenxyzl/grain/actor/def"
	"github.com/chenxyzl/grain/actor/provider"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/fun"
	"log/slog"
)

type System struct {
	config          *def.Config
	registry        *Registry
	clusterProvider provider.Provider
}

func NewSystem[Provider provider.Provider](config *def.Config) *System {
	system := &System{}
	system.config = config
	system.clusterProvider = fun.Zero[Provider]()
	system.registry = newRegistry(system)
	return system
}

func (x *System) Start() error {
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
	slog.Warn("uuid init success.", slog.Any("nodeId", nodeId))
}

func (x *System) NodesChanged() {

}

func (x *System) Stop() {
	if err := x.clusterProvider.Stop(); err != nil {
		slog.Error("cluster provider stop err.", slog.Any("err", err))
	}
}

func (x *System) GetConfig() *def.Config {
	return x.config
}

func (x *System) GetRegistry() *Registry {
	return x.registry
}
