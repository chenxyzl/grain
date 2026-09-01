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

// newRegistry takes no logger: the registry never logged. It used to be handed one at
// NewSystem time and store it unread, which made it look like a third place the logger
// had to be threaded through — and one that captured slog.Default() before InitLog could
// plausibly have run.
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
//
// It used to panic, which killed the whole process for what is a caller mistake (or a
// benign race on respawn) — see SpawnNamed.
func (r *registry) add(iProcP iProcessProvider) (iProcess, error) {
	proc := iProcP()
	id := proc.self().GetId()
	if _, exists := r.lookup.SetIfNotExist(id, proc); exists {
		return nil, fmt.Errorf("%w: %s", ErrNameExists, id)
	}
	proc.init()
	return proc, nil
}

// getOrAdd is the idempotent variant of add: if a process with the given id
// already exists it returns the existing one, otherwise it builds, adds and
// inits a new one. Unlike add it never panics on a duplicate id — used for
// internal system actors (write_stream / cluster kinds) that may be spawned
// concurrently by many senders.
//
// The builder is only invoked when the process must actually be created: a
// lock-free fast lookup handles the common "already exists" case, and a create
// mutex serializes the slow path so a losing racer returns the existing process
// without ever building a throwaway one.
func (r *registry) getOrAdd(id string, build iProcessProvider) iProcess {
	if proc, ok := r.lookup.Get(id); ok {
		return proc
	}
	r.createMu.Lock()
	defer r.createMu.Unlock()
	// double-check under the create lock: a concurrent getOrAdd may have created
	// it between our fast lookup and acquiring the lock.
	if proc, ok := r.lookup.Get(id); ok {
		return proc
	}
	proc := build()
	if old, existed := r.lookup.SetIfNotExist(id, proc); existed {
		// Lost a race with a non-getOrAdd creation path (add): drop the proc we
		// just built WITHOUT init — it never started a run loop, so its queued
		// initialize message is GC'd with it and there are no side effects.
		return old
	}
	proc.init()
	return proc
}
// rangeIt invokes fun once per registered process.
//
// It snapshots first and calls fun OUTSIDE the registry lock, which is what makes
// it safe for fun to touch the registry again (get / add / remove / Poison).
// Iterating in place would not be: ConcurrentMap.IterCb holds a shard read lock
// while invoking the callback, and a nested lookup of the same key hits the same
// shard — sync.RWMutex read locks are not reentrant once a writer is queued, so any
// concurrent actor spawn/stop would wedge the iteration and the shard with it.
//
// The snapshot may therefore contain a process that stopped in the meantime; every
// operation callers perform on it (poison in particular) is idempotent.
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
