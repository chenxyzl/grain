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

var _ Context = (*ContextImpl)(nil)

type ContextImpl struct {
	system  *System
	target  *ActorRef
	sender  *ActorRef
	msgSnId uint64
	message proto.Message
	context.Context
}

func (x *ContextImpl) Reply(message proto.Message) {
	x.system.sendWithoutSender(x.Sender(), message, x.msgSnId)
}

func newContext(target *ActorRef, sender *ActorRef, message proto.Message, msgSnId uint64, ctx context.Context, system *System) Context {
	return &ContextImpl{
		system:  system,
		target:  target,
		sender:  sender,
		msgSnId: msgSnId,
		message: message,
		Context: ctx,
	}
}

func (x *ContextImpl) Target() *ActorRef {
	return x.target
}

func (x *ContextImpl) Sender() *ActorRef {
	return x.sender
}

func (x *ContextImpl) Message() proto.Message {
	return x.message
}

func (x *ContextImpl) GetMsgSnId() uint64 {
	return x.msgSnId
}

func (x *ContextImpl) Forward(target *ActorRef) {
	x.system.sendWithSender(target, x.message, x.sender, x.msgSnId)
}
