package grain

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/chenxyzl/grain/message"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"google.golang.org/protobuf/proto"
)

// The etcd key layout is a compatibility contract, not an implementation detail: two nodes that
// disagree about it form two disjoint clusters that cannot see each other's members or grain locks,
// so a rolling upgrade would silently double-activate everything. These are golden strings — if a
// change breaks them, it needs a migration, not a test update.
func TestEtcdKeyLayoutIsStable(t *testing.T) {
	c := newConfig("mygame", "0.0.1", nil)
	ref := newClusterActorRef("player", "1234567890123456789", nil)

	for _, tc := range []struct{ name, got, want string }{
		{"memberPrefix", c.getMemberPrefix(), "/mygame/member/"},
		{"memberPath", c.getMemberPath(7), "/mygame/member/7"},
		{"memberPathLarge", c.getMemberPath(1023), "/mygame/member/1023"},
		{"extDataPrefix", c.getMemberExtDataPath("zone"), "/mygame/member_ext/zone"},
		{"extDataPath", c.getMemberExtDataPath("zone", 7), "/mygame/member_ext/zone/7"},
		{"extDataPathZeroId", c.getMemberExtDataPath("zone", 0), "/mygame/member_ext/zone"},
		{"eventStreamWatch", c.getEventStreamWatchPath(), "/mygame/event_stream/"},
		{"actorRegister", c.getActorRegisterName(ref), "/mygame/cluster/player/1234567890123456789"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// getMemberPath must round-trip through the id that applyMemberLocked parses back out of the key,
// or a node's own member entry lands under a different key than freeNodeIds looks for.
func TestMemberPathRoundTripsThroughParse(t *testing.T) {
	x := &providerEtcd{config: newConfig("mygame", "0.0.1", nil)}
	x.logger = discardLogger()
	x.nodeMap = make(map[string]tNodeState)
	for _, id := range []uint64{1, 9, 10, 512, 1023} {
		key := x.config.getMemberPath(id)
		x.applyMemberLocked(mvccpb.PUT, key, []byte(`{"NodeId":`+strconv.FormatUint(id, 10)+`,"Address":"10.0.0.1:1"}`))
	}
	x.publishNodesLocked()
	for _, id := range []uint64{1, 9, 10, 512, 1023} {
		if _, ok := x.nodeMap[strconv.FormatUint(id, 10)]; !ok {
			t.Errorf("node %d is not keyed by its id after parsing %q",
				id, x.config.getMemberPath(id))
		}
	}
}

// eventStreamProvider stubs just the subscription-registry calls eventStream makes.
type eventStreamProvider struct {
	iProvider
	mu   sync.Mutex
	keys map[string]string
}

func (p *eventStreamProvider) registerEventStream(path string, val string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.keys == nil {
		p.keys = map[string]string{}
	}
	p.keys[path] = val
	return nil
}
func (p *eventStreamProvider) unregisterEventStream(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.keys, path)
	return nil
}
func (p *eventStreamProvider) watchEventStream(string, func(watchOp, string, []byte)) error {
	return nil // no peers in this test
}
func (p *eventStreamProvider) has(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.keys[path]
	return ok
}

// A local publish must reach every subscriber, and stop reaching one that unsubscribed. Covers the
// subscription table holding a prebuilt ActorRef per subscriber rather than re-parsing the id on
// every delivery.
func TestEventStreamPublishReachesSubscribers(t *testing.T) {
	sys := newTestSystem(t)
	prov := &eventStreamProvider{}
	sys.clusterProvider = prov

	es := newEventStream(1, prov, sys.config.getEventStreamWatchPath()).(*eventStream)
	esRef, err := sys.SpawnNamed(func() IActor { return es }, eventStreamWatchName,
		WithOptsKindName(defaultSystemKind))
	if err != nil {
		t.Fatal(err)
	}
	sys.eventStream = esRef

	got := make(chan string, 8)
	sub := &subscriberActor{seen: got}
	subRef, err := sys.SpawnNamed(func() IActor { return sub }, "subscriber-1")
	if err != nil {
		t.Fatal(err)
	}

	// NOT message.Subscribe/Unsubscribe/BroadcastPublishProtoWrapper: eventStream.Receive treats
	// those as its own control messages, so publishing one is swallowed rather than delivered.
	// The framework's message.* types are reserved; real events are the app's own protos.
	event := &message.ErrCode{Code: 1, Des: "payload"}
	sys.Subscribe(subRef, event)
	waitFor(t, func() bool {
		return prov.has(es.getEventNamePath(string(proto.MessageName(event))))
	}, "subscription to be registered")

	sys.PublishLocal(event)
	select {
	case id := <-got:
		if id != subRef.GetId() {
			t.Errorf("delivered to %q, want %q", id, subRef.GetId())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("published event never reached the subscriber")
	}

	sys.Unsubscribe(subRef, event)
	waitFor(t, func() bool {
		return !prov.has(es.getEventNamePath(string(proto.MessageName(event))))
	}, "subscription to be dropped")

	sys.PublishLocal(event)
	select {
	case id := <-got:
		t.Fatalf("event still delivered to %q after Unsubscribe", id)
	case <-time.After(300 * time.Millisecond):
	}
}

type subscriberActor struct {
	BaseActor
	seen chan string
}

func (a *subscriberActor) Started() {}
func (a *subscriberActor) PreStop() {}
func (a *subscriberActor) Receive(ctx Context) {
	if _, ok := ctx.Message().(*message.ErrCode); ok {
		a.seen <- a.Self().GetId()
	}
}
