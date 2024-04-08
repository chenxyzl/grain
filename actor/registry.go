package actor

import (
	"errors"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	lookup map[string]process
	system *System
}

func newRegistry(s *System) *Registry {
	return &Registry{
		lookup: make(map[string]process, 1024),
		system: s,
	}
}

func (r *Registry) GetActorRef(kind, id string) ActorRef {
	proc := r.getByID(kind + "/" + id)
	if proc != nil {
		return proc.Self()
	}
	return nil
}

func (r *Registry) Remove(ref ActorRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lookup, ref.id())
}

func (r *Registry) get(ref ActorRef) process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if proc, ok := r.lookup[ref.id()]; ok {
		return proc
	}
	return nil
}

func (r *Registry) getByID(id string) process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookup[id]
}

func (r *Registry) add(proc process) error {
	r.mu.Lock()
	id := proc.Self().id()
	if _, ok := r.lookup[id]; ok {
		r.mu.Unlock()
		return errors.New("duplicate actor " + id)
	}
	r.lookup[id] = proc
	r.mu.Unlock()
	return nil
}
