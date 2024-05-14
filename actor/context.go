package actor

import (
	"context"
	"google.golang.org/protobuf/proto"
)

type IContext interface {
	Self() *ActorRef
	Sender() *ActorRef
	GetMsgSnId() uint64
	Message() proto.Message
	Reply(message proto.Message)
}

var _ IContext = (*Context)(nil)

type Context struct {
	system  *System
	self    *ActorRef
	sender  *ActorRef
	msgSnId uint64
	message proto.Message
	context.Context
}

func (x *Context) Reply(message proto.Message) {
	x.system.send(x.Sender(), message, x.msgSnId)
}

func newContext(self *ActorRef, sender *ActorRef, message proto.Message, msgSnId uint64, ctx context.Context, system *System) *Context {
	return &Context{
		system:  system,
		self:    self,
		sender:  sender,
		msgSnId: msgSnId,
		message: message,
		Context: ctx,
	}
}

func (x *Context) Self() *ActorRef {
	return x.self
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
