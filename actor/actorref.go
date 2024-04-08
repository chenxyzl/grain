package actor

type ActorRef interface{ id() string }
type actorRefImpl Address

// NewActorRef wrapper Address, for data safe
func NewActorRef(address *Address) ActorRef {
	var x = (*actorRefImpl)(address)
	return x
}

func (x *actorRefImpl) id() string { return x.Id }

func (x *actorRefImpl) String() string { return x.Id }
