package grain

import "github.com/chenxyzl/grain/al/safemap"

// registry owns only the actor ID index. Creation and startup belong to the spawner.
type registry struct {
	processes safemap.ConcurrentMap[string, iProcess]
}

func newRegistry() *registry { return &registry{processes: safemap.NewStringC[iProcess]()} }

func (r *registry) lookup(id string) iProcess {
	proc, _ := r.processes.Get(id)
	return proc
}

// register atomically publishes proc or returns the existing instance. It never
// constructs or starts a processor; only the inserting caller may start it.
func (r *registry) register(id string, proc iProcess) (actual iProcess, inserted bool) {
	actual, existed := r.processes.SetIfNotExist(id, proc)
	if existed {
		return actual, false
	}
	return proc, true
}

// remove cannot evict a newer activation that has reused the same actor ID.
func (r *registry) remove(id string, expected iProcess) {
	r.processes.RemoveCb(id, func(_ string, actual iProcess, exists bool) bool {
		return exists && actual == expected
	})
}

// rangeIt invokes fun once per registered process. It MUST snapshot first and call fun
// outside the shard lock, so fun may re-enter the registry (lookup/register/remove/Poison): IterCb
// holds a shard read lock, and RWMutex read locks are not reentrant once a writer is queued,
// so a nested lookup on the same shard would wedge it. The snapshot may therefore hold an
// already-stopped process — every operation callers do on it must be idempotent.
func (r *registry) rangeIt(fun func(key string, v iProcess)) {
	type entry struct {
		key  string
		proc iProcess
	}
	var snapshot []entry
	r.processes.IterCb(func(key string, v iProcess) {
		snapshot = append(snapshot, entry{key, v})
	})
	for _, e := range snapshot {
		fun(e.key, e.proc)
	}
}
