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
		// Wait until the constructor has published t (stateReady): AfterFunc can fire
		// before the assignment to t completes for a zero/tiny delay.
		for atomic.LoadInt32(&state) == stateInit {
			runtime.Gosched()
		}

		// Must be an atomic load: the cancel func below writes state with
		// atomic.SwapInt32 from another goroutine.
		if atomic.LoadInt32(&state) == stateDone {
			return
		}

		fn()

		// Re-check before re-arming, otherwise a cancel racing with fn() leaves the
		// timer armed for one more interval after Stop() already ran. Still a race in
		// the strict sense — cancel does not guarantee fn() has stopped, only that no
		// *further* fn() runs after the tick that observes stateDone.
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

func (s *timerSchedule) sendOnce(target ActorRef, delay time.Duration, message proto.Message) CancelScheduleFunc {
	t := time.AfterFunc(delay, func() {
		s.sender.tell(target, message)
	})

	return func() { t.Stop() }
}

func (s *timerSchedule) sendRepeatedly(target ActorRef, initial, interval time.Duration, message proto.Message) CancelScheduleFunc {
	return startTimer(initial, interval, func() {
		s.sender.tell(target, message)
	})
}
