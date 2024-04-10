package provider

import (
	"github.com/chenxyzl/grain/actor/def"
)

type ProviderListener interface {
	ClusterErr()
	InitGlobalUuid(nodeId uint64)
	NodesChanged()
}

type Provider interface {
	//life
	Start(state def.NodeState, listener ProviderListener, config *def.Config) error
	Stop() error
	//node
	GetNodesByKind(kind string) []def.NodeState
	//actor
	RegisterActor(state def.ActorState) error
	UnregisterActor(state def.ActorState)
}
