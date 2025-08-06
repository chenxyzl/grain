package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
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
	helloSend            = &testpb.Hello{Name: body}
	helloRequest         = &testpb.HelloRequest{Name: body}
	helloReply           = &testpb.HelloReply{Name: "hell go reply"}
	requestTimeout       = time.Second * 1
)

type TestSystem struct {
	system *actor.System
	actors []*actor.ActorRef
}

type HelloActor struct {
	actor.BaseActor
}

func (x *HelloActor) Started() {
	x.Logger().Info("Started")
}
func (x *HelloActor) PreStop() {
	x.Logger().Info("PreStop")
}
func (x *HelloActor) Receive(context actor.Context) {
	switch context.Message().(type) {
	case *testpb.Hello:
	case *testpb.HelloRequest:
		context.Reply(helloReply)
	default:
		x.Logger().Error("unregister msg")
	}
}

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() * 2)
	//log
	//actor.InitLog("./test.log")
	slog.SetLogLoggerLevel(slog.LevelWarn)
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}, actor.WithConfigRequestTimeout(requestTimeout))
	//new
	testSystem.system = actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	testSystem.system.Logger().Warn("system starting")
	//
	testSystem.system.Start()
	//
	testSystem.system.Logger().Warn("system started successfully")

	for i := int64(0); i < maxIdx; i++ {
		actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloActor{} })
		testSystem.actors = append(testSystem.actors, actorRef)
	}
}
func BenchmarkSendOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloActor{} })
	b.ResetTimer()
	for range b.N {
		actor.NoReentrySend(testSystem.system, actorRef, helloSend)
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
			actor.NoReentrySend(testSystem.system, actorRef, helloSend)
		}
	})
}
func BenchmarkRequestOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloActor{} })
	b.ResetTimer()
	for range b.N {
		reply, err := actor.NoReentryRequest[*testpb.HelloReply](testSystem.system, actorRef, helloRequest)
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
			reply, err := actor.NoReentryRequest[*testpb.HelloReply](testSystem.system, actorRef, helloRequest)
			if reply == nil {
				b.Error(err)
			}
		}
	})
}
