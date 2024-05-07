package actor

import (
	"context"
	"github.com/chenxyzl/grain/utils/al/safemap"
)

type Registry struct {
	lookup safemap.ConcurrentMap[string, iProcess]
	system *System
}

func newRegistry(s *System) *Registry {
	return &Registry{
		lookup: safemap.NewStringC[iProcess](),
		system: s,
	}
}

func (r *Registry) remove(actRef *ActorRef) {
	//todo  cluster provider
	r.lookup.Remove(actRef.GetId())
}

func (r *Registry) get(actRef *ActorRef) iProcess {
	proc, _ := r.lookup.Get(actRef.GetId())
	return proc
}

func (r *Registry) add(proc iProcess) iProcess {
	//todo  cluster provider
	id := proc.self().GetId()
	if old, ok := r.lookup.Set(id, proc); ok {
		//force to stop old proc
		old.send(newContext(proc.self(), nil, messageDef.poison, 0, context.Background()))
		r.system.Logger().Error("duplicated process id, force poison old processor", "id", id)
	}
	return proc
}
