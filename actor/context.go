package actor

import (
	"context"
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

var _ Context = (*contextImpl)(nil)

type contextImpl struct {
	system  *System
	target  *ActorRef
	sender  *ActorRef
	msgSnId uint64
	message proto.Message
	context.Context
}

func (x *contextImpl) Reply(message proto.Message) {
	x.system.sendWithoutSender(x.Sender(), message, x.msgSnId)
}

func newContext(target *ActorRef, sender *ActorRef, message proto.Message, msgSnId uint64, ctx context.Context, system *System) Context {
	return &contextImpl{
		system:  system,
		target:  target,
		sender:  sender,
		msgSnId: msgSnId,
		message: message,
		Context: ctx,
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
	x.system.sendWithSender(target, x.message, x.sender, x.msgSnId)
}
