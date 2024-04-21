package actor

import (
	"context"
	"google.golang.org/protobuf/proto"
)

type IContext interface {
	Self() *ActorRef
	Sender() *ActorRef
	Message() proto.Message
}

var _ IContext = (*Context)(nil)

type Context struct {
	self    *ActorRef
	sender  *ActorRef
	message proto.Message
	context.Context
}

func newContext(self iProcess, sender *ActorRef, message proto.Message, ctx context.Context) *Context {
	return &Context{
		self:    self.self(),
		sender:  sender,
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
