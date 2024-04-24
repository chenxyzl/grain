package main

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/examples/testpb"
	"github.com/chenxyzl/grain/utils/helper"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

var (
	maxIdx     int64 = 10
	testSystem       = TestSystem{}
)

func init() {

	//log
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}).WithRequestTimeout(time.Second * 10)
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

func TestRequestOne(t *testing.T) {
	actorRef := testSystem.actors[0]
	reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: "hello go request"})
	if reply == nil {
		t.Error()
	}
	reply1 := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: "hello go request"})
	if reply1 == nil {
		t.Error()
	}
}

func BenchmarkHelloSend(b *testing.B) {
	actorRef := testSystem.actors[0]
	b.ResetTimer()
	for range b.N {
		testSystem.system.Send(actorRef, &testpb.Hello{Name: "helle grain"})
	}
}

func BenchmarkHelloRequestOne(b *testing.B) {
	actorRef := testSystem.actors[0]
	b.ResetTimer()
	for range b.N {
		reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: "hello go request"})
		if reply == nil {
			b.Error()
		}
	}
}

func BenchmarkHelloRequestMore(b *testing.B) {
	var idx int64 = 0
	b.ResetTimer()

	// 限制并发数
	var maxConcurrency = maxIdx
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{} // 获取一个信号量

			go func() {
				defer func() { <-sem }() // 释放信号量
				v := atomic.AddInt64(&idx, 1)
				actorRef := testSystem.actors[v%maxIdx]
				reply := actor.Request[*testpb.HelloReply](testSystem.system, actorRef, &testpb.HelloRequest{Name: "hello go request"})
				if reply == nil {
					b.Error()
				}
			}()
		}
	})
	wg.Wait()
}
