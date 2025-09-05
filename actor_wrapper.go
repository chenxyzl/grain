package grain

type actorIdWrapper struct {
	cacheParse
	fullPath    string
	system      ISystem
	cacheRemote *cacheRemote
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

// needReply ...
func (x *actorIdWrapper) needReply() bool {
	return x.GetKind() == defaultReplyKind
}

// SetRemoteAddrCache ...
func (x *actorIdWrapper) setRemoteAddrCache(addr string, version int64) {
	if !x.isCluster() {
		return
	}
	if addr == "" {
		return
	}
	x.cacheRemote = &cacheRemote{version, addr}
}

// GetRemoteAddrCache ...
// return remote addr
// return cluster provider version
func (x *actorIdWrapper) getRemoteAddrCache() (string, int64) {
	if !x.isCluster() {
		return "", 0
	}
	v := x.cacheRemote
	if v == nil {
		return "", 0
	}
	return v.remoteAddr, v.version
}
