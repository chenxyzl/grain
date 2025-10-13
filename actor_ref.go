package grain

import (
	"google.golang.org/protobuf/proto"
)

type ActorRef interface {
	GetSystem() ISystem
	GetId() string
	GetKind() string
	GetName() string
	GetDirectAddr() string

	isDirect() bool
	isCluster() bool
	isAsk() bool

	getRemoteAddrCache() (string, bool)

	Send(msg proto.Message)
	NoReentryAsk(target ActorRef, req proto.Message) proto.Message
	NoReentryAskE(target ActorRef, req proto.Message) (proto.Message, error)
}
