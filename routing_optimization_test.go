package grain

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

type routingNodeSnapshot struct {
	version int64
	nodes   []tNodeState
}
type changingRoutingProvider struct {
	iProvider
	snapshot atomic.Pointer[routingNodeSnapshot]
}

func (p *changingRoutingProvider) GetNodesVersion() int64 { return p.snapshot.Load().version }
func (p *changingRoutingProvider) GetNodes() ([]tNodeState, int64) {
	s := p.snapshot.Load()
	return s.nodes, s.version
}
func (p *changingRoutingProvider) set(version int64, address string) {
	var nodes []tNodeState
	if address != "" {
		nodes = []tNodeState{{NodeId: uint64(version), Address: address, Kinds: []string{"player"}}}
	}
	p.snapshot.Store(&routingNodeSnapshot{version, nodes})
}

// Captures actual routing decisions without opening a network stream.
type routingCaptureProcess struct {
	ref  ActorRef
	sent chan Context
}

func (p *routingCaptureProcess) self() ActorRef   { return p.ref }
func (*routingCaptureProcess) opts() *tOpts       { return nil }
func (*routingCaptureProcess) init()              {}
func (*routingCaptureProcess) poison()            {}
func (p *routingCaptureProcess) send(ctx Context) { p.sent <- ctx }
func captureRoute(sys *system, ref ActorRef) *routingCaptureProcess {
	p := &routingCaptureProcess{ref: ref, sent: make(chan Context, 8)}
	sys.registry.processes.Set(ref.GetId(), p)
	return p
}

func TestStreamReferenceCacheFollowsClusterOwnerChanges(t *testing.T) {
	sys := newTestSystem(t)
	provider := &changingRoutingProvider{}
	provider.set(1, "peer-a:9000")
	sys.clusterProvider = provider
	ref := newClusterActorRef("player", "moving", sys)
	a := captureRoute(sys, newDirectActorRef(defaultWriteStreamKind, "peer-a:9000", sys.addr, sys))
	b := captureRoute(sys, newDirectActorRef(defaultWriteStreamKind, "peer-b:9000", sys.addr, sys))
	req := &message.Subscribe{}
	ref.Tell(req)
	receiveTestValue(t, a.sent)
	provider.set(2, "peer-b:9000")
	ref.Tell(req) // reuse the SAME actor reference after owner migration
	receiveTestValue(t, b.sent)
	if len(a.sent) != 0 {
		t.Fatal("used the departed owner's stream")
	}
	if ref.routeCache().entry.Load().version != 2 {
		t.Fatal("actor route was not recomputed")
	}
	if _, ok := sys.routing.generation.Load().streams.Load("peer-a:9000"); ok {
		t.Fatal("new generation retained departed peer reference")
	}

	provider.set(3, sys.addr)
	local := captureRoute(sys, newClusterActorRef("player", "moving", sys))
	ref.Tell(req)
	receiveTestValue(t, local.sent)
	if len(b.sent) != 0 {
		t.Fatal("local delivery used previous remote stream")
	}
	sys.registry.remove(local.ref.GetId(), local)
	provider.set(4, "")
	dead := make(chan DeadLetter, 1)
	sys.config.deadLetterHandler = func(dl DeadLetter) { dead <- dl }
	ref.Tell(req)
	if dl := receiveTestValue(t, dead); dl.Reason != DeadLetterReasonNoOwner {
		t.Fatalf("unexpected rejection: %s", dl.Reason)
	}
	if len(a.sent) != 0 || len(b.sent) != 0 {
		t.Fatal("no-owner send used a cached destination")
	}
	// An empty ownership result must also expire when a node rejoins.
	provider.set(5, "peer-a:9000")
	ref.Tell(req)
	receiveTestValue(t, a.sent)
}

func TestStreamReferenceCacheResolvesReplacementProcessor(t *testing.T) {
	sys := newTestSystem(t)
	peer := "peer:9000"
	ref := resolveTestStream(sys, peer)
	old := captureRoute(sys, ref)
	target := newDirectActorRef("player", "one", peer, sys)
	target.Tell(&message.Subscribe{})
	receiveTestValue(t, old.sent)
	sys.registry.remove(ref.GetId(), old)
	replacement := captureRoute(sys, newDirectActorRef(defaultWriteStreamKind, peer, sys.addr, sys))
	target.Tell(&message.Subscribe{})
	receiveTestValue(t, replacement.sent)
	if len(old.sent) != 0 {
		t.Fatal("cached a stopped processor")
	}
	if resolveTestStream(sys, peer) != ref {
		t.Fatal("stable transport reference was not reused")
	}
}

func TestStreamReferenceCacheConcurrentMembershipChanges(t *testing.T) {
	sys := newTestSystem(t)
	provider := &changingRoutingProvider{}
	provider.set(1, "peer:9000")
	sys.clusterProvider = provider
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 100 {
				ref := resolveTestStream(sys, "peer:9000")
				if ref.GetName() != "peer:9000" || ref.GetDirectAddr() != sys.addr {
					t.Error("invalid stream identity")
				}
			}
		})
	}
	workers.Go(func() {
		for version := int64(2); version <= 100; version++ {
			provider.set(version, "peer:9000")
		}
	})
	workers.Wait()
	ref := resolveTestStream(sys, "peer:9000")
	if sys.routing.generation.Load().version != 100 {
		t.Fatal("cache generation did not advance")
	}
	if resolveTestStream(sys, "peer:9000") != ref {
		t.Fatal("reference not reused in final generation")
	}
}

type replyCaptureSender struct{ got chan Context }

func (s *replyCaptureSender) tell(ref ActorRef, msg proto.Message) {
	s.tellWithSender(ref, msg, nil, 0)
}
func (s *replyCaptureSender) tellWithSender(ref ActorRef, msg proto.Message, sender ActorRef, id uint64) {
	s.got <- newContext(ref, sender, msg, id, s)
}
func TestReplyFastPathPreservesRouting(t *testing.T) {
	sys := newTestSystem(t)
	actor := newDirectActorRef("player", "one", sys.addr, sys)
	response := &message.Subscribe{}
	t.Run("local", func(t *testing.T) {
		id := sys.nextSnId()
		waiter := sys.registerAsk(id)
		ctx := newContext(actor, newReplyRef(id, sys.addr, sys), nil, id, sys)
		ctx.Reply(response)
		if receiveTestValue(t, waiter) != response || sys.pending.Has(id) {
			t.Fatal("local reply was not delivered and removed")
		}
		ctx.Reply(response) // duplicate must not fill the completed waiter again
		if len(waiter) != 0 {
			t.Fatal("duplicate reply delivered")
		}
	})
	t.Run("custom-sender", func(t *testing.T) {
		sender := &replyCaptureSender{got: make(chan Context, 1)}
		ref := newReplyRef(123, sys.addr, sys)
		newContext(actor, ref, nil, 123, sender).Reply(response)
		got := receiveTestValue(t, sender.got)
		if got.Target() != ref || got.Sender() != actor || got.Message() != response || got.GetMsgSnId() != 123 {
			t.Fatal("custom sender bypassed or reply metadata changed")
		}
	})
	t.Run("forwarded-from-other-system", func(t *testing.T) {
		origin := newTestSystem(t)
		origin.addr = "origin:9000"
		id := origin.nextSnId()
		waiter := origin.registerAsk(id)
		defer origin.cancelAsk(id)
		stream := captureRoute(sys, newDirectActorRef(defaultWriteStreamKind, origin.addr, sys.addr, sys))
		reply := newReplyRef(id, origin.addr, origin)
		newContext(actor, reply, nil, id, sys).Reply(response)
		got := receiveTestValue(t, stream.sent)
		if got.Target() != reply || got.Message() != response || len(waiter) != 0 {
			t.Fatal("cross-system reply bypassed transport")
		}
	})
}

func resolveTestStream(sys *system, peer string) ActorRef {
	return sys.routing.resolve(sys, newDirectActorRef("player", "test", peer, sys)).stream
}

// The invalidation hint may advance before the corresponding node snapshot is
// published. An address must never be cached under the hint's newer version.
type laggingRoutingProvider struct {
	changingRoutingProvider
	announced atomic.Int64
}

func (p *laggingRoutingProvider) GetNodesVersion() int64 { return p.announced.Load() }

func TestRoutingResolverWaitsForMatchingSnapshotVersion(t *testing.T) {
	sys := newTestSystem(t)
	provider := &laggingRoutingProvider{}
	provider.set(1, "peer-a:9000")
	provider.announced.Store(2)
	sys.clusterProvider = provider
	target := newClusterActorRef("player", "moving", sys)
	first := sys.routing.resolve(sys, target)
	if first.address != "peer-a:9000" || first.stream.GetName() != first.address {
		t.Fatal("inconsistent old route")
	}
	if sys.routing.generation.Load().version != 1 || target.routeCache().entry.Load().version != 1 {
		t.Fatal("old node snapshot was marked with announced new version")
	}
	provider.set(2, "peer-b:9000") // hint remains 2: this MUST still cause a refresh
	next := sys.routing.resolve(sys, target)
	if next.address != "peer-b:9000" || next.stream.GetName() != next.address {
		t.Fatal("cached old address after matching snapshot became available")
	}
	if sys.routing.generation.Load().version != 2 {
		t.Fatal("snapshot did not advance")
	}
}

func TestRoutingResolverOldSnapshotCannotOverwriteNewOwner(t *testing.T) {
	sys := newTestSystem(t)
	provider := &changingRoutingProvider{}
	provider.set(1, "peer-a:9000")
	sys.clusterProvider = provider
	target := newClusterActorRef("player", "moving", sys)
	old := sys.routing.currentGeneration(provider)
	provider.set(2, "peer-b:9000")
	current := sys.routing.resolve(sys, target)
	// Complete a resolution that captured its generation before the update.
	staleRoute := sys.routing.resolveMiss(sys, old, target, target.routeCache(), false, "")
	stale := staleRoute.address
	if stale != "peer-a:9000" || old.streamRef(sys, stale).GetName() != stale {
		t.Fatal("old send mixed generations")
	}
	if cached := target.routeCache().entry.Load(); cached.version != 2 || cached.address != current.address {
		t.Fatal("older lookup rolled actor cache back")
	}
}

func TestRoutingResolverScopesActorCacheToSendingSystem(t *testing.T) {
	first, second := newTestSystem(t), newTestSystem(t)
	a, b := &changingRoutingProvider{}, &changingRoutingProvider{}
	a.set(1, "peer-a:9000")
	b.set(1, "peer-b:9000")
	first.clusterProvider = a
	second.clusterProvider = b
	ref := newClusterActorRef("player", "shared", first)
	for range 3 {
		if first.routing.resolve(first, ref).address != "peer-a:9000" {
			t.Fatal("first system used foreign route")
		}
		if second.routing.resolve(second, ref).address != "peer-b:9000" {
			t.Fatal("second system used foreign route with same version")
		}
	}
}

func TestRoutingResolverDirectAddressSurvivesMembershipChange(t *testing.T) {
	sys := newTestSystem(t)
	provider := &changingRoutingProvider{}
	provider.set(1, "owner-a:9000")
	sys.clusterProvider = provider
	ref := newDirectActorRef("player", "fixed", "explicit:9000", sys)
	for _, version := range []int64{1, 2, 3} {
		provider.set(version, "owner-b:9000")
		route := sys.routing.resolve(sys, ref)
		if route.address != "explicit:9000" || route.stream.GetName() != route.address {
			t.Fatal("direct address was changed by cluster ownership")
		}
	}
	// Local direct routing must work without even consulting the provider.
	sys.clusterProvider = nil
	local := sys.routing.resolve(sys, newDirectActorRef("player", "local", sys.addr, sys))
	if local.address != sys.addr || local.stream != nil {
		t.Fatal("local direct route opened a remote stream")
	}
}

func TestRoutingResolverSharedReferenceDuringMembershipChange(t *testing.T) {
	sys := newTestSystem(t)
	provider := &changingRoutingProvider{}
	provider.set(1, "peer-a:9000")
	sys.clusterProvider = provider
	target := newClusterActorRef("player", "shared-hot-reference", sys)
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 200 {
				route := sys.routing.resolve(sys, target)
				if route.stream == nil || route.stream.GetName() != route.address {
					t.Error("cached stream and destination came from different generations")
				}
			}
		})
	}
	workers.Go(func() {
		for version := int64(2); version <= 100; version++ {
			peer := "peer-a:9000"
			if version%2 == 0 {
				peer = "peer-b:9000"
			}
			provider.set(version, peer)
		}
	})
	workers.Wait()
	route := sys.routing.resolve(sys, target)
	if route.address != "peer-b:9000" || route.stream.GetName() != route.address || target.routeCache().entry.Load().version != 100 {
		t.Fatal("shared route did not converge to final membership")
	}
}
