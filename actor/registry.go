package actor

import (
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
	r.lookup.Remove(actRef.GetFullIdentifier())
}

func (r *Registry) get(actRef *ActorRef) iProcess {
	proc, _ := r.lookup.Get(actRef.GetFullIdentifier())
	return proc
}

func (r *Registry) add(proc iProcess) iProcess {
	id := proc.self().GetFullIdentifier()
	old, ok := r.lookup.SetIfNotExist(id, proc)
	if ok {
		//force to stop old proc
		//old.send(newContext(proc.self(), nil, messageDef.poisonIgnoreRegistry, r.system.getNextSnId(), context.Background()))
		//r.system.Logger().Error("duplicated process id, force poison old processorMailBox", "id", id)
		r.system.Logger().Warn("duplicated process id, ignore new processor, return old processor", "id", id)
		return old
	}
	proc.init()
	return proc
}
