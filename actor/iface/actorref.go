package iface

import "github.com/chenxyzl/grain/actor"

type ActorRef interface{ id() string }
type actorRefImpl actor.Address

// NewActorRef wrapper Address, for data safe
func NewActorRef(address *actor.Address) ActorRef {
	var x = (*actorRefImpl)(address)
	return x
}

func (x *actorRefImpl) id() string { return x.Id }

func (x *actorRefImpl) String() string { return x.Id }
