package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"log/slog"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var (
	maxIdx         int64 = 10000
	testSystem           = TestSystem{}
	idx            int64 = 0
	parallelism          = 100
	body                 = "123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_123456789_"
	helloRequest         = &testpb.HelloRequest{Name: body}
	requestTimeout       = time.Second * 1
)

type TestSystem struct {
	system *actor.System
	actors []*actor.ActorRef
}

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
			if context.Sender() != nil {
				x.Send(context.Sender(), &testpb.HelloReply{Name: "hell go reply"})
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
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}).WithRequestTimeout(requestTimeout)
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

	n := runtime.NumCPU()
	runtime.GOMAXPROCS(n * 2)
}
func BenchmarkSendOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloGoActor{} })
	b.ResetTimer()
	for range b.N {
		actor.NoEntrySend(testSystem.system, actorRef, &testpb.Hello{Name: "helle grain"})
	}
}
func BenchmarkSendMore(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = v
			actorRef := testSystem.actors[v]
			actor.NoEntrySend(testSystem.system, actorRef, helloRequest)
		}
	})
}
func BenchmarkRequestOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloGoActor{} })
	b.ResetTimer()
	for range b.N {
		reply, err := actor.NoEntryRequestE[*testpb.HelloReply](testSystem.system, actorRef, helloRequest)
		if reply == nil {
			b.Error(err)
		}
	}
}
func BenchmarkRequestMore(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = v
			actorRef := testSystem.actors[v]
			reply, err := actor.NoEntryRequestE[*testpb.HelloReply](testSystem.system, actorRef, helloRequest)
			if reply == nil {
				b.Error(err)
			}
		}
	})
}
