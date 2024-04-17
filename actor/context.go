package actor

import (
	"context"
	"google.golang.org/protobuf/proto"
	"log/slog"
)

type IContext interface {
	Logger() *slog.Logger
	Self() IProcess
	Sender() *ActorRef
	Message() proto.Message
}

var _ IContext = (*Context)(nil)

type Context struct {
	self    IProcess
	sender  *ActorRef
	message proto.Message
	context.Context
}

func newContext(self IProcess, sender *ActorRef, message proto.Message, ctx context.Context) *Context {
	return &Context{
		self:    self,
		sender:  sender,
		message: message,
		Context: ctx,
	}
}

func (x *Context) Logger() *slog.Logger {
	return x.self.Logger()
}

func (x *Context) Self() IProcess {
	return x.self
}

func (x *Context) Sender() *ActorRef {
	return x.sender
}

func (x *Context) Message() proto.Message {
	return x.message
}
