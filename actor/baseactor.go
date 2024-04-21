package actor

var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	IActor
	system *System
	//parent   *ActorRef
	//children *safemap.SafeMap[string, *ActorRef]
}

func (x *BaseActor) init(system *System, this IActor) {
	x.IActor = this
}
