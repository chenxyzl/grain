package actor

import (
	"github.com/chenxyzl/grain/actor/iface"
	"google.golang.org/protobuf/types/known/anypb"
)

type messageHeader map[string]string

type MessageEnvelope struct {
	Sender  iface.ActorRef
	Header  messageHeader
	Message anypb.Any
}
