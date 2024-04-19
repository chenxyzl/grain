package actor

type ProviderListener interface {
	ClusterErr()
	InitGlobalUuid(nodeId uint64)
	NodesChanged()
}

type Provider interface {
	//
	SelfAddr() string
	//life
	Start(system *System, state NodeState, config *Config, listener ProviderListener) error
	Stop() error
	//node
	GetNodesByKind(kind string) []NodeState
	//actor
	RegisterActor(state ActorState) error
	UnregisterActor(state ActorState)
}
