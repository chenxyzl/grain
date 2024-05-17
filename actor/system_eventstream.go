package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
	"google.golang.org/protobuf/proto"
)

func (x *System) Subscribe(ref *ActorRef, message proto.Message) {
	x.sendWithoutSender(x.eventStream, &Subscribe{Self: ref, EventName: string(proto.MessageName(message))})
}
func (x *System) Unsubscribe(ref *ActorRef, message proto.Message) {
	x.sendWithoutSender(x.eventStream, &Unsubscribe{Self: ref, EventName: string(proto.MessageName(message))})
}
func (x *System) Publish(message proto.Message) {
	x.sendWithoutSender(x.eventStream, &internal.BroadcastPublishProtoWrapper{Message: message})
}
