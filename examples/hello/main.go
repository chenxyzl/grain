package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"log/slog"
	"runtime"
	"time"
)

var (
	testSystem     = TestSystem{}
	body           = "123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_"
	requestTimeout = time.Second * 100
	aRef           *actor.ActorRef
	bRef           *actor.ActorRef
)

type TestSystem struct {
	system *actor.System
}

type HelloGoActorA struct {
	actor.BaseActor
	times int
}

func (x *HelloGoActorA) Started() {
	x.Logger().Info("Started")
}

func (x *HelloGoActorA) PreStop() {
	x.Logger().Info("PreStop")
}
func (x *HelloGoActorA) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("tell: %v", msg.GetName()))
		}
	case *testpb.HelloRequest:
		{
			x.Logger().Info("HelloRequestA2B")
			_ = msg
			r2 := x.Request(bRef, &testpb.HelloRequestA2B{Name: body}).(*testpb.HelloReplyA2B)
			_ = r2
			x.Logger().Info("HelloReplyA2B end")
			context.Reply(&testpb.HelloReply{Name: "hell go reply from A"})
		}
	case *testpb.HelloRequestB2A:
		{
			context.Reply(&testpb.HelloReplyB2A{Name: "hell go reply from A"})
		}
	default:
		x.Logger().Error("unknown msg")
	}
}

type HelloGoActorB struct {
	actor.BaseActor
}

func (x *HelloGoActorB) Started() {
	x.Logger().Info("Started B")
}
func (x *HelloGoActorB) PreStop() {
	x.Logger().Info("PreStop B")
}
func (x *HelloGoActorB) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("tell: %v", msg.GetName()))
		}
	case *testpb.HelloRequestA2B:
		{
			x.Logger().Info("HelloRequestB2A")
			_ = msg
			r2 := x.Request(aRef, &testpb.HelloRequestB2A{Name: body}).(*testpb.HelloReplyB2A)
			_ = r2
			x.Logger().Info("HelloReplyB2A end")
			context.Reply(&testpb.HelloReplyA2B{Name: "hell go reply from B"})
		}
	default:
		x.Logger().Error("unknown msg")
	}
}
func init() {
	//log
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}).
		WithRequestTimeout(requestTimeout).
		WithKind("hello", func() actor.IActor { return &HelloGoActorA{} })
	//new
	testSystem.system = actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	testSystem.system.Logger().Warn("system starting")
	if err := testSystem.system.Start(); err != nil {
		panic(err)
	}
	testSystem.system.Logger().Warn("system started successfully")

	n := runtime.NumCPU()
	runtime.GOMAXPROCS(n * 2)

	aRef = testSystem.system.Spawn(func() actor.IActor { return &HelloGoActorA{} })
	bRef = testSystem.system.Spawn(func() actor.IActor { return &HelloGoActorB{} })
}
func main() {
	slog.Info("HelloRequest")
	r1, err := actor.NoEntryRequestE[*testpb.HelloReply](testSystem.system, aRef, &testpb.HelloRequest{Name: body})
	if r1 == nil || err != nil {
		panic("x")
	}
	slog.Info("HelloReply end")
	testSystem.system.WaitStopSignal()
}
