package actor

import (
	"google.golang.org/protobuf/proto"
)

func (x *System) Subscribe(ref *ActorRef, message proto.Message) {
	x.send(x.eventStream, &Subscribe{Self: ref, EventName: string(proto.MessageName(message))}, x.getNextSnId())
}
func (x *System) Unsubscribe(ref *ActorRef, message proto.Message) {
	x.send(x.eventStream, &Unsubscribe{Self: ref, EventName: string(proto.MessageName(message))}, x.getNextSnId())
}
func (x *System) PublishLocal(message proto.Message) {
	x.send(x.eventStream, message, x.getNextSnId())
}
func (x *System) PublishGlobal(message proto.Message) {
	nodes := x.clusterProvider.getNodes()
	for _, node := range nodes {
		actRef := newActorRefWithKind(node.Address, defaultSystemKind, eventStreamName)
		x.send(actRef, message, x.getNextSnId())
	}
}
