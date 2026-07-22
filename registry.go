package grain

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/chenxyzl/grain/al/safemap"
)

type registry struct {
	lookup   safemap.ConcurrentMap[string, iProcess]
	createMu sync.Mutex // serializes getOrAdd so the loser never builds a processor
	logger   *slog.Logger
}

func newRegistry(logger *slog.Logger) *registry {
	return &registry{
		lookup: safemap.NewStringC[iProcess](),
		logger: logger,
	}
}

func (r *registry) remove(actRef ActorRef) {
	r.lookup.Remove(actRef.GetId())
}

func (r *registry) get(actRef ActorRef) iProcess {
	proc, _ := r.lookup.Get(actRef.GetId())
	return proc
}

func (r *registry) add(iProcP iProcessProvider) iProcess {
	proc := iProcP()
	id := proc.self().GetId()
	_, ok := r.lookup.SetIfNotExist(id, proc)
	if ok {
		panic(fmt.Sprintf("duplicated process id, id: %s", id))
	}
	proc.init()
	return proc
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
func (r *registry) rangeIt(fun func(key string, v iProcess)) {
	r.lookup.IterCb(fun)
}
