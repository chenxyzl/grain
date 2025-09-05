package main

import (
	"examples/testpb"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/chenxyzl/grain"
)

var (
	testSystem     = TestSystem{}
	body           = "123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_"
	requestTimeout = time.Second * 100
	aRef           grain.ActorRef
)

type TestSystem struct {
	system grain.ISystem
}

type HelloGoActorA struct {
	grain.BaseActor
	times   int
	cancel1 grain.CancelScheduleFunc
	cancel2 grain.CancelScheduleFunc
	cancel3 grain.CancelScheduleFunc
	cancel4 grain.CancelScheduleFunc
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
func (x *HelloGoActorA) Receive(context grain.Context) {
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
	grain.InitLog("./test.log", slog.LevelInfo)
	//new
	testSystem.system = grain.NewSystem("schedule", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigRequestTimeout(requestTimeout),
		grain.WithConfigKind("hello", func() grain.IActor { return &HelloGoActorA{} }))
	//start
	testSystem.system.Logger().Warn("system starting")
	//
	testSystem.system.Start()
	//
	testSystem.system.Logger().Warn("system started successfully")

	n := runtime.NumCPU()
	runtime.GOMAXPROCS(n * 2)

	aRef = testSystem.system.Spawn(func() grain.IActor { return &HelloGoActorA{} })
}
func main() {
	r1, err := grain.NoReentryRequest[*testpb.HelloReply](aRef, &testpb.HelloRequest{Name: body})
	if r1 == nil || err != nil {
		panic("x")
	}
	systemScheduleFunc := testSystem.system.GetScheduler().ScheduleRepeated(aRef, 0, time.Second, &testpb.Hello{Name: "repeated system delay"})
	time.Sleep(time.Second * 5)
	systemScheduleFunc()
	testSystem.system.WaitStopSignal()
}
