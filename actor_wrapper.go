package grain

import (
	"strconv"
	"sync"
	"sync/atomic"
)

type actorIdWrapper struct {
	cacheParse
	fullPath    string
	system      ISystem
	cacheRemote atomic.Pointer[cacheRemote]
	// sync.Mutex, not RWMutex: the read path is the lock-free atomic load in getRemoteAddrCache,
	// so this only ever guards the recompute and is never read-locked. 16 bytes smaller, on a
	// struct there is one of per ActorRef.
	mu sync.Mutex
}

func newDirectActorRef(kind string, name string, addr string, system ISystem) ActorRef {
	ret := &actorIdWrapper{
		fullPath: defaultActDirect + "/" + kind + "/" + name + "@" + addr,
		system:   system,
	}
	ret.parseCache()
	return ret
}

func newClusterActorRef(kind string, name string, system ISystem) ActorRef {
	ret := &actorIdWrapper{
		fullPath: defaultActCluster + "/" + kind + "/" + name,
		system:   system,
	}
	ret.parseCache()
	return ret
}

func newActorRefFromAID(aid string, system ISystem) ActorRef {
	ret := &actorIdWrapper{
		fullPath: aid,
		system:   system,
	}
	ret.parseCache()
	return ret
}

func (x *actorIdWrapper) parseCache() {
	typ, kind, name, directAddr := parseCache(x.fullPath)
	x.cacheParse = cacheParse{
		d8c:        typ,
		kind:       kind,
		name:       name,
		directAddr: directAddr,
	}
}

func (x *actorIdWrapper) GetId() string { return x.fullPath }

func (x *actorIdWrapper) String() string { return x.fullPath }

func (x *actorIdWrapper) isDirect() bool { return x.GetType() == defaultActDirect }

func (x *actorIdWrapper) isCluster() bool { return x.GetType() == defaultActCluster }

func (x *actorIdWrapper) isAsk() bool { return x.GetKind() == defaultReplyKind }

// askSnId parses the correlation id out of a reply ref's name. Only meaningful on the remote
// inbound path; local asks use replyRef, which carries the snId directly.
func (x *actorIdWrapper) askSnId() uint64 {
	n, _ := strconv.ParseUint(x.GetName(), 10, 64)
	return n
}

// getRemoteAddrCache resolves which node currently owns this cluster actor, caching the answer
// against the provider's node-set version. Returns "" when the kind is hosted nowhere.
func (x *actorIdWrapper) getRemoteAddrCache() string {
	if !x.isCluster() {
		return ""
	}
	// fast path: version compare only; the node set is consulted only once it has advanced
	version := x.GetSystem().getProvider().GetNodesVersion()
	if vCache := x.cacheRemote.Load(); vCache != nil && vCache.version == version {
		return vCache.remoteAddr
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	//double check under the lock, not the stale value read above
	vCache := x.cacheRemote.Load()
	if vCache != nil && vCache.version == version {
		return vCache.remoteAddr
	}
	nodes, ver := x.GetSystem().getProvider().GetNodes()
	cacheAddr := x.GetSystem().getAddrHash().CalcAddrByKind8Name(nodes, x.GetKind(), x.GetName())
	// "" is cached too, and must be: it is a real answer for this node set ("no node hosts this
	// kind"), and the version guards it exactly as well as an address. Skipping it made every
	// send to an unhosted kind re-run the hash over the whole node set.
	x.cacheRemote.Store(&cacheRemote{ver, cacheAddr})
	return cacheAddr
}
