package grain

import (
	"sync/atomic"

	"github.com/timandy/routine"
)

type drainStatus int32

const (
	drainIdle drainStatus = iota
	drainRunning
	drainStopped
)

// mailboxScheduler owns scheduling and execution accounting, not a dedicated
// goroutine. One drainer runs Receive directly until the mailbox is empty or an
// Ask transfers draining to a successor. Suspended calls remain counted.
type mailboxScheduler struct {
	processor       *processorMailBox
	drainStatus     atomic.Int32
	unfinishedCalls atomic.Int32
	executionGate   executionGate
}

func (s *mailboxScheduler) initialize(p *processorMailBox) {
	s.processor = p
	s.executionGate.initialize()
}
func (s *mailboxScheduler) schedule() {
	if s.drainStatus.CompareAndSwap(int32(drainIdle), int32(drainRunning)) {
		go s.drain()
	}
}

func (s *mailboxScheduler) drain() {
	call := &execution{scheduler: s, goroutineID: routine.Goid()}
	// Also park or transfer responsibility when a user handler invokes Goexit.
	defer s.finishDraining(call)
	for s.drainStatus.Load() != int32(drainStopped) {
		ctx, ok := s.processor.mailbox.Pop()
		if !ok {
			if s.stopReady() {
				s.stopIfIdle(call)
			}
			return
		}
		s.invoke(call, ctx)
		if call.drainOwnershipTransferred {
			return
		}
	}
}
func (s *mailboxScheduler) invoke(call *execution, ctx Context) {
	s.unfinishedCalls.Add(1)
	s.executionGate.enter(call)
	defer func() {
		s.executionGate.leave()
		remaining := s.unfinishedCalls.Add(-1)
		// The successor may already have parked while this call awaited its reply.
		if call.drainOwnershipTransferred && remaining == 0 && s.processor.lifecycle.stopRequested.Load() {
			s.schedule()
		}
	}()
	call.invoke(ctx)
}
func (s *mailboxScheduler) stopReady() bool {
	return s.processor.lifecycle.stopRequested.Load() && s.unfinishedCalls.Load() == 0
}
func (s *mailboxScheduler) finishDraining(call *execution) {
	// The successor owns running -> idle after a handoff.
	if call.drainOwnershipTransferred {
		return
	}
	if !s.drainStatus.CompareAndSwap(int32(drainRunning), int32(drainIdle)) {
		return
	}
	// A send/Poison may have observed running just before we parked. Rechecking
	// after the CAS prevents a lost wakeup. Do not spin while calls are suspended.
	if s.processor.mailbox.Len() > 0 || s.stopReady() {
		s.schedule()
	}
}
func (s *mailboxScheduler) stopIfIdle(call *execution) {
	call.callKind = callStop
	s.executionGate.enter(call)
	defer s.executionGate.leave()
	if s.drainStatus.Load() == int32(drainStopped) || s.processor.mailbox.Len() != 0 || s.unfinishedCalls.Load() != 0 {
		return
	}
	// Lifecycle cleanup and its panic handling run on this drainer, with permission.
	call.stop()
}
