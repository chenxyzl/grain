package grain

import (
	"errors"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/remote"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
)

// fakeProvider implements only the node-set queries routing needs; the rest panics (nil embed).
type fakeProvider struct {
	iProvider
	nodes []tNodeState
}

func (f *fakeProvider) GetNodes() ([]tNodeState, int64) { return f.nodes, 1 }
func (f *fakeProvider) GetNodesVersion() int64          { return 1 }

// newTestSystem builds a real *system on a fakeProvider: envelope and routing paths, no etcd.
func newTestSystem(t *testing.T) *system { return newTestSystemTB(t) }

// newTestSystemTB is the testing.TB form, so benchmarks can use it too.
func newTestSystemTB(t testing.TB) *system {
	t.Helper()
	// Spawn's generated names come from the uuid generator, so it must be seeded first
	_ = uuid.Init(1)
	sys := &system{
		config:   newConfig("test", "0.0.1", []string{"127.0.0.1:2379"}),
		addr:     "10.10.108.145:50685",
		pending:  safemap.NewIntC[uint64, chan proto.Message](),
		addrHash: newAddrHash(),
	}
	// logger left unset, so Logger() falls back to slog.Default() as it does before Start()
	sys.registry = newRegistry()
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

// TestRecvEnvelopeEmptySenderStaysNil: a remote Tell carries Sender "", which must arrive as a
// nil ctx.Sender() — newActorRefFromAID("") yields a non-nil ref with all-empty fields, making
// `ctx.Sender() != nil` true for every remote Tell.
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

// TestRecvEnvelopeKeepsRealSender guards against over-correcting: a real sender AID survives.
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

// TestRecvEnvelopeRejectsGarbage: a nil envelope or empty target is dropped, not panicked on.
func TestRecvEnvelopeRejectsGarbage(t *testing.T) {
	sys := newTestSystem(t)
	body, _ := proto.Marshal(&message.Subscribe{EventName: "e"})
	name := string(proto.MessageName(&message.Subscribe{}))

	// must not be dereferenced
	sys.RecvEnvelope(nil)
	// empty target has nothing to route to
	sys.RecvEnvelope(&remote.Envelope{Target: "", MsgName: name, Content: body})
}

// TestSpawnNamedDuplicateReturnsError: a duplicate named spawn returns ErrNameExists rather
// than panicking, and leaves the original registration intact.
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

// TestSpawnGeneratedNameNeverCollides: Spawn uses a uuid, hence its error-free signature.
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
