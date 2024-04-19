package actor

import (
	"google.golang.org/protobuf/types/known/anypb"
)

type messageHeader map[string]string

type MessageEnvelope struct {
	Sender  *ActorRef
	Header  messageHeader
	Message anypb.Any
}
