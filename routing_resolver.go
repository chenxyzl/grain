package grain

import (
	"sync"
	"sync/atomic"
)

// routingResolver owns both routing policy and cache generations for one system.
// An actor's tiny cache never retains a generation (and all its peer references).
type routingResolver struct {
	generation atomic.Pointer[routingGeneration]
	refreshMu  sync.Mutex
}
type routingGeneration struct {
	version int64
	nodes   []tNodeState // immutable provider snapshot, paired with version
	streams sync.Map     // destination address -> local stream ActorRef, never processor
}
type actorRouteCache struct {
	entry atomic.Pointer[cachedActorRoute]
	mu    sync.Mutex
}
type cachedActorRoute struct {
	resolver *routingResolver // one reference can be used by different sending systems
	version  int64
	resolvedRoute
}
type resolvedRoute struct {
	address string   // empty for an unhosted cluster kind
	stream  ActorRef // nil for local or unhosted destinations
}

// A version read is only an invalidation hint. Published generations ALWAYS use
// the version returned alongside GetNodes, which can lag the hint briefly.
func (r *routingResolver) currentGeneration(provider iProvider) *routingGeneration {
	version := provider.GetNodesVersion()
	current := r.generation.Load()
	if current != nil && current.version >= version {
		return current
	}
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	current = r.generation.Load()
	if current != nil && current.version >= version {
		return current
	}
	nodes, snapshotVersion := provider.GetNodes()
	if current != nil && current.version >= snapshotVersion {
		return current
	}
	next := &routingGeneration{version: snapshotVersion, nodes: nodes}
	r.generation.Store(next)
	return next
}

// resolve uses one generation for BOTH owner selection and transport lookup.
// Local direct sends never need a provider lookup or a cache generation.
func (r *routingResolver) resolve(sys *system, target ActorRef) resolvedRoute {
	direct := target.isDirect()
	var address string
	if direct {
		address = target.GetDirectAddr()
		if address == sys.addr {
			return resolvedRoute{address: address}
		}
	}
	generation := r.currentGeneration(sys.clusterProvider)
	cache := target.routeCache()
	if cache != nil {
		entry := cache.entry.Load()
		if entry != nil && entry.resolver == r && entry.version == generation.version {
			return entry.resolvedRoute
		}
	}
	return r.resolveMiss(sys, generation, target, cache, direct, address)
}

// Cache the complete route, not just its address. A warm actor reference can
// skip the per-node sync.Map lookup; it retains only its stream identity, never
// the generation, node snapshot, or live processor.
func (r *routingResolver) resolveMiss(sys *system, generation *routingGeneration, target ActorRef, cache *actorRouteCache, direct bool, address string) resolvedRoute {
	if cache != nil {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		entry := cache.entry.Load()
		if entry != nil && entry.resolver == r && entry.version == generation.version {
			return entry.resolvedRoute
		}
	}
	route := resolvedRoute{address: address}
	if !direct {
		if !target.isCluster() {
			return resolvedRoute{}
		}
		route.address = sys.addrHash.CalcAddrByKind8Name(generation.nodes, target.GetKind(), target.GetName())
	}
	if (direct || route.address != "") && route.address != sys.addr {
		route.stream = generation.streamRef(sys, route.address)
	}
	if cache != nil {
		entry := cache.entry.Load()
		// An older in-flight resolution must not roll the shared route back.
		if entry == nil || entry.resolver != r || entry.version <= generation.version {
			cache.entry.Store(&cachedActorRoute{resolver: r, version: generation.version, resolvedRoute: route})
		}
	}
	return route
}
func (g *routingGeneration) streamRef(sys *system, address string) ActorRef {
	if ref, ok := g.streams.Load(address); ok {
		return ref.(ActorRef)
	}
	ref := newDirectActorRef(defaultWriteStreamKind, address, sys.addr, sys)
	actual, _ := g.streams.LoadOrStore(address, ref)
	return actual.(ActorRef)
}
