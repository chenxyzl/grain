package provider

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/actor/def"
)

type ProviderListener interface {
	ClusterErr()
	InitGlobalUuid(nodeId uint64)
	NodesChanged()
}

type Provider interface {
	//
	SelfAddr() string
	//life
	Start(system *actor.System, state def.NodeState, config *def.Config, listener ProviderListener) error
	Stop() error
	//node
	GetNodesByKind(kind string) []def.NodeState
	//actor
	RegisterActor(state def.ActorState) error
	UnregisterActor(state def.ActorState)
}
