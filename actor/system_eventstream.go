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
func (x *System) Publish(message proto.Message) {
	x.send(x.eventStream, newPublishWrapper(message), x.getNextSnId())
}
