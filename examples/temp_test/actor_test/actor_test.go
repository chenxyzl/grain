package main

import (
	"encoding/json"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/examples/testpb"
	"github.com/chenxyzl/grain/utils/al/safemap"
	"github.com/chenxyzl/grain/utils/helper"
	"google.golang.org/protobuf/proto"
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
	testMap              = safemap.NewM[string, string]()
	testStringMap        = safemap.NewStringC[string]()
	testIntMap           = safemap.NewIntC[int, string]()
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
				x.System().Send(context.Sender(), &testpb.HelloReply{Name: "hell go reply"})
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
		testMap.Set(actorRef.GetId(), actorRef.GetId())
		testStringMap.Set(actorRef.GetId(), actorRef.GetId())
		testIntMap.Set(int(i), actorRef.GetId())
	}

	n := runtime.NumCPU()
	runtime.GOMAXPROCS(n * 2)
}
func BenchmarkSendOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloGoActor{} })
	b.ResetTimer()
	for range b.N {
		testSystem.system.Send(actorRef, &testpb.Hello{Name: "helle grain"})
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
			testSystem.system.Send(actorRef, helloRequest)
		}
	})
}
func BenchmarkRequestOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() actor.IActor { return &HelloGoActor{} })
	b.ResetTimer()
	for range b.N {
		reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, helloRequest)
		if reply == nil {
			b.Error()
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
			reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, helloRequest)
			if reply == nil {
				b.Error()
			}
		}
	})
}

func BenchmarkMarshalJson(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = v
			msg := &testpb.HelloRequest{}
			//marshal
			content, _ := json.Marshal(helloRequest)
			_ = json.Unmarshal(content, msg)
		}
	})
}

func BenchmarkMarshal(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = v
			msg := &testpb.HelloRequest{}
			//marshal
			content, _ := proto.Marshal(helloRequest)
			_ = proto.Unmarshal(content, msg)
		}
	})
}

func BenchmarkSafeMap1(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			//
			actorRef := testSystem.actors[v]
			testMap.Get(actorRef.GetId())
			//testMap.Set(actorRef.GetId(), actorRef.GetId())
			//testMap.Get(actorRef.GetId())
		}
	})
}
func BenchmarkSafeMap2(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testMap.Get(actorRef.GetId())
			testMap.Set(actorRef.GetId(), actorRef.GetId())
			//testMap.Get(actorRef.GetId())
		}
	})
}
func BenchmarkSafeMap3(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testMap.Get(actorRef.GetId())
			testMap.Set(actorRef.GetId(), actorRef.GetId())
			testMap.Get(actorRef.GetId())
		}
	})
}

func BenchmarkSafeMap4(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testStringMap.Get(actorRef.GetId())
			//testStringMap.Set(actorRef.GetId(), actorRef.GetId())
			//testStringMap.Get(actorRef.GetId())
		}
	})
}
func BenchmarkSafeMap5(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testStringMap.Get(actorRef.GetId())
			testStringMap.Set(actorRef.GetId(), actorRef.GetId())
			//testStringMap.Get(actorRef.GetId())
		}
	})
}
func BenchmarkSafeMap6(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testStringMap.Get(actorRef.GetId())
			testStringMap.Set(actorRef.GetId(), actorRef.GetId())
			testStringMap.Get(actorRef.GetId())
		}
	})
}

func BenchmarkSafeMap7(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = testSystem.actors[v]
			testIntMap.Get(int(v))
			//testIntMap.Set(actorRef.GetId(), actorRef.GetId())
			//testIntMap.Get(actorRef.GetId())
		}
	})
}
func BenchmarkSafeMap8(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testIntMap.Get(int(v))
			testIntMap.Set(int(v), actorRef.GetId())
			//testIntMap.Get(actorRef.GetId())
		}
	})
}
func BenchmarkSafeMap9(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			actorRef := testSystem.actors[v]
			testIntMap.Get(int(v))
			testIntMap.Set(int(v), actorRef.GetId())
			testIntMap.Get(int(v))
		}
	})
}
