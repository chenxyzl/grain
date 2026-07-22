package grain

import (
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
)

// ISystem ...
type ISystem interface {
	/*
		get
	*/
	getAddr() string
	getSender() iSender
	getConfig() *config
	getRegistry() iRegistry
	GetProvider() iProvider
	GetScheduler() iScheduler
	Logger() *slog.Logger
	nextSnId() uint64

	/*
		system life
	*/
	Start()
	ForceStop(error)
	WaitStopSignal(beforeQuit func(), afterQuit func())
	/*
		actor create/poison
	*/
	Spawn(p iProducer, opts ...KindOptFunc) ActorRef
	SpawnNamed(p iProducer, name string, opts ...KindOptFunc) ActorRef
	Poison(ref ActorRef)
	/*
		get cluster actorRef
	*/
	GetClusterActorRef(kind string, name string) ActorRef
	getAddrHash() *AddrHash
	/*
		sub pub
	*/
	Subscribe(ref ActorRef, message proto.Message)
	Unsubscribe(ref ActorRef, message proto.Message)
	PublishLocal(message proto.Message)
	PublishGlobal(message proto.Message)
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
	add(iProcP iProcessProvider) iProcess
	getOrAdd(id string, iProcP iProcessProvider) iProcess
	remove(actRef ActorRef)
}

// CancelScheduleFunc ...
type CancelScheduleFunc func()

// iScheduler ...
type iScheduler interface {
	ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc
	ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc
}

type iRpcServer interface {
	Start() error
	Stop() error
	Addr() string
}
