package actor

import (
	"errors"
	"log/slog"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	lookup map[string]IProcess
	system *System
}

func newRegistry(s *System) *Registry {
	return &Registry{
		lookup: make(map[string]IProcess, 1024),
		system: s,
	}
}

func (r *Registry) GetProcessor(id *ActorRef) IProcess {
	proc := r.getByID(id.GetId())
	if proc == nil {
		//todo send to DeadLetter
		slog.Error("get actor by id fail", "id", id.GetId())
		return proc
	}
	return proc
}

func (r *Registry) Remove(id *ActorRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lookup, id.GetId())
}

func (r *Registry) get(ref *ActorRef) IProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if proc, ok := r.lookup[ref.GetId()]; ok {
		return proc
	}
	return nil
}

func (r *Registry) getByID(id string) IProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookup[id]
}

func (r *Registry) add(proc IProcess) error {
	r.mu.Lock()
	id := proc.Self().GetIdentifier()
	if _, ok := r.lookup[id]; ok {
		r.mu.Unlock()
		return errors.New("duplicate actor " + id)
	}
	r.lookup[id] = proc
	r.mu.Unlock()
	return nil
}
