package grain

import (
	"sync/atomic"

	"github.com/timandy/routine"
)

// executionGate owns actor execution authority. It is embedded by value in the
// scheduler and bound to BaseActor before publication; it must not be copied.
// Only the permit holder may access executingCall without an identity check.
type executionGate struct {
	executionPermit  chan struct{}
	executionOwnerID atomic.Uint64 // zero means no published authority
	executingCall    *execution
}

func (g *executionGate) initialize() {
	g.executionPermit = make(chan struct{}, 1)
	g.executionPermit <- struct{}{}
}

// enter acquires permission before publishing the pointer and then its owner.
func (g *executionGate) enter(call *execution) {
	<-g.executionPermit
	g.executingCall = call
	g.executionOwnerID.Store(call.goroutineID)
}

// leave revokes identity and clears the pointer before another caller can enter.
func (g *executionGate) leave() {
	g.executionOwnerID.Store(0)
	g.executingCall = nil
	g.executionPermit <- struct{}{}
}

// Failed identity checks must NEVER read executingCall: another goroutine may be
// updating it. A successful check proves that this is the permit holder's own
// goroutine, which cannot concurrently release its permission while in this call.
// This depends on routine.Goid identifying the actual, non-inherited goroutine.
func (g *executionGate) currentExecution() *execution {
	if g == nil {
		return nil
	}
	ownerID := g.executionOwnerID.Load()
	if ownerID == 0 || ownerID != routine.Goid() {
		return nil
	}
	return g.executingCall
}
