package actor

import "github.com/chenxyzl/grain/actor/iface"

type process interface {
	Self() iface.ActorRef
}
