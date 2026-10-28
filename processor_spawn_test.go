package grain

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestProcessorSpawnPoliciesShareCreationReservation(t *testing.T) {
	sys := newTestSystem(t)
	var builds, starts atomic.Int32
	opts := newOpts(func() IActor {
		builds.Add(1)
		return &schedulerTestActor{onStart: func() { starts.Add(1) }}
	}, withOptsDirectSelf("shared-spawn", sys.addr, sys))
	results := make(chan iProcess, 64)
	failures := make(chan error, 64)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < 64; i++ {
		workers.Go(func() {
			<-start
			if i%2 == 0 {
				results <- getOrSpawnProcessor(sys, opts)
				return
			}
			proc, err := spawnNewProcessor(sys, opts)
			if err != nil {
				if !errors.Is(err, ErrNameExists) {
					failures <- err
				}
				return
			}
			results <- proc
		})
	}
	close(start)
	workers.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Errorf("unexpected spawn error: %v", err)
	}
	winner := sys.registry.lookup(opts._self.GetId())
	if winner == nil {
		t.Fatal("nothing registered")
	}
	defer winner.poison()
	for proc := range results {
		if proc != winner {
			t.Fatal("returned different instances for the same ID")
		}
	}
	waitFor(t, func() bool { return starts.Load() == 1 }, "single startup")
	if builds.Load() != 1 {
		t.Fatalf("producer called %d times", builds.Load())
	}
	creator := sys.getProcessorCreator()
	creator.mu.Lock()
	active := len(creator.active)
	creator.mu.Unlock()
	if active != 0 {
		t.Fatal("creation reservations leaked")
	}
}

func TestProcessorCreationOfDifferentIDsRunsConcurrently(t *testing.T) {
	sys := newTestSystem(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	results := make(chan iProcess, 2)
	for _, id := range []string{"independent-a", "independent-b"} {
		opts := newOpts(func() IActor { entered <- struct{}{}; <-release; return &schedulerTestActor{} }, withOptsDirectSelf(id, sys.addr, sys))
		go func() { results <- getOrSpawnProcessor(sys, opts) }()
	}
	receiveTestValue(t, entered)
	receiveTestValue(t, entered) // both producers must run before either is released
	unblock()
	for range 2 {
		receiveTestValue(t, results).poison()
	}
}

func TestProcessorCreationFailureReleasesReservation(t *testing.T) {
	for _, mode := range []string{"panic", "goexit"} {
		t.Run(mode, func(t *testing.T) {
			sys := newTestSystem(t)
			opts := newOpts(func() IActor {
				if mode == "goexit" {
					runtime.Goexit()
				}
				panic("producer failed")
			}, withOptsDirectSelf("retry-creation", sys.addr, sys))
			done := make(chan struct{})
			go func() { defer close(done); defer func() { _ = recover() }(); getOrSpawnProcessor(sys, opts) }()
			receiveTestValue(t, done)
			if sys.registry.lookup(opts._self.GetId()) != nil {
				t.Fatal("failed producer published a processor")
			}
			opts.producer = func() IActor { return &schedulerTestActor{} }
			result := make(chan iProcess, 1)
			go func() { result <- getOrSpawnProcessor(sys, opts) }()
			receiveTestValue(t, result).poison()
			creator := sys.getProcessorCreator()
			creator.mu.Lock()
			active := len(creator.active)
			creator.mu.Unlock()
			if active != 0 {
				t.Fatal("creation reservation leaked after retry")
			}
		})
	}
}

type registrationProbe struct {
	stubProc
	starts int
}

func (p *registrationProbe) init() { p.starts++ }
func TestRegistryOnlyPublishesAndRemovesExpectedInstance(t *testing.T) {
	reg := newRegistry()
	ref := newClusterActorRef("player", "registry-only", nil)
	first := &registrationProbe{stubProc: stubProc{ref: ref}}
	second := &registrationProbe{stubProc: stubProc{ref: ref}}
	actual, inserted := reg.register(ref.GetId(), first)
	if actual != first || !inserted || first.starts != 0 {
		t.Fatal("register must publish without starting")
	}
	actual, inserted = reg.register(ref.GetId(), second)
	if actual != first || inserted || second.starts != 0 {
		t.Fatal("register replaced or started a duplicate")
	}
	reg.remove(ref.GetId(), first)
	reg.register(ref.GetId(), second)
	reg.remove(ref.GetId(), first)
	if reg.lookup(ref.GetId()) != second {
		t.Fatal("stale cleanup removed replacement")
	}
}
