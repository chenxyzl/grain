package grain

import (
	"testing"
	"time"
)

// stubProc is a minimal iProcess for registry-level tests.
type stubProc struct {
	ref      ActorRef
	poisoned bool
}

func (s *stubProc) self() ActorRef   { return s.ref }
func (s *stubProc) opts() *tOpts     { return nil }
func (s *stubProc) init()            {}
func (s *stubProc) send(ctx Context) {}
func (s *stubProc) poison()          { s.poisoned = true }

// TestRangeItCallbackMayReenterRegistry pins the fix for a deadlock that took down
// the whole shard.
//
// ConcurrentMap.IterCb holds a shard read lock while invoking its callback, and
// clusterMemberChanged's callback used to call system.Poison -> registry.get, which
// hashes to the SAME shard. sync.RWMutex read locks are not reentrant once a writer
// is queued, so any concurrent actor spawn/stop (registry.add/remove) wedged the
// iteration permanently — and it never released the shard lock, so every later
// lookup or spawn on that shard blocked too. Reproduced before the fix.
//
// rangeIt now snapshots first and calls back outside the lock, so re-entry is safe.
func TestRangeItCallbackMayReenterRegistry(t *testing.T) {
	reg := newRegistry()
	ref := newClusterActorRef("player", "p1", nil)
	reg.lookup.Set(ref.GetId(), &stubProc{ref: ref})

	inCallback := make(chan struct{})
	writerQueued := make(chan struct{})
	done := make(chan struct{})

	go func() {
		reg.rangeIt(func(key string, v iProcess) {
			close(inCallback)
			<-writerQueued   // a writer is now queued on this key's shard
			_ = reg.get(ref) // re-entrant read of the same shard
		})
		close(done)
	}()

	<-inCallback
	// Queue a writer on the same shard, as any actor stopping/spawning would.
	go func() { reg.remove(ref) }()
	time.Sleep(100 * time.Millisecond) // give Lock() time to enqueue
	close(writerQueued)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: rangeIt's callback blocked re-entering the registry — " +
			"the callback is running under the shard lock again")
	}
}

// TestRangeItVisitsEverything guards the snapshot itself: it must not drop or
// duplicate entries.
func TestRangeItVisitsEverything(t *testing.T) {
	reg := newRegistry()
	want := map[string]bool{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		ref := newClusterActorRef("player", name, nil)
		reg.lookup.Set(ref.GetId(), &stubProc{ref: ref})
		want[ref.GetId()] = true
	}

	got := map[string]int{}
	reg.rangeIt(func(key string, v iProcess) {
		got[key]++
		if v.self().GetId() != key {
			t.Errorf("key %q paired with process for %q", key, v.self().GetId())
		}
	})

	if len(got) != len(want) {
		t.Fatalf("visited %d entries, want %d", len(got), len(want))
	}
	for id := range want {
		if got[id] != 1 {
			t.Errorf("entry %q visited %d times, want exactly 1", id, got[id])
		}
	}
}
