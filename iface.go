package grain

import (
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
)

// iSystem groups the framework-internal hooks. Embedded INTO ISystem rather than wrapping it:
// unexported methods are unreachable from other packages, so these stay internal AND ISystem is
// sealed (no outside type can satisfy it), leaving the framework free to add methods.
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

// ISystem is the actor system: what NewSystem returns and what ActorRef.GetSystem() hands back.
// The exported half is the whole user-facing contract; the embedded iSystem is package-internal.
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
		cluster node info and per-node ext data; forwarded to the cluster provider, which is
		itself internal (getProvider).
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
// ISystem.GetScheduler returns it.
//
// ⚠️ The msg pointer is DELIVERED AS-IS, never copied — treat it as owned by the scheduler from
// the moment it is handed over. ScheduleRepeated delivers the SAME instance on every tick, so a
// field the handler writes is still set on the next one; the caller's own reference aliases it
// too; and for a REMOTE target it is a genuine data race, since the write-stream actor marshals
// msg on its own goroutine and proto.Marshal of a struct being mutated can emit torn bytes. Not
// enforced, because a copy per tick would allocate on a path that exists to be cheap.
//
//	// 1. schedule a fieldless trigger and build the real message in the handler
//	x.ScheduleSelfRepeated(0, time.Minute, &pb.Tick{})
//
//	// 2. or, if the handler must mutate, clone first
//	m := proto.Clone(ctx.Message()).(*pb.Save)
type IScheduler interface {
	ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc
	ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc
}

type iRpcServer interface {
	Start() error
	Stop() error
	Addr() string
}
