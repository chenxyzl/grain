package actor

import (
	"github.com/chenxyzl/grain/actor/iface"
	"google.golang.org/protobuf/proto"
	"log/slog"
)

type Context interface {
	Logger() *slog.Logger
	Self() *iface.ActorRef
	Sender() *iface.ActorRef
	Message() proto.Message
}

type ActorContext struct {
	logger  *slog.Logger
	self    IActor
	sender  *iface.ActorRef
	message proto.Message
}
