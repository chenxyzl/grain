package actor

import "reflect"

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

func newProvider[T Provider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
