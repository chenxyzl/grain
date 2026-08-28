package grain

import (
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
)

// iSystem groups the framework-internal hooks. It is embedded INTO ISystem rather
// than wrapping it, which buys three things at once:
//
//   - Encapsulation: an unexported method is unreachable from another package
//     ("cannot refer to unexported method"), so these stay internal even while riding
//     on the public interface.
//   - A SEALED interface: an outside type cannot satisfy ISystem either ("does not
//     implement ISystem (unexported method ...)"), so the framework stays free to add
//     methods without breaking anyone's implementation.
//   - No dual typing: internal code calls these straight off any ISystem value, so
//     ActorRef needs GetSystem() only — no second unexported accessor.
type iSystem interface {
	getAddr() string
	getSender() iSender
	getConfig() *config
	getRegistry() iRegistry
	getProvider() iProvider
	getAddrHash() *AddrHash
	nextSnId() uint64
	/*
		ask correlation (reply futures)
	*/
	registerAsk(snId uint64) chan proto.Message
	cancelAsk(snId uint64)
}

// ISystem is the actor system: what NewSystem returns and what ActorRef.GetSystem()
// hands back.
//
// The exported half is the entire user-facing contract; the embedded iSystem is the
// framework's own and is unreachable from outside this package. Grouping the internals
// behind one embedded name also keeps godoc readable — nine unexported methods used to
// sit inline among the public ones.
type ISystem interface {
	iSystem
	/*
		system life
	*/
	Start()
	ForceStop(error)
	WaitStopSignal(beforeQuit func(), afterQuit func())
	/*
		actor create/poison
	*/
	Spawn(p Producer, opts ...KindOptFunc) ActorRef
	SpawnNamed(p Producer, name string, opts ...KindOptFunc) (ActorRef, error)
	Poison(ref ActorRef)
	/*
		get cluster actorRef
	*/
	GetClusterActorRef(kind string, name string) ActorRef
	/*
		sub pub
	*/
	Subscribe(ref ActorRef, message proto.Message)
	Unsubscribe(ref ActorRef, message proto.Message)
	PublishLocal(message proto.Message)
	PublishGlobal(message proto.Message)
	/*
		scheduling + logging
	*/
	GetScheduler() IScheduler
	Logger() *slog.Logger
	/*
		cluster node info and per-node ext data.

		These forward to the cluster provider. They used to be reached through
		GetProvider(), which returned the unexported iProvider — usable only by
		accident, since a caller could invoke its exported methods but never name the
		type. The provider itself is internal now (getProvider).
	*/
	GetNodeId() uint64
	GetNodeExtData(subKey string) (string, error)
	SetNodeExtData(subKey string, val string) error
	RemoveNodeExtData(subKey string) error
	WatchNodeExtData(subKey string, f func(key, val string)) error
}

// iSender ...
type iSender interface {
	tell(target ActorRef, msg proto.Message)
	tellWithSender(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64)
}

// iSystemLife ...
type iSystemLife interface {
	init(nodeId uint64)
	ForceStop(err error)
}

// iRegistry ...
type iRegistry interface {
	get(actRef ActorRef) iProcess
	add(iProcP iProcessProvider) (iProcess, error)
	getOrAdd(id string, iProcP iProcessProvider) iProcess
	remove(actRef ActorRef)
}

// CancelScheduleFunc ...
type CancelScheduleFunc func()

// IScheduler schedules delayed and repeated message deliveries. Exported because
// ISystem.GetScheduler returns it — an exported method must not return an unexported
// type, or callers cannot declare a variable, write a helper, or mock it.
type IScheduler interface {
	ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc
	ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc
}

type iRpcServer interface {
	Start() error
	Stop() error
	Addr() string
}
