package grain

import (
	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

func (x *system) Subscribe(ref ActorRef, msg proto.Message) {
	x.tell(x.eventStream, &message.Subscribe{ActorId: ref.GetId(), EventName: string(proto.MessageName(msg))})
}
func (x *system) Unsubscribe(ref ActorRef, msg proto.Message) {
	x.tell(x.eventStream, &message.Unsubscribe{ActorId: ref.GetId(), EventName: string(proto.MessageName(msg))})
}
func (x *system) PublishGlobal(msg proto.Message) {
	x.tell(x.eventStream, &message.BroadcastPublishProtoWrapper{Message: msg})
}
func (x *system) PublishLocal(msg proto.Message) {
	x.tell(x.eventStream, msg)
}
