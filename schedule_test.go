package grain

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestStartTimerCancelIsRaceFree pins the fix for a data race at schedule.go:25:
// the fired callback read `state` non-atomically while the cancel func wrote it with
// atomic.SwapInt32. Run under -race; the detector flagged it before the fix.
func TestStartTimerCancelIsRaceFree(t *testing.T) {
	var fires atomic.Int32
	for range 200 {
		cancel := startTimer(time.Millisecond, time.Millisecond, func() {
			fires.Add(1)
		})
		// Cancel while the callback is plausibly mid-flight, to hit the window.
		time.Sleep(time.Millisecond)
		cancel()
	}
	time.Sleep(50 * time.Millisecond)
	t.Logf("callback fired %d times across 200 timers", fires.Load())
}

// TestStartTimerRepeatsThenStops checks the behaviour the race fix must preserve:
// the callback repeats on the interval, and stops firing after cancel.
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
	// One in-flight tick may still complete; nothing may fire after that.
	time.Sleep(20 * time.Millisecond)
	afterCancel := fires.Load()
	time.Sleep(150 * time.Millisecond)
	if final := fires.Load(); final != afterCancel {
		t.Errorf("timer fired %d more times after cancel settled (%d -> %d)",
			final-afterCancel, afterCancel, final)
	}
}

// TestStartTimerCancelTwice: cancel must be idempotent (the Swap guard).
func TestStartTimerCancelTwice(t *testing.T) {
	cancel := startTimer(time.Hour, time.Hour, func() {})
	cancel()
	cancel() // must not panic
}
