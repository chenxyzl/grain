package actor

import (
	"context"
	"google.golang.org/protobuf/proto"
)

type IContext interface {
	Self() *ActorRef
	Sender() *ActorRef
	RequestId() uint64
	Message() proto.Message
}

var _ IContext = (*Context)(nil)

type Context struct {
	self      *ActorRef
	sender    *ActorRef
	requestId uint64
	message   proto.Message
	context.Context
}

func newContext(self *ActorRef, sender *ActorRef, message proto.Message, requestId uint64, ctx context.Context) *Context {
	return &Context{
		self:      self,
		sender:    sender,
		requestId: requestId,
		message:   message,
		Context:   ctx,
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

func (x *Context) RequestId() uint64 {
	return x.requestId
}
