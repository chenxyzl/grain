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
	Start(system *System, config *Config) error
	Stop() error
	//node
	GetNodesByKind(kind string) []NodeState
	//actor
	RegisterActor(state ActorState) error
	UnregisterActor(state ActorState)
}
