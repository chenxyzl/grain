package actor

type process interface {
	Self() ActorRef
}
