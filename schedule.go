package grain

import (
	"runtime"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	stateInit = iota
	stateReady
	stateDone
)

func startTimer(delay, interval time.Duration, fn func()) CancelScheduleFunc {
	var t *time.Timer
	var state int32
	t = time.AfterFunc(delay, func() {
		// AfterFunc can fire before the assignment to t completes for a zero/tiny delay
		for atomic.LoadInt32(&state) == stateInit {
			runtime.Gosched()
		}

		// must be atomic: the cancel func writes state from another goroutine
		if atomic.LoadInt32(&state) == stateDone {
			return
		}

		fn()

		// Re-check before re-arming, or a cancel racing fn() leaves the timer armed for one
		// more interval. Cancel only guarantees no FURTHER fn() runs, not that fn() has ended.
		if atomic.LoadInt32(&state) != stateDone {
			t.Reset(interval)
		}
	})

	atomic.StoreInt32(&state, stateReady)

	return func() {
		if atomic.SwapInt32(&state, stateDone) != stateDone {
			t.Stop()
		}
	}
}

type timerSchedule struct {
	sender iSender
}

func newTimerSchedule(sender iSender) *timerSchedule {
	s := &timerSchedule{sender: sender}
	return s
}

// sendOnce tells message to target once after delay, delivering the caller's instance as-is.
func (s *timerSchedule) sendOnce(target ActorRef, delay time.Duration, message proto.Message) CancelScheduleFunc {
	t := time.AfterFunc(delay, func() {
		s.sender.tell(target, message)
	})

	return func() { t.Stop() }
}

// sendRepeatedly tells message to target every interval, starting after initial. ALIASING: the
// same instance is sent every tick, never copied, so it is shared with the caller and the
// receiving actor — and a remote target means the write-stream actor marshals it, so a
// concurrent write is a real data race. See IScheduler for the safe patterns.
func (s *timerSchedule) sendRepeatedly(target ActorRef, initial, interval time.Duration, message proto.Message) CancelScheduleFunc {
	return startTimer(initial, interval, func() {
		s.sender.tell(target, message)
	})
}
