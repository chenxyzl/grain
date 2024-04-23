package actor

import (
	"net"
)

type NodeState struct {
	NodeId  uint64
	Address string
	Time    string
	Version string
	Kinds   []string
}

type ActorState struct {
	Id      string //
	Kind    string
	Address net.Addr
}
