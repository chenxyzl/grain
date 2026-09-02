package grain

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// Callback and cancel both touch `state`, so every access must be atomic. Needs -race.
func TestStartTimerCancelIsRaceFree(t *testing.T) {
	var fires atomic.Int32
	for range 200 {
		cancel := startTimer(time.Millisecond, time.Millisecond, func() {
			fires.Add(1)
		})
		// cancel while the callback is plausibly mid-flight, to hit the window
		time.Sleep(time.Millisecond)
		cancel()
	}
	time.Sleep(50 * time.Millisecond)
	t.Logf("callback fired %d times across 200 timers", fires.Load())
}

// The callback repeats on the interval and stops firing after cancel.
func TestStartTimerRepeatsThenStops(t *testing.T) {
	var fires atomic.Int32
	cancel := startTimer(10*time.Millisecond, 10*time.Millisecond, func() {
		fires.Add(1)
	})

	time.Sleep(120 * time.Millisecond)
	got := fires.Load()
	if got < 3 {
		t.Fatalf("repeated timer fired only %d times in 120ms, want >= 3", got)
	}

	cancel()
	// one in-flight tick may still complete; nothing may fire after that
	time.Sleep(20 * time.Millisecond)
	afterCancel := fires.Load()
	time.Sleep(150 * time.Millisecond)
	if final := fires.Load(); final != afterCancel {
		t.Errorf("timer fired %d more times after cancel settled (%d -> %d)",
			final-afterCancel, afterCancel, final)
	}
}

// cancel must be idempotent (the Swap guard).
func TestStartTimerCancelTwice(t *testing.T) {
	cancel := startTimer(time.Hour, time.Hour, func() {})
	cancel()
	cancel() // must not panic
}

// collector forwards the received message so a test can tell "same instance" from "a copy".
type collector struct {
	BaseActor
	got chan proto.Message
}

func (a *collector) Started() {}
func (a *collector) PreStop() {}
func (a *collector) Receive(ctx Context) {
	if _, ok := ctx.Message().(*message.Subscribe); ok {
		select {
		case a.got <- ctx.Message():
		default:
		}
	}
}

// Pins the aliasing IScheduler documents: every tick delivers the identical pointer, shared
// with the caller. If a per-tick copy is introduced this fails, and the docs must change too.
func TestScheduleRepeatedDeliversTheSameInstance(t *testing.T) {
	sys := newFakeSys()
	act := &collector{got: make(chan proto.Message, 8)}
	p := newTestProcessor(sys, act, 8)
	p.init()

	msg := &message.Subscribe{EventName: "tick"}
	cancel := newTimerSchedule(sys).sendRepeatedly(p.self(), time.Millisecond, 5*time.Millisecond, msg)
	defer cancel()

	for i := range 3 {
		select {
		case got := <-act.got:
			// pointer identity, not proto.Equal: the point is that it is not a copy
			if got != proto.Message(msg) {
				t.Fatalf("tick %d delivered a different instance (%p) than the one scheduled (%p); "+
					"if this is now a per-tick copy, IScheduler's aliasing warning is stale",
					i, got, msg)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d of 3 ticks arrived", i)
		}
	}

	// the consequence: a write by the handler or the caller is still set on the next tick
	msg.EventName = "mutated by the handler"
	select {
	case got := <-act.got:
		if got.(*message.Subscribe).EventName != "mutated by the handler" {
			t.Errorf("a mutation must be visible on the next tick, got %q",
				got.(*message.Subscribe).EventName)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no further tick arrived")
	}
}
