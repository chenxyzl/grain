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
//
// ⚠️ The msg pointer is DELIVERED AS-IS, never copied. Treat it as immutable — owned by
// the scheduler — from the moment it is handed over:
//
//   - ScheduleRepeated delivers the SAME instance on every tick, so the target actor and
//     the timer goroutine both hold it for the schedule's whole life. A field the handler
//     writes is still there on the next tick, which makes ticks stateful in a way nothing
//     at the call site suggests.
//   - The caller keeps its own reference too, so mutating msg after the call changes what
//     the next tick delivers, from a goroutine the actor knows nothing about.
//   - For a REMOTE target this is a genuine data race, not just surprising aliasing: the
//     write-stream actor marshals msg on its own goroutine, concurrently with any handler
//     or caller writing to it. proto.Marshal on a struct being mutated can emit torn
//     bytes.
//
// Nothing here is enforced, because a copy per tick would cost an allocation on a path
// that exists precisely to be cheap. The two safe patterns:
//
//	// 1. schedule a fieldless trigger and build the real message in the handler
//	x.ScheduleSelfRepeated(0, time.Minute, &pb.Tick{})
//
//	// 2. if the handler must mutate, clone first
//	m := proto.Clone(ctx.Message()).(*pb.Save)
//	m.At = timestamppb.Now()
type IScheduler interface {
	ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc
	ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc
}

type iRpcServer interface {
	Start() error
	Stop() error
	Addr() string
}
