package grain

import (
	"google.golang.org/protobuf/proto"
)

var _ Context = (*contextImpl)(nil)

type contextImpl struct {
	target     ActorRef
	sender     ActorRef
	msgSnId    uint64
	message    proto.Message
	senderFunc iSender
}

func (x *contextImpl) Reply(message proto.Message) {
	x.senderFunc.tellWithSender(x.Sender(), message, x.Target(), x.msgSnId)
}

func newContext(target ActorRef, sender ActorRef, message proto.Message, msgSnId uint64, senderFunc iSender) Context {
	return &contextImpl{
		target:     target,
		sender:     sender,
		msgSnId:    msgSnId,
		message:    message,
		senderFunc: senderFunc,
	}
}

func (x *contextImpl) Target() ActorRef {
	return x.target
}

func (x *contextImpl) Sender() ActorRef {
	if x.sender == nil {
		return nil
	}
	return x.sender
}

func (x *contextImpl) Message() proto.Message {
	return x.message
}

func (x *contextImpl) GetMsgSnId() uint64 {
	return x.msgSnId
}

func (x *contextImpl) Forward(target ActorRef) {
	x.senderFunc.tellWithSender(target, x.message, x.sender, x.msgSnId)
}
