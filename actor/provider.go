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

	//actor
	ensureLocalActorExist(ref *ActorRef)
}

func newProvider[T Provider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
