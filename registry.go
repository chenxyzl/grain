package grain

import (
	"fmt"
	"sync"

	"github.com/chenxyzl/grain/al/safemap"
)

type registry struct {
	lookup   safemap.ConcurrentMap[string, iProcess]
	createMu sync.Mutex // serializes getOrAdd so the loser never builds a processor
}

func newRegistry() *registry {
	return &registry{
		lookup: safemap.NewStringC[iProcess](),
	}
}

func (r *registry) remove(actRef ActorRef) {
	r.lookup.Remove(actRef.GetId())
}

func (r *registry) get(actRef ActorRef) iProcess {
	proc, _ := r.lookup.Get(actRef.GetId())
	return proc
}

// add publishes a newly built process, failing with ErrNameExists if the id is taken.
// Never panics: a duplicate id is a caller mistake or a benign respawn race.
func (r *registry) add(iProcP iProcessProvider) (iProcess, error) {
	proc := iProcP()
	id := proc.self().GetId()
	if _, exists := r.lookup.SetIfNotExist(id, proc); exists {
		return nil, fmt.Errorf("%w: %s", ErrNameExists, id)
	}
	proc.init()
	return proc, nil
}

// getOrAdd is the idempotent variant of add: returns the existing process for id, else
// builds, adds and inits one. For internal system actors (write_stream / cluster kinds) that
// many senders may spawn concurrently. build is invoked only when a create is really needed:
// lock-free lookup for the common hit, createMu serializes the slow path.
func (r *registry) getOrAdd(id string, build iProcessProvider) iProcess {
	if proc, ok := r.lookup.Get(id); ok {
		return proc
	}
	r.createMu.Lock()
	defer r.createMu.Unlock()
	// a concurrent getOrAdd may have created it between the fast lookup and the lock
	if proc, ok := r.lookup.Get(id); ok {
		return proc
	}
	proc := build()
	if old, existed := r.lookup.SetIfNotExist(id, proc); existed {
		// Lost a race with add(): drop the proc just built WITHOUT init — it never started a
		// run loop, so it is GC'd with its queued initialize message and no side effects.
		return old
	}
	proc.init()
	return proc
}

// rangeIt invokes fun once per registered process. It MUST snapshot first and call fun
// outside the shard lock, so fun may re-enter the registry (get/add/remove/Poison): IterCb
// holds a shard read lock, and RWMutex read locks are not reentrant once a writer is queued,
// so a nested lookup on the same shard would wedge it. The snapshot may therefore hold an
// already-stopped process — every operation callers do on it must be idempotent.
func (r *registry) rangeIt(fun func(key string, v iProcess)) {
	type entry struct {
		key  string
		proc iProcess
	}
	var snapshot []entry
	r.lookup.IterCb(func(key string, v iProcess) {
		snapshot = append(snapshot, entry{key, v})
	})
	for _, e := range snapshot {
		fun(e.key, e.proc)
	}
}
