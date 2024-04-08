package actor

import (
	"google.golang.org/protobuf/proto"
	"log/slog"
)

type Context interface {
	Logger() *slog.Logger
	Self() *ActorRef
	Sender() *ActorRef
	Message() proto.Message
}

type ActorContext struct {
	logger  *slog.Logger
	self    IActor
	sender  *ActorRef
	message proto.Message
}
