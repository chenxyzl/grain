package grain

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/ringbuffer"
	"github.com/chenxyzl/grain/message"
	"github.com/timandy/routine"
	"google.golang.org/protobuf/proto"
)

type schedulerTestActor struct {
	BaseActor
	onStart   func()
	onReceive func(Context)
	onStop    func()
}

func (a *schedulerTestActor) Started() {
	if a.onStart != nil {
		a.onStart()
	}
}
func (a *schedulerTestActor) Receive(ctx Context) {
	if a.onReceive != nil {
		a.onReceive(ctx)
	}
}
func (a *schedulerTestActor) PreStop() {
	if a.onStop != nil {
		a.onStop()
	}
}
func receiveTestValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for test event")
		var zero T
		return zero
	}
}

// Resolve from the immutable message context; the production BaseActor caches
// this scheduler during binding instead of paying for a registry lookup.
func currentTestExecution(ctx Context) *execution {
	p := ctx.Target().GetSystem().getRegistry().lookup(ctx.Target().GetId()).(*processorMailBox)
	return p.scheduler.executionGate.currentExecution()
}

func schedulerIdle(p *processorMailBox) bool {
	return p.scheduler.drainStatus.Load() == int32(drainIdle)
}

func TestSchedulerIdleRestartPreservesActorLifecycle(t *testing.T) {
	sys := newFakeSys()
	var starts, stops atomic.Int32
	processed := make(chan int, 1)
	count := 0
	actor := &schedulerTestActor{onStart: func() { starts.Add(1) }, onStop: func() { stops.Add(1) }, onReceive: func(ctx Context) { count++; processed <- count }}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	for i := 1; i <= 5; i++ {
		p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
		if got := receiveTestValue(t, processed); got != i {
			t.Fatalf("state lost: %d", got)
		}
		waitFor(t, func() bool { return schedulerIdle(p) }, "drainer to retire")
		if starts.Load() != 1 || stops.Load() != 0 {
			t.Fatal("idle retirement changed actor lifecycle")
		}
	}
	p.poison()
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "actor to stop")
	if starts.Load() != 1 || stops.Load() != 1 {
		t.Fatal("lifecycle hooks not paired exactly once")
	}
}

func TestSchedulerRetirementRacesSendAndPoison(t *testing.T) {
	sys := newFakeSys()
	var received atomic.Int32
	actor := &schedulerTestActor{onReceive: func(ctx Context) { received.Add(1) }}
	p := newTestProcessor(sys, actor, 1024)
	p.init()
	const senders, messages = 8, 100
	var wg sync.WaitGroup
	for range senders {
		wg.Go(func() {
			for range messages {
				p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
				runtime.Gosched()
			}
		})
	}
	wg.Wait()
	waitFor(t, func() bool { return received.Load() == senders*messages }, "all accepted messages")
	// Race a stop request with the last completion and idle retirement.
	p.poison()
	waitFor(t, func() bool {
		return p.lifecycleIs(actorStopped) && p.scheduler.drainStatus.Load() == int32(drainStopped)
	}, "stop after retirement race")
}
func TestSchedulerParksWhileCallSuspended(t *testing.T) {
	sys := newFakeSys()
	blocked := make(chan struct{})
	release := make(chan struct{})
	resumed := make(chan struct{})
	actor := &schedulerTestActor{onReceive: func(ctx Context) {
		call := currentTestExecution(ctx)
		call.suspend()
		close(blocked)
		<-release
		call.resume()
		close(resumed)
	}}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	receiveTestValue(t, blocked)
	waitFor(t, func() bool { return schedulerIdle(p) }, "successor to park while call is suspended")
	if p.scheduler.unfinishedCalls.Load() != 1 {
		t.Fatal("suspended call not counted")
	}
	p.poison()
	waitFor(t, func() bool { return schedulerIdle(p) }, "stop request to park without spinning")
	if p.scheduler.drainStatus.Load() == int32(drainStopped) {
		t.Fatal("stopped before call resumed")
	}
	close(release)
	receiveTestValue(t, resumed)
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "suspended call to drain")
}

func TestSchedulerGoexitAndPanicDoNotLoseCompletion(t *testing.T) {
	for _, exit := range []bool{false, true} {
		t.Run(fmt.Sprint(exit), func(t *testing.T) {
			sys := newFakeSys()
			done := make(chan struct{}, 1)
			calls := 0
			actor := &schedulerTestActor{onReceive: func(ctx Context) {
				calls++
				if calls == 1 {
					if exit {
						runtime.Goexit()
					}
					panic("handler failure")
				}
				done <- struct{}{}
			}}
			p := newTestProcessor(sys, actor, 8)
			p.init()
			for range 2 {
				p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
			}
			receiveTestValue(t, done)
			p.poison()
			waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "failed handler to complete")
		})
	}
}
func TestExecutionScopeSurvivesProfilerLabels(t *testing.T) {
	sys := newFakeSys()
	result := make(chan bool, 1)
	actor := &schedulerTestActor{onReceive: func(ctx Context) {
		call := currentTestExecution(ctx)
		pprof.Do(context.Background(), pprof.Labels("actor", "test"), func(context.Context) { result <- currentTestExecution(ctx) == call })
	}}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	if !receiveTestValue(t, result) {
		t.Fatal("pprof labels lost the execution scope")
	}
	p.poison()
}

// A send-path panic after yielding must wait for the inserted handler to finish
// before any user defer/recover can access actor state.
type panicDeliverySystem struct {
	*fakeSys
	deliver func()
}

func (s *panicDeliverySystem) getSender() iSender { return s }
func (s *panicDeliverySystem) tellWithSender(ActorRef, proto.Message, ActorRef, uint64) {
	s.deliver()
	panic("delivery failure")
}
func TestAskPanicRestoresExecutionBeforeRecover(t *testing.T) {
	sys := newFakeSys()
	inserted := make(chan struct{})
	release := make(chan struct{})
	recovered := make(chan bool, 1)
	var actor *schedulerTestActor
	var p *processorMailBox
	targetSys := &panicDeliverySystem{fakeSys: sys}
	target := newDirectActorRef("local", "panic-target", "test", targetSys)
	state := 0
	actor = &schedulerTestActor{onReceive: func(ctx Context) {
		switch ctx.Message().(*message.Subscribe).EventName {
		case "outer":
			defer func() {
				failure := recover()
				recovered <- failure != nil && state == 2 && currentTestExecution(ctx) != nil
			}()
			state = 1
			_, _ = actor.Ask[*message.Subscribe](target, &message.Subscribe{})
		case "inserted":
			close(inserted)
			<-release
			state = 2
		}
	}}
	p = newTestProcessor(sys, actor, 8)
	targetSys.deliver = func() {
		p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "inserted"}, 0, sys))
		<-inserted
	}
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "outer"}, 0, sys))
	receiveTestValue(t, inserted)
	select {
	case <-recovered:
		t.Fatal("recover ran without execution permission")
	default:
	}
	close(release)
	if !receiveTestValue(t, recovered) {
		t.Fatal("recover did not observe restored actor state/scope")
	}
	p.poison()
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "panic recovery to drain")
	if sys.pending.Count() != 0 {
		t.Fatal("pending request leaked")
	}
}
func TestPendingRegistrationIsClosedAtomically(t *testing.T) {
	sys := newTestSystem(t)
	var wg sync.WaitGroup
	replies := make(chan chan proto.Message, 200)
	for range 200 {
		wg.Go(func() {
			id := sys.nextSnId()
			reply := sys.registerAsk(id)
			if reply != nil {
				replies <- reply
			}
		})
	}
	sys.wakePendingAsks()
	wg.Wait()
	close(replies)
	for reply := range replies {
		value := receiveTestValue(t, reply)
		err, ok := value.(*message.ErrCode)
		if !ok || !errors.Is(err, message.CodeSystemStopping) {
			t.Fatalf("unexpected shutdown reply: %v", value)
		}
	}
	if sys.registerAsk(sys.nextSnId()) != nil {
		t.Fatal("registration accepted after close")
	}
	target := newDirectActorRef("local", "absent", sys.addr, sys)
	if _, err := NoReentryAsk[*message.Subscribe](target, &message.Subscribe{}); !errors.Is(err, message.CodeSystemStopping) {
		t.Fatalf("unexpected Ask result: %v", err)
	}
}

func TestSchedulerResumeSurvivesFullMailbox(t *testing.T) {
	sys := newFakeSys()
	waiting := make(chan struct{})
	release := make(chan struct{})
	entered := make(chan struct{})
	finish := make(chan struct{})
	resumed := make(chan struct{})
	actor := &schedulerTestActor{onReceive: func(ctx Context) {
		switch ctx.Message().(*message.Subscribe).EventName {
		case "ask":
			call := currentTestExecution(ctx)
			call.suspend()
			close(waiting)
			<-release
			call.resume()
			close(resumed)
		case "block":
			close(entered)
			<-finish
		}
	}}
	p := newTestProcessor(sys, actor, 1)
	startup, _ := p.mailbox.Pop()
	p.mailbox = ringbuffer.New[Context](1, 1)
	p.mailbox.Push(startup)
	p.init()
	waitFor(t, func() bool { return p.lifecycleIs(actorStarted) }, "startup to leave the capacity-one mailbox")
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "ask"}, 0, sys))
	receiveTestValue(t, waiting)
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "block"}, 0, sys))
	receiveTestValue(t, entered)
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "queued"}, 0, sys))
	close(release) // resume waits for execution permission independently of the full mailbox
	p.poison()
	close(finish)
	receiveTestValue(t, resumed)
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "full mailbox and suspended call to drain")
}
func TestAskWithAnotherActorsBaseIsRejected(t *testing.T) {
	sys := newFakeSys()
	other := &schedulerTestActor{}
	otherProcessor := newTestProcessor(sys, other, 8)
	otherProcessor.init()
	result := make(chan *message.ErrCode, 1)
	actor := &schedulerTestActor{onReceive: func(ctx Context) {
		_, err := other.Ask[*message.Subscribe](other.Self(), &message.Subscribe{})
		result <- err
	}}
	// Use a different identity; the helper's default name is reused by ordinary tests.
	options := newOpts(func() IActor { return actor }, withOptsDirectSelf("second", "test", sys))
	p := newTestProcessorWithOpts(sys, actor, 8, options)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	if err := receiveTestValue(t, result); !errors.Is(err, message.CodeAskInvalidContext) {
		t.Fatalf("wrong actor accepted: %v", err)
	}
	p.poison()
	otherProcessor.poison()
}
func TestSchedulerStartedFailureCleansUpOnce(t *testing.T) {
	for _, goexit := range []bool{false, true} {
		t.Run(fmt.Sprint(goexit), func(t *testing.T) {
			sys := newFakeSys()
			var unregisters, stops, receives atomic.Int32
			actor := &schedulerTestActor{onStart: func() {
				if goexit {
					runtime.Goexit()
				}
				panic("start failed")
			}, onStop: func() { stops.Add(1) }, onReceive: func(ctx Context) { receives.Add(1) }}
			opts := newOpts(func() IActor { return actor }, withOptsDirectSelf("start-failure", "test", sys))
			opts.registerToCluster = func(iProvider, *config, ActorRef) error { return nil }
			opts.unRegisterFromCluster = func(iProvider, *config, ActorRef) error { unregisters.Add(1); return nil }
			p := newTestProcessorWithOpts(sys, actor, 8, opts)
			p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
			waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "failed Started cleanup")
			if unregisters.Load() != 1 || stops.Load() != 0 || receives.Load() != 0 {
				t.Fatal("startup failure violated lifecycle cleanup")
			}
		})
	}
}

// PreStop failures must not skip deregistration or strand execution permission.
func TestSchedulerPreStopFailureCleansUpOnce(t *testing.T) {
	for _, mode := range []string{"panic", "goexit"} {
		t.Run(mode, func(t *testing.T) {
			sys := newFakeSys()
			started := make(chan struct{})
			stopping := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			t.Cleanup(unblock)
			var registers, unregisters, stops atomic.Int32
			actor := &schedulerTestActor{
				onStart: func() { close(started) },
				onStop: func() {
					if stops.Add(1) == 1 {
						close(stopping)
					}
					<-release
					if mode == "goexit" {
						runtime.Goexit()
					}
					panic("PreStop failed")
				},
			}
			opts := newOpts(func() IActor { return actor }, withOptsDirectSelf("stop-failure", "test", sys))
			opts.registerToCluster = func(iProvider, *config, ActorRef) error { registers.Add(1); return nil }
			opts.unRegisterFromCluster = func(iProvider, *config, ActorRef) error { unregisters.Add(1); return nil }
			p := newTestProcessorWithOpts(sys, actor, 8, opts)
			p.init()
			receiveTestValue(t, started)
			p.poison()
			receiveTestValue(t, stopping)
			for range 10 {
				p.poison()
			}
			unblock()
			waitFor(t, func() bool {
				return p.scheduler.drainStatus.Load() == int32(drainStopped)
			}, "failed PreStop to stop draining")

			// stopped is published before the deferred gate cleanup, so acquire
			// with a timeout and inspect the plain fields under the permit.
			gate := &p.scheduler.executionGate
			select {
			case <-gate.executionPermit:
				lifecycle := p.lifecycle.lifecycleState
				owner := gate.executionOwnerID.Load()
				retained := gate.executingCall != nil
				gate.executionPermit <- struct{}{}
				if lifecycle != actorStopped || owner != 0 || retained {
					t.Fatalf("incomplete cleanup: lifecycle=%v owner=%d retained=%v", lifecycle, owner, retained)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("PreStop failure stranded execution permission")
			}
			for range 10 {
				p.poison()
			}
			if sys.reg.lookup(p.self().GetId()) != nil {
				t.Fatal("stopped actor remained registered")
			}
			if p.scheduler.drainStatus.Load() != int32(drainStopped) || p.scheduler.unfinishedCalls.Load() != 0 {
				t.Fatal("stopped actor restarted or retained unfinished calls")
			}
			if registers.Load() != 1 || unregisters.Load() != 1 || stops.Load() != 1 {
				t.Fatalf("cleanup must run once: register=%d unregister=%d PreStop=%d", registers.Load(), unregisters.Load(), stops.Load())
			}
		})
	}
}

func TestExecutionScopeIsNotInheritedByRoutineGo(t *testing.T) {
	sys := newFakeSys()
	result := make(chan bool, 1)
	actor := &schedulerTestActor{onReceive: func(ctx Context) { routine.Go(func() { result <- currentTestExecution(ctx) == nil }) }}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	if !receiveTestValue(t, result) {
		t.Fatal("child inherited actor authority")
	}
	p.poison()
}

func TestSchedulerIdleClearsExecutionOwner(t *testing.T) {
	sys := newFakeSys()
	done := make(chan struct{}, 1)
	actor := &schedulerTestActor{onReceive: func(Context) { done <- struct{}{} }}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	receiveTestValue(t, done)
	waitFor(t, func() bool { return schedulerIdle(p) }, "drainer to park")
	if p.scheduler.executionGate.executionOwnerID.Load() != 0 {
		t.Fatal("idle actor retained an owner")
	}
	// Inspect cleanup without enter overwriting the pointer being checked.
	<-p.scheduler.executionGate.executionPermit
	retained := p.scheduler.executionGate.executingCall != nil
	p.scheduler.executionGate.executionPermit <- struct{}{}
	if retained {
		t.Fatal("idle actor retained an execution scope")
	}
	p.poison()
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "actor cleanup")
}

func TestAskRoundTripReentryRepeated(t *testing.T) {
	sys := newTestSystem(t)
	firstActor, secondActor := &cycleBenchmarkActor{}, &cycleBenchmarkActor{middle: true}
	first, err := sys.SpawnNamed(func() IActor { return firstActor }, "cycle-first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := sys.SpawnNamed(func() IActor { return secondActor }, "cycle-second")
	if err != nil {
		t.Fatal(err)
	}
	firstActor.target = second
	secondActor.target = first
	for range 50 {
		response, err := NoReentryAsk[*message.Subscribe](first, &message.Subscribe{EventName: "outer"})
		if err != nil || response.EventName != "callback" {
			t.Fatalf("cycle failed: response=%v err=%v", response, err)
		}
	}
	sys.Poison(first)
	sys.Poison(second)
	waitFor(t, func() bool {
		return sys.registry.lookup(first.GetId()) == nil && sys.registry.lookup(second.GetId()) == nil
	}, "cycle actors to stop")
}

func TestAskRepeatedWithinOneHandler(t *testing.T) {
	sys := newFakeSys()
	done := make(chan *message.ErrCode, 1)
	var actor *schedulerTestActor
	actor = &schedulerTestActor{onReceive: func(ctx Context) {
		if ctx.Message().(*message.Subscribe).EventName == "inner" {
			ctx.Reply(&message.Subscribe{})
			return
		}
		for range 10 {
			if _, err := actor.Ask[*message.Subscribe](actor.Self(), &message.Subscribe{EventName: "inner"}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "outer"}, 0, sys))
	if err := receiveTestValue(t, done); err != nil {
		t.Fatal(err)
	}
	p.poison()
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "repeated Ask to drain")
}

func TestExecutionOwnerRejectsConcurrentCallers(t *testing.T) {
	sys := newFakeSys()
	var processed, invalidOwners atomic.Int32
	var actor *schedulerTestActor
	actor = &schedulerTestActor{onReceive: func(Context) {
		if actor.executionGate.currentExecution() == nil {
			invalidOwners.Add(1)
		}
		processed.Add(1)
	}}
	p := newTestProcessor(sys, actor, 1024)
	p.init()
	const messages = 1000
	var callers sync.WaitGroup
	var accepted atomic.Int32
	for range 8 {
		callers.Go(func() {
			for range messages {
				_, err := actor.Ask[*message.Subscribe](actor.Self(), &message.Subscribe{})
				if !errors.Is(err, message.CodeAskInvalidContext) {
					accepted.Add(1)
				}
				runtime.Gosched()
			}
		})
	}
	for range messages {
		p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	}
	callers.Wait()
	waitFor(t, func() bool { return processed.Load() == messages }, "legitimate calls to finish")
	if accepted.Load() != 0 || invalidOwners.Load() != 0 {
		t.Fatalf("identity errors: external accepted=%d legitimate rejected=%d", accepted.Load(), invalidOwners.Load())
	}
	p.poison()
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "concurrent callers cleanup")
}

func TestExecutionOwnerRevokedDuringAskSend(t *testing.T) {
	sys := newFakeSys()
	result := make(chan *message.ErrCode, 1)
	var actor *schedulerTestActor
	targetSystem := &panicDeliverySystem{fakeSys: sys}
	target := newDirectActorRef("local", "send-callback", "test", targetSystem)
	actor = &schedulerTestActor{onReceive: func(Context) {
		defer func() { _ = recover() }()
		_, _ = actor.Ask[*message.Subscribe](target, &message.Subscribe{})
	}}
	targetSystem.deliver = func() {
		// This callback runs on the suspended handler's goroutine, before it resumes.
		// Its Goid is unchanged, so revoking owner identity is essential.
		_, err := actor.Ask[*message.Subscribe](actor.Self(), &message.Subscribe{})
		result <- err
	}
	p := newTestProcessor(sys, actor, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{}, 0, sys))
	if err := receiveTestValue(t, result); !errors.Is(err, message.CodeAskInvalidContext) {
		t.Fatalf("suspended sender retained authority: %v", err)
	}
	p.poison()
	waitFor(t, func() bool { return p.lifecycleIs(actorStopped) }, "suspended sender cleanup")
}

// Reply, cancellation and shutdown may race without closing reply channels or
// leaving an active waiter stranded. Cancellation intentionally needs no reply.
func TestPendingShutdownRacesReplyAndCancel(t *testing.T) {
	sys := newTestSystem(t)
	const requests = 256
	type request struct {
		id    uint64
		reply chan proto.Message
	}
	pending := make([]request, requests)
	for i := range pending {
		id := sys.nextSnId()
		pending[i] = request{id, sys.registerAsk(id)}
	}
	start := make(chan struct{})
	response := &message.Subscribe{EventName: "reply"}
	var workers sync.WaitGroup
	for i, req := range pending {
		workers.Go(func() {
			<-start
			switch i % 3 {
			case 0:
				sys.deliverReply(req.id, response)
			case 1:
				sys.cancelAsk(req.id)
				// Remaining requests must be woken by shutdown.
			}
		})
	}
	workers.Go(func() { <-start; sys.wakePendingAsks() })
	close(start)
	workers.Wait()
	for i, req := range pending {
		if i%3 != 1 {
			value := receiveTestValue(t, req.reply)
			if value != response {
				err, ok := value.(*message.ErrCode)
				if !ok || !errors.Is(err, message.CodeSystemStopping) {
					t.Fatalf("unexpected shutdown/reply result: %v", value)
				}
			} else if i%3 == 2 {
				t.Fatal("request without a sender received a normal reply")
			}
		}
		sys.cancelAsk(req.id)
		// A late/duplicate reply after cleanup must be harmless.
		sys.deliverReply(req.id, response)
	}
	if sys.pending.Count() != 0 {
		t.Fatal("pending requests leaked after shutdown cleanup")
	}
	if sys.registerAsk(sys.nextSnId()) != nil {
		t.Fatal("registration reopened after shutdown")
	}
}
