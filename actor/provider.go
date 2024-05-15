package actor

import "reflect"

type ProviderListener interface {
	ClusterErr()
	InitGlobalUuid(nodeId uint64)
	NodesChanged()
}

type Provider interface {
	//
	addr() string
	//life
	start(system *System, config *Config) error
	stop()

	//event
	getNodes() []NodeState

	//actor
	ensureRemoteKindActorExist(ref *ActorRef)
	//
	getAddressByKind7Id(kind string, name string) string
}

func newProvider[T Provider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
