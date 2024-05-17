package actor

import (
	"reflect"
)

type Provider interface {
	//
	addr() string
	//life
	start(system *System, config *Config) error
	stop()

	//nodes
	getNodes() []NodeState
	//actor
	ensureRemoteKindActorExist(ref *ActorRef)
	getAddressByKind7Id(kind string, name string) string

	//
	registerRemoteActorKind(ref *ActorRef) bool
	unRegisterRemoteActorKind(ref *ActorRef) bool
}

func newProvider[T Provider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
