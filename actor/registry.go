package actor

import (
	"log/slog"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	lookup map[string]iProcess
	system *System
}

func newRegistry(s *System) *Registry {
	return &Registry{
		lookup: make(map[string]iProcess, 1024),
		system: s,
	}
}

func (r *Registry) GetProcessor(id *ActorRef) iProcess {
	proc := r.getByID(id.GetId())
	if proc == nil {
		//todo send to DeadLetter
		slog.Error("get actor by id fail", "id", id.GetId())
		return proc
	}
	return proc
}

func (r *Registry) Remove(id *ActorRef) {
	//todo remove from provider
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lookup, id.GetId())
}

func (r *Registry) get(ref *ActorRef) iProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if proc, ok := r.lookup[ref.GetId()]; ok {
		return proc
	}
	return nil
}

func (r *Registry) getByID(id string) iProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookup[id]
}

func (r *Registry) add(proc iProcess) iProcess {
	//todo register to provider
	r.mu.Lock()
	id := proc.self().GetIdentifier()
	if old, ok := r.lookup[id]; ok {
		r.mu.Unlock()
		r.system.Logger().Error("duplicated process id, ignore add", "id", id)
		return old
	}
	r.lookup[id] = proc
	r.mu.Unlock()
	return proc
}
