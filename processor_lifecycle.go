package grain

import "sync/atomic"

type actorLifecycle uint8

const (
	actorInitializing actorLifecycle = iota
	actorStarting
	actorStarted
	actorStopping
	actorStopped
)

// phase and registration ownership are accessed only with execution permission.
// stopRequested is an independent atomic latch because senders can request stop.
type actorLifecycleState struct {
	lifecycleState           actorLifecycle
	stopRequested            atomic.Bool
	clusterRegistrationOwned bool
}

func (l *actorLifecycleState) start(call *execution) {
	if l.lifecycleState != actorInitializing {
		return
	}
	p := call.scheduler.processor
	l.lifecycleState = actorStarting
	started := false
	// Registration or Started can panic/Goexit; both must still clean up resources.
	defer func() {
		if !started {
			l.stop(call)
		}
	}()
	if p.registerToCluster != nil {
		if err := p.registerToCluster(p.system.getProvider(), p.system.getConfig(), p.self()); err != nil {
			p.system.Logger().Error("register to cluster failed, stop self", "id", p.self(), "err", err)
			return
		}
		l.clusterRegistrationOwned = true
	}
	p.receiver.Started()
	l.lifecycleState = actorStarted
	started = true
}
func (l *actorLifecycleState) stop(call *execution) {
	if l.lifecycleState == actorStopped || l.lifecycleState == actorStopping {
		return
	}
	p := call.scheduler.processor
	runPreStop := l.lifecycleState == actorStarted
	l.lifecycleState = actorStopping
	call.callKind = callStop
	// Close admission before running cleanup. A racing Push is either retained
	// for dead-lettering below or refused immediately; no accepted message vanishes.
	p.mailbox.Close()
	defer func() {
		l.lifecycleState = actorStopped
		call.scheduler.drainStatus.Store(int32(drainStopped))
		p.system.getRegistry().remove(p.self().GetId(), p)
		for {
			ctx, ok := p.mailbox.Pop()
			if !ok {
				break
			}
			p.toDeadLetter(ctx, DeadLetterReasonStopped)
		}
	}()
	defer func() {
		if l.clusterRegistrationOwned && p.unRegisterFromCluster != nil {
			if err := p.unRegisterFromCluster(p.system.getProvider(), p.system.getConfig(), p.self()); err != nil {
				p.system.Logger().Warn("unregister from cluster failed, stale routing entry may remain", "id", p.self(), "err", err)
			}
		}
	}()
	if runPreStop {
		p.receiver.PreStop()
	}
}
