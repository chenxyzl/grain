package grain

import (
	"fmt"
	"log/slog"

	"github.com/chenxyzl/grain/al/safemap"
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
func (r *registry) rangeIt(fun func(key string, v iProcess)) {
	r.lookup.IterCb(fun)
}
