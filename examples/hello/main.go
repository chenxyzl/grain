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

func (x *HelloGoActorA) Started() error {
	x.Logger().Info("Started")
	return nil
}
func (x *HelloGoActorA) PreStop() error {
	x.Logger().Info("PreStop")
	return nil
}
func (x *HelloGoActorA) Receive(context actor.IContext) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("tell: %v", msg.GetName()))
		}
	case *testpb.HelloRequest:
		{
			_ = msg
			r2 := x.Request(bRef, &testpb.HelloRequestA2B{Name: body}).(*testpb.HelloReplyA2B)
			if r2 == nil {
				panic("x")
			}
			//x.Logger().Info(fmt.Sprintf("request: %v", msg.GetName()))
			if context.Sender() != nil {
				x.Send(context.Sender(), &testpb.HelloReply{Name: "hell go reply from A"})
			}
		}
	case *testpb.HelloRequestB2A:
		{
			//x.Logger().Info(fmt.Sprintf("request: %v", msg.GetName()))
			if context.Sender() != nil {
				x.Send(context.Sender(), &testpb.HelloReplyB2A{Name: "hell go reply from A"})
			}
		}
	default:
		x.Logger().Error("xxx")
	}
}

type HelloGoActorB struct {
	actor.BaseActor
}

func (x *HelloGoActorB) Started() error {
	x.Logger().Info("Started B")
	return nil
}
func (x *HelloGoActorB) PreStop() error {
	x.Logger().Info("PreStop B")
	return nil
}
func (x *HelloGoActorB) Receive(context actor.IContext) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("tell: %v", msg.GetName()))
		}
	case *testpb.HelloRequestA2B:
		{
			_ = msg
			r2 := x.Request(aRef, &testpb.HelloRequestB2A{Name: body}).(*testpb.HelloReplyB2A)
			if r2 == nil {
				panic("x")
			}
			//x.Logger().Info(fmt.Sprintf("request: %v", msg.GetName()))
			if context.Sender() != nil {
				x.Send(context.Sender(), &testpb.HelloReplyA2B{Name: "hell go reply from B"})
			}
		}
	default:
		x.Logger().Error("xxx")
	}
}
func init() {
	//log
	helper.InitLog("./test.log")
	slog.SetLogLoggerLevel(slog.LevelWarn)
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}).
		WithRequestTimeout(requestTimeout)
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
	r1, _ := actor.NoEntryRequestE[*testpb.HelloReply](testSystem.system, aRef, &testpb.HelloRequest{Name: body})
	if r1 == nil {
		panic("x")
	}
}
