package actor

import (
	"github.com/chenxyzl/grain/utils/al/safemap"
	"log/slog"
)

type registry struct {
	lookup safemap.ConcurrentMap[string, iProcess]
	logger *slog.Logger
}

func newRegistry(logger *slog.Logger) *registry {
	return &registry{
		lookup: safemap.NewStringC[iProcess](),
		logger: logger,
	}
}

func (r *registry) remove(actRef *ActorRef) {
	r.lookup.Remove(actRef.GetId())
}

func (r *registry) get(actRef *ActorRef) iProcess {
	proc, _ := r.lookup.Get(actRef.GetId())
	return proc
}

func (r *registry) add(proc iProcess) iProcess {
	id := proc.self().GetId()
	old, ok := r.lookup.SetIfNotExist(id, proc)
	if ok {
		r.logger.Warn("duplicated process id, ignore new processor, return old processor", "id", id)
		return old
	}
	proc.init()
	return proc
}
func (r *registry) rangeIt(fun func(key string, v iProcess)) {
	r.lookup.IterCb(fun)
}
