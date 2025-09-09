package main

import (
	"examples/testpb"
	"log/slog"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenxyzl/grain"
)

var (
	actorCount  int64 = 10000
	testSystem        = TestSystem{}
	idx         int64 = 0
	parallelism       = 32
	body              = "hello world"
	helloSend         = &testpb.Hello{Name: body}
	helloAsk          = &testpb.HelloAsk{Name: body}
	helloReply        = &testpb.HelloReply{Name: "hell go reply"}
	askTimeout        = time.Second * 1
)

type TestSystem struct {
	system grain.ISystem
	actors []grain.ActorRef
}

type HelloActor struct {
	grain.BaseActor
}

func (x *HelloActor) Started() {
	x.Logger().Info("Started")
}
func (x *HelloActor) PreStop() {
	x.Logger().Info("PreStop")
}
func (x *HelloActor) Receive(context grain.Context) {
	switch context.Message().(type) {
	case *testpb.Hello:
	case *testpb.HelloAsk:
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
	//new
	testSystem.system = grain.NewSystem("hello", "0.0.1", []string{"127.0.0.1:2379"}, grain.WithConfigAskTimeout(askTimeout))
	//start
	testSystem.system.Logger().Warn("system starting")
	//
	testSystem.system.Start()
	//
	testSystem.system.Logger().Warn("system started successfully")

	for i := int64(0); i < actorCount; i++ {
		actorRef := testSystem.system.Spawn(func() grain.IActor { return &HelloActor{} })
		testSystem.actors = append(testSystem.actors, actorRef)
	}
}
func BenchmarkSendOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() grain.IActor { return &HelloActor{} })
	b.ResetTimer()
	for range b.N {
		actorRef.Send(helloSend)
	}
}
func BenchmarkSendMore(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % actorCount
			_ = v
			actorRef := testSystem.actors[v]
			actorRef.Send(helloSend)
		}
	})
}
func BenchmarkAskOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() grain.IActor { return &HelloActor{} })
	b.ResetTimer()
	for range b.N {
		reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, helloAsk)
		if reply == nil {
			b.Error(err)
		}
	}
}
func BenchmarkAskMore(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % actorCount
			_ = v
			actorRef := testSystem.actors[v]
			reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, helloAsk)
			if reply == nil {
				b.Error(err)
			}
		}
	})
}

// TestMain main enter
func TestMain(m *testing.M) {
	// init
	testSystem.system.Logger().Info("test init")
	// run
	exitCode := m.Run()
	testSystem.system.Logger().Info("test end with code:" + strconv.Itoa(exitCode))
	// end with clean
	testSystem.system.ForceStop(nil)
	testSystem.system.Logger().Info("test exit")
}
