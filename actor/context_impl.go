package actor

import (
	"google.golang.org/protobuf/proto"
)

var _ Context = (*contextImpl)(nil)

type contextImpl struct {
	senderFunc SenderWith
	target     *ActorRef
	sender     *ActorRef
	msgSnId    uint64
	message    proto.Message
}

func (x *contextImpl) Reply(message proto.Message) {
	x.senderFunc(x.Sender(), message, nil, x.msgSnId)
}

func newContext(target *ActorRef, sender *ActorRef, message proto.Message, msgSnId uint64, senderFunc SenderWith) Context {
	return &contextImpl{
		senderFunc: senderFunc,
		target:     target,
		sender:     sender,
		msgSnId:    msgSnId,
		message:    message,
	}
}

func (x *contextImpl) Target() *ActorRef {
	return x.target
}

func (x *contextImpl) Sender() *ActorRef {
	return x.sender
}

func (x *contextImpl) Message() proto.Message {
	return x.message
}

func (x *contextImpl) GetMsgSnId() uint64 {
	return x.msgSnId
}

func (x *contextImpl) Forward(target *ActorRef) {
	x.senderFunc(target, x.message, x.sender, x.msgSnId)
}
