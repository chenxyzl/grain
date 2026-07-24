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
	// askSnId returns the correlation id of a reply ref (valid only when isAsk()).
	askSnId() uint64

	getRemoteAddrCache() (string, bool)

	Tell(msg proto.Message)
}
