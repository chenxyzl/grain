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

	Tell(msg proto.Message)
}
