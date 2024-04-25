package actor

import (
	"context"
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

func (r *Registry) remove(actRef *ActorRef) {
	//todo  cluster provider
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lookup, actRef.GetId())
}

func (r *Registry) get(actRef *ActorRef) iProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if proc, ok := r.lookup[actRef.GetId()]; ok {
		return proc
	}
	return nil
}

func (r *Registry) add(proc iProcess) iProcess {
	//todo  cluster provider
	r.mu.Lock()
	id := proc.self().GetId()
	if old, ok := r.lookup[id]; ok {
		r.mu.Unlock()
		//force to stop old proc
		old.send(newContext(proc.self(), nil, Message.poison, context.Background()))
		r.system.Logger().Error("duplicated process id, ignore add", "id", id)
	}
	r.lookup[id] = proc
	r.mu.Unlock()
	return proc
}
