package actor

import (
	"google.golang.org/protobuf/proto"
	"runtime"
	"sync/atomic"
	"time"
)

type CancelScheduleFunc func()

type SenderFunc func(*ActorRef, proto.Message, ...uint64)

type Stopper interface {
	Stop()
}

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
	sender SenderFunc
}

func newTimerSchedule(sender SenderFunc) *timerSchedule {
	s := &timerSchedule{sender: sender}
	return s
}

func (s *timerSchedule) sendOnce(target *ActorRef, delay time.Duration, message proto.Message) CancelScheduleFunc {
	t := time.AfterFunc(delay, func() {
		s.sender(target, message)
	})

	return func() { t.Stop() }
}

func (s *timerSchedule) sendRepeatedly(target *ActorRef, initial, interval time.Duration, message proto.Message) CancelScheduleFunc {
	return startTimer(initial, interval, func() {
		s.sender(target, message)
	})
}
