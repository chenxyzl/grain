package grain

import (
	"fmt"
	"github.com/chenxyzl/grain/message"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

type dispatchBenchmarkActor struct {
	BaseActor
	completed chan struct{}
}

func (*dispatchBenchmarkActor) Started() {}
func (*dispatchBenchmarkActor) PreStop() {}
func (a *dispatchBenchmarkActor) Receive(ctx Context) {
	if ctx.Message().(*message.Subscribe).EventName == "last" {
		a.completed <- struct{}{}
	}
}

// Unlike BenchmarkTellLocal this measures completion, never the overflow path.
func BenchmarkActorDispatch(b *testing.B) {
	for _, batch := range []int{1, 64} {
		b.Run(fmt.Sprintf("batch=%d", batch), func(b *testing.B) {
			sys := newTestSystemTB(b)
			actor := &dispatchBenchmarkActor{completed: make(chan struct{}, 1)}
			ref, err := sys.SpawnNamed(func() IActor { return actor }, "dispatch", WithOptsMailboxSize(128))
			if err != nil {
				b.Fatal(err)
			}
			defer sys.Poison(ref)
			ordinary, last := &message.Subscribe{}, &message.Subscribe{EventName: "last"}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				for i := 0; i < batch-1; i++ {
					ref.Tell(ordinary)
				}
				ref.Tell(last)
				<-actor.completed
			}
		})
	}
}

type cycleBenchmarkActor struct {
	BaseActor
	target ActorRef
	middle bool
}

func (*cycleBenchmarkActor) Started() {}
func (*cycleBenchmarkActor) PreStop() {}
func (a *cycleBenchmarkActor) Receive(ctx Context) {
	request := ctx.Message().(*message.Subscribe)
	if request.EventName == "callback" {
		ctx.Reply(request)
		return
	}
	next := "middle"
	if a.middle {
		next = "callback"
	}
	response, err := a.Ask[*message.Subscribe](a.target, &message.Subscribe{EventName: next})
	if err != nil {
		ctx.Reply(err)
	} else {
		ctx.Reply(response)
	}
}
func BenchmarkActorReentrantCycle(b *testing.B) {
	sys := newTestSystemTB(b)
	a, c := &cycleBenchmarkActor{}, &cycleBenchmarkActor{middle: true}
	first, err := sys.SpawnNamed(func() IActor { return a }, "first")
	if err != nil {
		b.Fatal(err)
	}
	second, err := sys.SpawnNamed(func() IActor { return c }, "second")
	if err != nil {
		b.Fatal(err)
	}
	a.target = second
	c.target = first
	defer sys.Poison(first)
	defer sys.Poison(second)
	request := &message.Subscribe{EventName: "outer"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := NoReentryAsk[*message.Subscribe](first, request); err != nil {
			b.Fatal(err)
		}
	}
}

// Run with -benchtime=1x: retained state after 10,000 actors have started and parked.
func BenchmarkIdleActorFootprint(b *testing.B) {
	const actors = 10000
	for range b.N {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		goroutinesBefore := runtime.NumGoroutine()
		sys := newTestSystemTB(b)
		var started atomic.Int32
		refs := make([]ActorRef, 0, actors)
		for i := range actors {
			ref, err := sys.SpawnNamed(func() IActor { return &footprintActor{started: &started} }, strconv.Itoa(i))
			if err != nil {
				b.Fatal(err)
			}
			refs = append(refs, ref)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			settled := started.Load() == actors
			if settled {
				for _, ref := range refs {
					if !schedulerIdle(sys.registry.get(ref).(*processorMailBox)) {
						settled = false
						break
					}
				}
			}
			if settled {
				break
			}
			if time.Now().After(deadline) {
				b.Fatal("actors did not park")
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/actors, "heap-B/actor")
		b.ReportMetric(float64(int64(after.StackInuse)-int64(before.StackInuse))/actors, "stack-B/actor")
		b.ReportMetric(float64(runtime.NumGoroutine()-goroutinesBefore), "idle-goroutines")
		for _, ref := range refs {
			sys.Poison(ref)
		}
		for sys.registry.lookup.Count() != 0 {
			if time.Now().After(deadline) {
				b.Fatal("actors failed to stop")
			}
			time.Sleep(time.Millisecond)
		}
	}
}

type footprintActor struct {
	BaseActor
	started *atomic.Int32
}

func (a *footprintActor) Started()      { a.started.Add(1) }
func (*footprintActor) Receive(Context) {}
func (*footprintActor) PreStop()        {}
