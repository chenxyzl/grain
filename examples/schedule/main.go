package main

import (
	"examples/testpb"
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"runtime"
	"time"
)

var (
	testSystem     = TestSystem{}
	body           = "123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_"
	requestTimeout = time.Second * 100
	aRef           *actor.ActorRef
)

type TestSystem struct {
	system *actor.System
}

type HelloGoActorA struct {
	actor.BaseActor
	times   int
	cancel1 actor.CancelScheduleFunc
	cancel2 actor.CancelScheduleFunc
	cancel3 actor.CancelScheduleFunc
	cancel4 actor.CancelScheduleFunc
}

func (x *HelloGoActorA) Started() {
	x.Logger().Info("Started")
	x.cancel1 = x.ScheduleSelfOnce(time.Second, &testpb.Hello{Name: "once1 delay"})
	x.cancel2 = x.ScheduleSelfOnce(time.Second*2, &testpb.Hello{Name: "once2 delay"})
	x.cancel3 = x.ScheduleSelfRepeated(time.Second, time.Second, &testpb.Hello{Name: "repeated3 delay"})
	x.cancel4 = x.ScheduleSelfRepeated(time.Second*2, time.Second, &testpb.Hello{Name: "repeated4 delay"})
}

func (x *HelloGoActorA) PreStop() {
	x.Logger().Info("PreStop")
	x.cancel1()
	x.cancel2()
	x.cancel3()
	x.cancel4()
}
func (x *HelloGoActorA) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			x.times++
			_ = msg
			x.Logger().Info(fmt.Sprintf("recv: %v", msg.GetName()))
			if x.times > 5 {
				x.cancel2()
				x.Logger().Info(fmt.Sprintf("stop cancel2"))
				x.cancel4()
				x.Logger().Info(fmt.Sprintf("stop cancel4"))
			}
			if x.times > 20 {
				x.cancel1()
				x.Logger().Info(fmt.Sprintf("stop cancel1"))
				x.cancel3()
				x.Logger().Info(fmt.Sprintf("stop cancel3"))
			}
		}
	case *testpb.HelloRequest:
		{
			x.Logger().Info("recv HelloRequest")
			context.Reply(&testpb.HelloReply{Name: "reply HelloRequest"})
		}
	default:
		x.Logger().Error("unknown msg")
	}
}

func init() {
	//log
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("schedule", "0.0.1", []string{"127.0.0.1:2379"}).
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
}
func main() {
	r1, err := actor.NoEntryRequestE[*testpb.HelloReply](testSystem.system, aRef, &testpb.HelloRequest{Name: body})
	if r1 == nil || err != nil {
		panic("x")
	}
	systemScheduleFunc := testSystem.system.ScheduleRepeated(aRef, 0, time.Second, &testpb.Hello{Name: "repeated system delay"})
	time.Sleep(time.Second * 5)
	systemScheduleFunc()
	testSystem.system.WaitStopSignal()
}
