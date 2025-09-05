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
		for atomic.LoadInt32(&state) == stateInit {
			runtime.Gosched()
		}

		if state == stateDone {
			return
		}

		fn()
		t.Reset(interval)
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
