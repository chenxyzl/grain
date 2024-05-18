package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
	"google.golang.org/protobuf/proto"
)

func (x *System) Subscribe(ref *ActorRef, message proto.Message) {
	x.sendWithoutSender(x.eventStream, &internal.Subscribe{ActorId: ref.GetId(), EventName: string(proto.MessageName(message))})
}
func (x *System) Unsubscribe(ref *ActorRef, message proto.Message) {
	x.sendWithoutSender(x.eventStream, &internal.Unsubscribe{ActorId: ref.GetId(), EventName: string(proto.MessageName(message))})
}
func (x *System) PublishGlobal(message proto.Message) {
	x.sendWithoutSender(x.eventStream, &internal.BroadcastPublishProtoWrapper{Message: message})
}
func (x *System) PublishLocal(message proto.Message) {
	x.sendWithoutSender(x.eventStream, message)
}
