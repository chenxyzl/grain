package grain

import (
	"fmt"
	"sync"

	"github.com/chenxyzl/grain/al/ringbuffer"
)

// processorCreator serializes construction of the same ID, across both spawn
// policies. The bookkeeping mutex is never held while calling user producers.
// Slots exist only while creators are using or waiting for them.
type processorCreator struct {
	mu     sync.Mutex
	active map[string]*processorCreationSlot
}
type processorCreationSlot struct {
	mu    sync.Mutex
	users int // protected by processorCreator.mu, includes waiting callers
}

func (c *processorCreator) lock(id string) *processorCreationSlot {
	c.mu.Lock()
	if c.active == nil {
		c.active = make(map[string]*processorCreationSlot)
	}
	slot := c.active[id]
	if slot == nil {
		slot = &processorCreationSlot{}
		c.active[id] = slot
	}
	slot.users++
	c.mu.Unlock()
	slot.mu.Lock()
	return slot
}
func (c *processorCreator) unlock(id string, slot *processorCreationSlot) {
	slot.mu.Unlock()
	c.mu.Lock()
	slot.users--
	if slot.users == 0 {
		delete(c.active, id)
	}
	c.mu.Unlock()
}

// spawnNewProcessor reserves the ID before invoking the producer. A duplicate
// returns ErrNameExists without constructing an unused actor instance.
func spawnNewProcessor(system ISystem, opts tOpts) (iProcess, error) {
	id := opts._self.GetId()
	creator := system.getProcessorCreator()
	slot := creator.lock(id)
	defer creator.unlock(id, slot)
	registry := system.getRegistry()
	if registry.lookup(id) != nil {
		return nil, fmt.Errorf("%w: %s", ErrNameExists, id)
	}
	proc := buildProcessor(system, opts)
	if _, inserted := registry.register(id, proc); !inserted {
		return nil, fmt.Errorf("%w: %s", ErrNameExists, id)
	}
	proc.init()
	return proc, nil
}

// getOrSpawnProcessor reuses a published instance, or creates one while holding
// the same per-ID reservation used by explicit spawn. Hits do not take creation locks.
func getOrSpawnProcessor(system ISystem, opts tOpts) iProcess {
	id := opts._self.GetId()
	registry := system.getRegistry()
	if proc := registry.lookup(id); proc != nil {
		return proc
	}
	creator := system.getProcessorCreator()
	slot := creator.lock(id)
	defer creator.unlock(id, slot)
	if proc := registry.lookup(id); proc != nil {
		return proc
	}
	proc := buildProcessor(system, opts)
	actual, inserted := registry.register(id, proc)
	if inserted {
		proc.init()
	}
	return actual
}

// Bind the actor and enqueue Initialize before publication, so an external send
// cannot overtake startup even if it schedules the drainer before init is called.
func buildProcessor(system ISystem, opts tOpts) *processorMailBox {
	proc := &processorMailBox{tOpts: opts, system: system,
		mailbox: ringbuffer.New[Context](int64(opts.mailboxInitSize), int64(opts.mailboxMaxSize))}
	proc.receiver = opts.producer()
	proc.receiver._init(proc.self())
	proc.initializeScheduling()
	return proc
}
