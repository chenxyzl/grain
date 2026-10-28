package grain

import "sync"

// This cache contains transport identities by destination address, NOT cluster
// actor ownership or live processors. Each send still resolves the actor's owner
// via its versioned routing cache and looks up the current stream in the registry.
// Replacing the generation lets departed-node references be reclaimed.
type streamReferenceCache struct {
	nodesVersion int64
	references   sync.Map // destination address -> ActorRef
}

func (x *system) writeStreamRef(address string) ActorRef {
	version := x.clusterProvider.GetNodesVersion()
	cache := x.streamReferences.Load()
	for cache == nil || cache.nodesVersion < version {
		next := &streamReferenceCache{nodesVersion: version}
		if x.streamReferences.CompareAndSwap(cache, next) {
			cache = next
			break
		}
		cache = x.streamReferences.Load()
	}
	// A concurrent caller may have published a newer generation. Never replace
	// it with this caller's older version; address identities are version-independent.
	if ref, ok := cache.references.Load(address); ok {
		return ref.(ActorRef)
	}
	ref := newDirectActorRef(defaultWriteStreamKind, address, x.getAddr(), x)
	actual, _ := cache.references.LoadOrStore(address, ref)
	return actual.(ActorRef)
}
