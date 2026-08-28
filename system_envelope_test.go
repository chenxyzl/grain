package grain

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/remote"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
)

// fakeProvider is a minimal iProvider: only the node-set queries are implemented,
// which is all the routing paths need. Anything else panics via the nil embed.
type fakeProvider struct {
	iProvider
	nodes []tNodeState
}

func (f *fakeProvider) GetNodes() ([]tNodeState, int64) { return f.nodes, 1 }
func (f *fakeProvider) GetNodesVersion() int64          { return 1 }

// newTestSystem builds a real *system wired to a fakeProvider, so the envelope and
// routing paths (which are methods on *system, not on the fakeSys stub) can be
// driven without etcd or grpc.
func newTestSystem(t *testing.T) *system {
	t.Helper()
	// Production does this in providerEtcd.register -> system.init(nodeId); Spawn's
	// generated names come from the uuid generator, so it must be seeded first.
	_ = uuid.Init(1)
	sys := &system{
		config:   newConfig("test", "0.0.1", []string{"127.0.0.1:2379"}),
		logger:   slog.Default(),
		addr:     "testaddr",
		pending:  safemap.NewIntC[uint64, chan proto.Message](),
		addrHash: newAddrHash(),
	}
	sys.registry = newRegistry(sys.Logger())
	sys.clusterProvider = &fakeProvider{
		nodes: []tNodeState{{NodeId: 1, Address: sys.addr, Kinds: []string{"player"}}},
	}
	return sys
}

// recorder records what its handler observed about the incoming context.
type recorder struct {
	BaseActor
	got  chan ActorRef // the ctx.Sender() as seen by the handler
	nil_ chan bool     // whether ctx.Sender() was nil
}

func (a *recorder) Started() {}
func (a *recorder) PreStop() {}
func (a *recorder) Receive(ctx Context) {
	if _, ok := ctx.Message().(*message.Subscribe); !ok {
		return
	}
	a.nil_ <- ctx.Sender() == nil
	a.got <- ctx.Sender()
}

// TestRecvEnvelopeEmptySenderStaysNil pins the fix for a trap that broke the single
// most common idiom in a handler.
//
// A remote Tell carries no sender: stream_write writes "" into Envelope.Sender.
// RecvEnvelope used to feed that "" to newActorRefFromAID, which returns a NON-nil
// *actorIdWrapper whose kind/name/addr are all empty (parseCache bails out and
// returns four empty strings). So `if ctx.Sender() != nil` — used by the framework
// itself and by essentially every user handler — was TRUE for every remote Tell, and
// ctx.Reply() on such a message failed with the misleading "actor kind not in
// cluster".
func TestRecvEnvelopeEmptySenderStaysNil(t *testing.T) {
	sys := newTestSystem(t)
	act := &recorder{got: make(chan ActorRef, 1), nil_: make(chan bool, 1)}
	ref, err0 := sys.SpawnNamed(func() IActor { return act }, "target")
	if err0 != nil {
		t.Fatal(err0)
	}

	body, err := proto.Marshal(&message.Subscribe{EventName: "e"})
	if err != nil {
		t.Fatal(err)
	}
	sys.RecvEnvelope(&remote.Envelope{
		Target:  ref.GetId(),
		Sender:  "", // a remote Tell has no sender
		MsgName: string(proto.MessageName(&message.Subscribe{})),
		Content: body,
	})

	select {
	case isNil := <-act.nil_:
		sender := <-act.got
		if !isNil {
			t.Errorf("ctx.Sender() must be nil for a remote Tell with no sender, got %#v "+
				"(kind=%q name=%q) — `if ctx.Sender() != nil` is a false positive",
				sender, sender.GetKind(), sender.GetName())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the envelope was never delivered")
	}
}

// TestRecvEnvelopeKeepsRealSender guards against over-correcting: a real sender AID
// must still arrive intact.
func TestRecvEnvelopeKeepsRealSender(t *testing.T) {
	sys := newTestSystem(t)
	act := &recorder{got: make(chan ActorRef, 1), nil_: make(chan bool, 1)}
	ref, err0 := sys.SpawnNamed(func() IActor { return act }, "target2")
	if err0 != nil {
		t.Fatal(err0)
	}
	senderRef := newDirectActorRef("local", "peer", "otheraddr", sys)

	body, _ := proto.Marshal(&message.Subscribe{EventName: "e"})
	sys.RecvEnvelope(&remote.Envelope{
		Target:  ref.GetId(),
		Sender:  senderRef.GetId(),
		MsgName: string(proto.MessageName(&message.Subscribe{})),
		Content: body,
	})

	select {
	case isNil := <-act.nil_:
		sender := <-act.got
		if isNil {
			t.Fatal("a real sender AID was dropped")
		}
		if sender.GetName() != "peer" || sender.GetDirectAddr() != "otheraddr" {
			t.Errorf("sender not reconstructed: name=%q addr=%q", sender.GetName(), sender.GetDirectAddr())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the envelope was never delivered")
	}
}

// TestRecvEnvelopeRejectsGarbage: a nil envelope or an empty target must be dropped,
// not panic and not routed.
func TestRecvEnvelopeRejectsGarbage(t *testing.T) {
	sys := newTestSystem(t)
	body, _ := proto.Marshal(&message.Subscribe{EventName: "e"})
	name := string(proto.MessageName(&message.Subscribe{}))

	// nil envelope used to nil-deref envelope.MsgName and crash the process.
	sys.RecvEnvelope(nil)
	// empty target has nothing to route to.
	sys.RecvEnvelope(&remote.Envelope{Target: "", MsgName: name, Content: body})
}

// TestSpawnNamedDuplicateReturnsError pins the replacement for a process-killing
// panic: registry.add used to `panic("duplicated process id")`, so re-spawning a named
// actor after a crash — or two goroutines racing to create it — took the whole process
// down. protoactor-go returns ErrNameExists from SpawnNamed; Akka throws a *catchable*
// InvalidActorNameException; Orleans has no such failure at all because grains are
// virtual and activation is idempotent (this framework's cluster kinds already behave
// that way, via ensureClusterKindActorExist). For an explicit named spawn in Go, an
// error is the right answer.
func TestSpawnNamedDuplicateReturnsError(t *testing.T) {
	sys := newTestSystem(t)
	producer := func() IActor { return &recorder{got: make(chan ActorRef, 1), nil_: make(chan bool, 1)} }

	first, err := sys.SpawnNamed(producer, "dup")
	if err != nil {
		t.Fatalf("first spawn failed: %v", err)
	}
	if first == nil {
		t.Fatal("first spawn returned a nil ref")
	}

	second, err := sys.SpawnNamed(producer, "dup")
	if err == nil {
		t.Fatal("a duplicate name must return an error, not succeed")
	}
	if !errors.Is(err, ErrNameExists) {
		t.Errorf("want ErrNameExists, got %v", err)
	}
	if second != nil {
		t.Errorf("a failed spawn must return a nil ref, got %v", second)
	}
	// The original must be untouched and still registered.
	if sys.registry.get(first) == nil {
		t.Error("the failed duplicate spawn evicted the original actor")
	}
}

// TestSpawnGeneratedNameNeverCollides: Spawn uses a uuid, so it keeps the
// error-free signature.
func TestSpawnGeneratedNameNeverCollides(t *testing.T) {
	sys := newTestSystem(t)
	seen := map[string]bool{}
	for range 50 {
		ref := sys.Spawn(func() IActor { return &recorder{got: make(chan ActorRef, 1), nil_: make(chan bool, 1)} })
		if ref == nil {
			t.Fatal("Spawn returned nil")
		}
		if seen[ref.GetId()] {
			t.Fatalf("Spawn produced a duplicate id: %s", ref.GetId())
		}
		seen[ref.GetId()] = true
	}
}
