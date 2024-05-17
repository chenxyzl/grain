package actor

import (
	"context"
	"google.golang.org/protobuf/proto"
)

type IContext interface {
	Target() *ActorRef
	Sender() *ActorRef
	GetMsgSnId() uint64
	Message() proto.Message
	Reply(message proto.Message)
	Forward(target *ActorRef)
}

var _ IContext = (*Context)(nil)

type Context struct {
	system  *System
	target  *ActorRef
	sender  *ActorRef
	msgSnId uint64
	message proto.Message
	context.Context
}

func (x *Context) Reply(message proto.Message) {
	x.system.sendWithoutSender(x.Sender(), message, x.msgSnId)
}

func newContext(target *ActorRef, sender *ActorRef, message proto.Message, msgSnId uint64, ctx context.Context, system *System) *Context {
	return &Context{
		system:  system,
		target:  target,
		sender:  sender,
		msgSnId: msgSnId,
		message: message,
		Context: ctx,
	}
}

func (x *Context) Target() *ActorRef {
	return x.target
}

func (x *Context) Sender() *ActorRef {
	return x.sender
}

func (x *Context) Message() proto.Message {
	return x.message
}

func (x *Context) GetMsgSnId() uint64 {
	return x.msgSnId
}

func (x *Context) Forward(target *ActorRef) {
	x.system.sendWithSender(target, x.message, x.sender, x.msgSnId)
}
