package actor

import (
	"google.golang.org/protobuf/proto"
)

type Context interface {
	Target() *ActorRef
	Sender() *ActorRef
	GetMsgSnId() uint64
	Message() proto.Message
	Reply(message proto.Message)
	Forward(target *ActorRef)
}
