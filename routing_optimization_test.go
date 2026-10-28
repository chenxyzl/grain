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
	if ref.(*actorIdWrapper).cacheRemote.Load().version != 2 {
		t.Fatal("actor route was not recomputed")
	}
	if _, ok := sys.streamReferences.Load().references.Load("peer-a:9000"); ok {
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
	ref := sys.writeStreamRef(peer)
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
	if sys.writeStreamRef(peer) != ref {
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
				ref := sys.writeStreamRef("peer:9000")
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
	ref := sys.writeStreamRef("peer:9000")
	if sys.streamReferences.Load().nodesVersion != 100 {
		t.Fatal("cache generation did not advance")
	}
	if sys.writeStreamRef("peer:9000") != ref {
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
