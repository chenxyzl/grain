package main

import (
	"examples/testpb"
	"fmt"
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
	mailboxSize       = 1024
	testSystem        = TestSystem{}
	idx         int64 = 0
	parallelism       = 32
	body              = "hello world"
	helloSend         = &testpb.Hello{Name: body}
	helloAsk          = &testpb.HelloAsk{Name: body}
	helloReply        = &testpb.HelloReply{Name: "hell go reply"}
	askTimeout        = time.Second * 1

	// deadLetters counts drops: mailbox at max capacity, or the target actor already stopped.
	deadLetters       atomic.Int64
	deadLetterSampled atomic.Bool // capture one sample to print its reason
	deadLetterReason  atomic.Value
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
	//actor.InitLog("./test.log")
	slog.SetLogLoggerLevel(slog.LevelWarn)
	testSystem.system = grain.NewSystem("hello", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigAskTimeout(askTimeout),
		grain.WithConfigDeadLetter(func(dl grain.DeadLetter) {
			deadLetters.Add(1)
			if deadLetterSampled.CompareAndSwap(false, true) {
				deadLetterReason.Store(dl.Reason)
			}
		}))
	testSystem.system.Logger().Warn("system starting")
	testSystem.system.Start()
	testSystem.system.Logger().Warn("system started successfully")

	for i := int64(0); i < actorCount; i++ {
		actorRef := testSystem.system.Spawn(func() grain.IActor { return &HelloActor{} }, spawnOpts()...)
		testSystem.actors = append(testSystem.actors, actorRef)
	}
}

func spawnOpts() []grain.KindOptFunc {
	return []grain.KindOptFunc{grain.WithOptsMailboxSize(mailboxSize)}
}
func BenchmarkSendOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() grain.IActor { return &HelloActor{} }, spawnOpts()...)
	b.ResetTimer()
	for range b.N {
		actorRef.Tell(helloSend)
	}
}
func BenchmarkSendMore(b *testing.B) {
	b.ResetTimer()
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % actorCount
			_ = v
			actorRef := testSystem.actors[v]
			actorRef.Tell(helloSend)
		}
	})
}
func BenchmarkAskOne(b *testing.B) {
	actorRef := testSystem.system.Spawn(func() grain.IActor { return &HelloActor{} }, spawnOpts()...)
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

func TestMain(m *testing.M) {
	testSystem.system.Logger().Info("test init")
	exitCode := m.Run()
	// non-zero means the send rate outran consumption and filled the mailbox
	if n := deadLetters.Load(); n > 0 {
		reason, _ := deadLetterReason.Load().(string)
		fmt.Printf("[deadletter] total=%d sampleReason=%q (mailboxSize=%d)\n", n, reason, mailboxSize)
	} else {
		fmt.Printf("[deadletter] total=0 (no overflow; mailboxSize=%d)\n", mailboxSize)
	}
	testSystem.system.Logger().Info("test end with code:" + strconv.Itoa(exitCode))
	testSystem.system.ForceStop(nil)
	testSystem.system.Logger().Info("test exit")
}
