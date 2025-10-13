package grain

import "sync"

type actorIdWrapper struct {
	cacheParse
	fullPath    string
	system      ISystem
	cacheRemote *cacheRemote
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

func (x *actorIdWrapper) GetId() string {
	return x.fullPath
}

// String ...
func (x *actorIdWrapper) String() string {
	return x.fullPath
}

// IsDirect ...
func (x *actorIdWrapper) isDirect() bool {
	return x.GetType() == defaultActDirect
}

// IsCluster ...
func (x *actorIdWrapper) isCluster() bool {
	return x.GetType() == defaultActCluster
}

// isAsk ...
func (x *actorIdWrapper) isAsk() bool {
	return x.GetKind() == defaultReplyKind
}

// GetRemoteAddrCache ...
// @return remote addr
// @return remote changed
func (x *actorIdWrapper) getRemoteAddrCache() (string, bool) {
	if !x.isCluster() {
		return "", false
	}
	//
	nodes, version := x.GetSystem().GetProvider().GetNodes()
	vCache := x.cacheRemote
	changed := false
	if vCache == nil || vCache.version != version {
		x.Lock()
		//double check
		if vCache == nil || vCache.version != version {
			cacheAddr := x.GetSystem().getAddrHash().CalcAddrByKind8Name(nodes, x.GetKind(), x.GetName())
			if cacheAddr == "" {
				x.Unlock()
				return "", false
			}
			changed = vCache == nil || vCache.remoteAddr != cacheAddr
			vCache = &cacheRemote{version, cacheAddr}
			x.cacheRemote = vCache
		}
		x.Unlock()
	}
	//
	if vCache == nil {
		return "", changed
	}
	return vCache.remoteAddr, changed
}
