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
	sync.RWMutex
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

// String ...
func (x *actorIdWrapper) String() string { return x.fullPath }

// isDirect ...
func (x *actorIdWrapper) isDirect() bool { return x.GetType() == defaultActDirect }

// isCluster ...
func (x *actorIdWrapper) isCluster() bool { return x.GetType() == defaultActCluster }

// isAsk ...
func (x *actorIdWrapper) isAsk() bool { return x.GetKind() == defaultReplyKind }

// askSnId parses the correlation id from the reply ref's name. Only meaningful
// on the remote inbound path (a reply ref rebuilt via newActorRefFromAID);
// local asks use the lighter replyRef which carries the snId directly.
func (x *actorIdWrapper) askSnId() uint64 {
	n, _ := strconv.ParseUint(x.GetName(), 10, 64)
	return n
}

// GetRemoteAddrCache ...
// @return remote addr
// @return remote changed
func (x *actorIdWrapper) getRemoteAddrCache() (string, bool) {
	if !x.isCluster() {
		return "", false
	}
	//
	// fast path: compare the provider version only (no slice allocation). The
	// full GetNodes() is called just once, when the version actually advanced.
	version := x.GetSystem().GetProvider().GetNodesVersion()
	vCache := x.cacheRemote.Load()
	if vCache != nil && vCache.version == version {
		return vCache.remoteAddr, false
	}
	changed := false
	{
		x.Lock()
		//double check: re-read under the lock, not the stale value captured above.
		vCache = x.cacheRemote.Load()
		if vCache == nil || vCache.version != version {
			nodes, ver := x.GetSystem().GetProvider().GetNodes()
			cacheAddr := x.GetSystem().getAddrHash().CalcAddrByKind8Name(nodes, x.GetKind(), x.GetName())
			if cacheAddr == "" {
				x.Unlock()
				return "", false
			}
			changed = vCache == nil || vCache.remoteAddr != cacheAddr
			vCache = &cacheRemote{ver, cacheAddr}
			x.cacheRemote.Store(vCache)
		}
		x.Unlock()
	}
	//
	if vCache == nil {
		return "", changed
	}
	return vCache.remoteAddr, changed
}
