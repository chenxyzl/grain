package grain

import (
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
)

// ISystem ...
type ISystem interface {
	addr() string
	getSender() iSender
	getGenRequestId() iGenRequestId
	getConfig() *config
	getRegistry() iRegistry
	getProvider() iProvider
	GetScheduler() iScheduler
	//
	Logger() *slog.Logger
	// life
	Start()
	ForceStop(error)
	WaitStopSignal()
	//spawn
	Spawn(p iProducer, opts ...KindOptFunc) ActorRef
	SpawnNamed(p iProducer, name string, opts ...KindOptFunc) ActorRef
	Poison(ref ActorRef)
	//
	GetClusterActorRef(kind string, name string) ActorRef
	//sub pub
	Subscribe(ref ActorRef, message proto.Message)
	Unsubscribe(ref ActorRef, message proto.Message)
	PublishGlobal(message proto.Message)
	PublishLocal(message proto.Message)
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
	add(proc iProcess) iProcess
	remove(actRef ActorRef)
}

// iGenRequestId ...
type iGenRequestId interface {
	genRequestId() uint64
}

// CancelScheduleFunc ...
type CancelScheduleFunc func()

// iScheduler ...
type iScheduler interface {
	ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc
	ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc
}

type iRegister interface {
	add(proc iProcess) iProcess
	remove(actRef ActorRef)
}

type iRpcServer interface {
	Start() error
	Stop() error
	Addr() string
}
