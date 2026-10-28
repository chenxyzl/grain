package grain

import (
	"strconv"
)

type actorIdWrapper struct {
	cacheParse
	fullPath   string
	system     ISystem
	ownerRoute actorRouteCache
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

func (x *actorIdWrapper) routeCache() *actorRouteCache { return &x.ownerRoute }
