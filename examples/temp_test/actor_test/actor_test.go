package main

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/examples/testpb"
	"github.com/chenxyzl/grain/utils/helper"
	"sync/atomic"
	"testing"
	"time"
)

var (
	maxIdx      int64 = 10000
	testSystem        = TestSystem{}
	idx         int64 = 0
	parallelism       = 10
	body              = "123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_"
)

type HelloGoActor struct {
	actor.BaseActor
}

func (x *HelloGoActor) Started() error {
	x.Logger().Info("Started")
	return nil
}
func (x *HelloGoActor) PreStop() error {
	x.Logger().Info("PreStop")
	return nil
}
func (x *HelloGoActor) Receive(context actor.IContext) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("tell: %v", msg.GetName()))
		}
	case *testpb.HelloRequest:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("request: %v", msg.GetName()))
			x.System().Send(context.Sender(), &testpb.HelloReply{Name: "hell go reply"})
		}
	default:
		x.Logger().Error("xxx")
	}
}

type TestSystem struct {
	system *actor.System
	actors []*actor.ActorRef
}

func init() {

	//log
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}).WithRequestTimeout(time.Second * 1)
	//new
	testSystem.system = actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	testSystem.system.Logger().Warn("system starting")
	if err := testSystem.system.Start(); err != nil {
		panic(err)
	}
	testSystem.system.Logger().Warn("system started successfully")

	for i := int64(0); i < maxIdx; i++ {
		actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloGoActor{} })
		testSystem.actors = append(testSystem.actors, actorRef)
	}
}
func BenchmarkHelloSend(b *testing.B) {
	b.ResetTimer()
	actorRef := testSystem.actors[0]
	for range b.N {
		testSystem.system.Send(actorRef, &testpb.Hello{Name: "helle grain"})
	}
}

func BenchmarkHelloRequestOne(b *testing.B) {
	b.ResetTimer()
	actorRef := testSystem.actors[0]
	for range b.N {
		reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: body})
		if reply == nil {
			b.Error()
		}
	}
}

func BenchmarkHelloRequestMore(b *testing.B) {
	b.ResetTimer()

	// 限制并发数
	b.SetParallelism(parallelism)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1)
			_ = v
			actorRef := testSystem.actors[v%maxIdx]
			reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: body})
			if reply == nil {
				b.Error()
			}
		}
	})
}
func BenchmarkRequestTest(b *testing.B) {
	b.ResetTimer()

	// 限制并发数
	b.SetParallelism(parallelism)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1)
			_ = v
			actorRef := testSystem.actors[v%maxIdx]
			reply := actor.RequestTest[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: body})
			if reply == nil {
				b.Error()
			}
		}
	})
}

func BenchmarkRequestTest1(b *testing.B) {
	b.ResetTimer()

	// 限制并发数
	b.SetParallelism(parallelism)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1)
			_ = v
			actorRef := testSystem.actors[v%maxIdx]
			reply := actor.RequestTest1[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: body})
			if reply == nil {
				b.Error()
			}
		}
	})
}

func BenchmarkRequestTest2(b *testing.B) {
	b.ResetTimer()

	// 限制并发数
	b.SetParallelism(parallelism)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1)
			_ = v
			actorRef := testSystem.actors[v%maxIdx]
			reply := actor.RequestTest2[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: body})
			if reply == nil {
				b.Error()
			}
		}
	})
}
