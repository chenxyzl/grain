package actor

import (
	"net"
	"time"
)

type NodeState struct {
	NodeId  uint64
	Address net.Addr
	Time    time.Time
	Version string
	Kinds   []string
}

type ActorState struct {
	Id      string //
	Kind    string
	Address net.Addr
}
