package grain

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/remote"
	"google.golang.org/protobuf/proto"
)

// Regressions for the bugs found in the framework review. Each test fails against the code as it
// stood before its fix; the comment states the interleaving that made it reachable.

// deliverReply must not block. wakePendingAsks writes msgPoison WITHOUT popping the pending
// entry, so it is a second writer into the cap-1 channel. If the asker has already timed out it
// will never drain it (askImpl removes the entry only after resumeTurn, which waits for the
// turn), so a blocking send wedged the caller forever — for a local reply that is the replying
// actor's own processing goroutine, holding its turn, so the actor could never stop.
func TestDeliverReplyNeverBlocks(t *testing.T) {
	sys := newTestSystem(t)
	const snId = 42

	_ = sys.registerAsk(snId) // an Ask is in flight
	sys.wakePendingAsks()     // shutdown fills the cap-1 buffer, entry left in place

	done := make(chan struct{})
	go func() {
		sys.deliverReply(snId, &message.Unsubscribe{EventName: "late-reply"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deliverReply blocked on a full cap-1 pending channel")
	}
}

// relayActor stands in for streamWriteActor: it forwards whatever it is handed to a peer node.
type relayActor struct {
	BaseActor
	forwarded atomic.Int64
	stopped   atomic.Bool
}

func (a *relayActor) Started()            {}
func (a *relayActor) PreStop()            { a.stopped.Store(true) }
func (a *relayActor) Receive(ctx Context) { a.forwarded.Add(1) }

// A processor must only INTERPRET a control message that is addressed to itself. The write_stream
// actor is handed contexts whose target is an actor on another node; intercepting a Poison there
// stopped the outbound stream to that entire peer and left the remote grain running, never
// delivering the poison.
func TestPoisonInTransitIsForwardedNotInterpreted(t *testing.T) {
	sys := newTestSystem(t)
	r := &relayActor{}
	relayRef, err := sys.SpawnNamed(func() IActor { return r },
		"10.255.255.1:59999", WithOptsKindName(defaultWriteStreamKind))
	if err != nil {
		t.Fatal(err)
	}
	proc := sys.registry.get(relayRef)
	if proc == nil {
		t.Fatal("relay not registered")
	}

	// exactly what sendToCluster enqueues: a context whose TARGET is the remote grain
	remoteGrain := newDirectActorRef("player", "grain-1", "10.255.255.1:59999", sys)
	proc.send(newContext(remoteGrain, nil, msgPoison, 1, sys))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.forwarded.Load() > 0 {
			if r.stopped.Load() {
				t.Fatal("relay forwarded the poison but also stopped itself")
			}
			return // forwarded as opaque cargo, relay still alive
		}
		if r.stopped.Load() {
			t.Fatal("relay consumed a poison addressed to a remote grain and stopped itself; " +
				"the outbound stream to that peer is gone and the grain was never poisoned")
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("neither forwarded nor stopped: forwarded=%d", r.forwarded.Load())
}

// A control message addressed to the actor itself must still be interpreted.
func TestPoisonAddressedToSelfStillStops(t *testing.T) {
	sys := newTestSystem(t)
	r := &relayActor{}
	ref, err := sys.SpawnNamed(func() IActor { return r }, "self-poison")
	if err != nil {
		t.Fatal(err)
	}
	sys.tellWithSender(ref, msgPoison, nil, 1)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sys.registry.get(ref) == nil {
			if got := r.forwarded.Load(); got != 0 {
				t.Fatalf("poison was handed to Receive %d times instead of being interpreted", got)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("actor did not stop on a Poison addressed to itself")
}

// leaseRecorder captures the revokeLease decision WaitStopSignal hands the cluster provider.
type leaseRecorder struct {
	fakeProvider
	stopped atomic.Bool
	revoked atomic.Bool
}

func (f *leaseRecorder) stop(revokeLease bool) {
	f.revoked.Store(revokeLease)
	f.stopped.Store(true)
}

// wedgedActor never returns from Receive, so it can never be drained.
type wedgedActor struct {
	BaseActor
	release chan struct{}
}

func (a *wedgedActor) Started()            {}
func (a *wedgedActor) PreStop()            {}
func (a *wedgedActor) Receive(ctx Context) { <-a.release }

// Every grain's single-activation lock hangs off this node's etcd lease, so the lease must NOT be
// revoked while an activation is still running: a peer would take the freed lock, activate the
// same grain, and the two would write the same state. Draining is therefore allowed to fail, and
// the failure has to reach the provider.
func TestLeaseNotRevokedWhenActorsDoNotDrain(t *testing.T) {
	sys := newTestSystem(t)
	rec := &leaseRecorder{}
	sys.clusterProvider = rec
	sys.config.stopWaitTimeSecond = 1 // two phases, so ~2s

	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // let the wedged goroutine exit with the test
	ref, err := sys.SpawnNamed(func() IActor { return &wedgedActor{release: release} }, "wedged")
	if err != nil {
		t.Fatal(err)
	}
	sys.tellWithSender(ref, &message.Unsubscribe{EventName: "wedge"}, nil, 1)

	if drained := sys.stopActors(); drained {
		t.Fatal("stopActors reported success while an actor was still inside Receive")
	}

	// and the decision must actually reach the provider
	sys.ForceStop(nil)
	sys.WaitStopSignal(nil, nil)
	if !rec.stopped.Load() {
		t.Fatal("provider.stop was never called")
	}
	if rec.revoked.Load() {
		t.Fatal("lease was revoked even though actors are still running: a peer can now " +
			"double-activate every grain this node still hosts")
	}
}

// The healthy path must still revoke, or a clean restart waits out the lease TTL.
func TestLeaseRevokedOnCleanDrain(t *testing.T) {
	sys := newTestSystem(t)
	rec := &leaseRecorder{}
	sys.clusterProvider = rec
	sys.config.stopWaitTimeSecond = 5

	if _, err := sys.SpawnNamed(func() IActor { return &sink{} }, "tidy"); err != nil {
		t.Fatal(err)
	}
	sys.ForceStop(nil)
	sys.WaitStopSignal(nil, nil)

	if !rec.stopped.Load() {
		t.Fatal("provider.stop was never called")
	}
	if !rec.revoked.Load() {
		t.Fatal("actors drained cleanly but the lease was not revoked")
	}
}

// message.Initialize is a normally registered proto, so before this check any peer could name it
// in an envelope and have invoke() re-run start() on a live actor: Started() twice for a plain
// actor, or — for a cluster grain — a setTxn against our own key that fails and then tears the
// grain down through its own still-valid lock. The transport is insecure credentials with no auth
// on Remoting.Listen, so "any peer" is the whole reachable network.
func TestInboundInitializeIsRefused(t *testing.T) {
	sys := newTestSystem(t)
	c := &startCounter{}
	ref, err := sys.SpawnNamed(func() IActor { return c }, "victim")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return c.starts.Load() == 1 }, "actor to start")

	body, err := proto.Marshal(msgInitialize)
	if err != nil {
		t.Fatal(err)
	}
	sys.RecvEnvelope(&remote.Envelope{
		Target:  ref.GetId(),
		MsgName: msgNameInitialize,
		Content: body,
		MsgSnId: 999,
	})

	time.Sleep(200 * time.Millisecond)
	if got := c.starts.Load(); got != 1 {
		t.Fatalf("Started() ran %d times; a remote envelope re-activated a live actor", got)
	}
}

// A remote Poison must still be routed: that is how stopping a grain on another node works.
func TestInboundPoisonIsStillDelivered(t *testing.T) {
	sys := newTestSystem(t)
	ref, err := sys.SpawnNamed(func() IActor { return &startCounter{} }, "poisonable")
	if err != nil {
		t.Fatal(err)
	}
	body, err := proto.Marshal(msgPoison)
	if err != nil {
		t.Fatal(err)
	}
	sys.RecvEnvelope(&remote.Envelope{
		Target:  ref.GetId(),
		MsgName: string(proto.MessageName(msgPoison)),
		Content: body,
		MsgSnId: 1000,
	})
	waitFor(t, func() bool { return sys.registry.get(ref) == nil }, "remote poison to stop the actor")
}

type startCounter struct {
	BaseActor
	starts atomic.Int64
}

func (a *startCounter) Started()            { a.starts.Add(1) }
func (a *startCounter) PreStop()            {}
func (a *startCounter) Receive(ctx Context) {}

// A fire-and-forget Tell to a target that is no longer registered used to vanish into an ERROR log
// line, invisible to DeadLetterHandler. Only an Ask sender was notified, and the narrow stop window
// processorMailBox.send covers is not the common case — a target that already deregistered is.
func TestTellToMissingActorDeadLetters(t *testing.T) {
	sys := newTestSystem(t)
	got := make(chan DeadLetter, 16)
	sys.config.deadLetterHandler = func(d DeadLetter) { got <- d }

	ref, err := sys.SpawnNamed(func() IActor { return &sink{} }, "gone")
	if err != nil {
		t.Fatal(err)
	}
	sys.Poison(ref)
	waitFor(t, func() bool { return sys.registry.get(ref) == nil }, "actor to deregister")
	for len(got) > 0 { // discard anything the stop itself produced
		<-got
	}

	sys.tellWithSender(ref, &message.Unsubscribe{EventName: "x"}, nil, 7)
	select {
	case d := <-got:
		if d.Reason != DeadLetterReasonNotFound {
			t.Errorf("reason = %q, want %q", d.Reason, DeadLetterReasonNotFound)
		}
		if d.Target == nil || d.Target.GetId() != ref.GetId() {
			t.Errorf("target = %v, want %v", d.Target, ref)
		}
		if d.MsgSnId != 7 {
			t.Errorf("msgSnId = %d, want 7", d.MsgSnId)
		}
	case <-time.After(time.Second):
		t.Fatal("a Tell to a deregistered actor produced no dead letter")
	}
}

// Same for a cluster ref whose kind no node hosts: decidable locally, so it must not be a silent
// drop either.
func TestTellToUnhostedKindDeadLetters(t *testing.T) {
	sys := newTestSystem(t)
	got := make(chan DeadLetter, 4)
	sys.config.deadLetterHandler = func(d DeadLetter) { got <- d }

	// the fakeProvider hosts "player" only
	sys.tellWithSender(sys.GetClusterActorRef("nosuchkind", "1"),
		&message.Unsubscribe{EventName: "x"}, nil, 9)
	select {
	case d := <-got:
		if d.Reason != DeadLetterReasonNoOwner {
			t.Errorf("reason = %q, want %q", d.Reason, DeadLetterReasonNoOwner)
		}
	case <-time.After(time.Second):
		t.Fatal("a Tell to an unhosted cluster kind produced no dead letter")
	}
}

// streamWriteActor self-poisons when its stream fails to open, but poison() only sets a flag, so
// the run loop still drains what is queued. Those messages used to hit a nil x.remote, panic, and
// be swallowed by invoke's recover: never sent, never dead-lettered, and a waiting Ask burned its
// whole askTimeout.
func TestStreamWriteWithNoStreamDeadLetters(t *testing.T) {
	sys := newTestSystem(t)
	got := make(chan DeadLetter, 4)
	sys.config.deadLetterHandler = func(d DeadLetter) { got <- d }

	peer := "10.255.255.1:59999"
	wsRef := newDirectActorRef(defaultWriteStreamKind, peer, sys.getAddr(), sys)
	w := &streamWriteActor{router: wsRef, address: peer} // Started() never ran: conn/remote nil
	w._init(wsRef)

	target := newDirectActorRef("player", "g1", peer, sys)
	w.Receive(newContext(target, nil, &message.Unsubscribe{EventName: "x"}, 11, sys))

	select {
	case d := <-got:
		if d.Reason != DeadLetterReasonUnreachable {
			t.Errorf("reason = %q, want %q", d.Reason, DeadLetterReasonUnreachable)
		}
	case <-time.After(time.Second):
		t.Fatal("a send over an unopened stream produced no dead letter")
	}
}

// An actor id is "type/kind/name[@addr]", so a name carrying a separator parses into a different
// id than it was built from — and when the piece count stops matching, into an id with every field
// empty. Such a ref used to be accepted silently and was then a permanent black hole: isCluster()
// false, address lookup "", every send dropped, no node able to activate it.
func TestGetClusterActorRefRejectsSeparatorsInName(t *testing.T) {
	sys := newTestSystem(t)
	for _, name := range []string{"tenant/42", "a@b", "", "/", "x/"} {
		t.Run("name="+name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("GetClusterActorRef(%q) did not panic; it would have returned an "+
						"unroutable ref", name)
				}
				if s, ok := r.(string); !ok || !strings.Contains(s, "invalid actor name") {
					t.Fatalf("panic did not name the problem: %v", r)
				}
			}()
			sys.GetClusterActorRef("player", name)
		})
	}
}

// A valid name must still produce a fully parsed, routable ref.
func TestGetClusterActorRefAcceptsOrdinaryNames(t *testing.T) {
	sys := newTestSystem(t)
	for _, name := range []string{"1234567890123456789", "room-abc", "player_1", "tenant.42", "a:b"} {
		ref := sys.GetClusterActorRef("player", name)
		if ref.GetKind() != "player" || ref.GetName() != name || !ref.isCluster() {
			t.Errorf("name %q: kind=%q name=%q isCluster=%v", name,
				ref.GetKind(), ref.GetName(), ref.isCluster())
		}
	}
}

// SpawnNamed has an error to return, so it returns one rather than panicking.
func TestSpawnNamedRejectsSeparatorsInName(t *testing.T) {
	sys := newTestSystem(t)
	for _, name := range []string{"a/b", "a@b", ""} {
		ref, err := sys.SpawnNamed(func() IActor { return &sink{} }, name)
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("SpawnNamed(%q): err = %v, want ErrInvalidName", name, err)
		}
		if ref != nil {
			t.Errorf("SpawnNamed(%q) returned a ref alongside the error", name)
		}
	}
}

func TestValidateActorName(t *testing.T) {
	for _, name := range []string{"a", "1", "room-1", "a.b", "a:b", "a_b"} {
		if err := ValidateActorName(name); err != nil {
			t.Errorf("ValidateActorName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"", "/", "@", "a/b", "a@b", "a/b@c"} {
		if err := ValidateActorName(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateActorName(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

// goroutineAsker commits the documented misuse: it calls Ask from a goroutine it spawned, so the
// yieldTurn/resumeTurn pair runs on a goroutine that never held the turn. The turn token then gets
// released twice, and releaseTurn must say so instead of letting the actor quietly de-serialise.
type goroutineAsker struct {
	BaseActor
	fired chan struct{}
}

func (a *goroutineAsker) Started() {}
func (a *goroutineAsker) PreStop() {}
func (a *goroutineAsker) Receive(ctx Context) {
	if _, ok := ctx.Message().(*message.Unsubscribe); !ok {
		return
	}
	go func() {
		// wrong: Ask belongs on the handler's own goroutine
		_, _ = a.Ask[*message.Subscribe](a.Self(), &message.Subscribe{EventName: "x"})
		close(a.fired)
	}()
	time.Sleep(150 * time.Millisecond) // still "running" while the goroutine yields our turn
}

func TestUnmatchedTurnReleaseIsReported(t *testing.T) {
	var buf lockedBuffer
	sys := newTestSystem(t)
	sys.logger.Store(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))

	a := &goroutineAsker{fired: make(chan struct{})}
	ref, err := sys.SpawnNamed(func() IActor { return a }, "goroutine-asker")
	if err != nil {
		t.Fatal(err)
	}
	sys.tellWithSender(ref, &message.Unsubscribe{EventName: "go"}, nil, 1)
	select {
	case <-a.fired:
	case <-time.After(5 * time.Second):
	}
	waitFor(t, func() bool { return strings.Contains(buf.String(), "never held it") },
		"the framework to report the broken single-threading")
}

// lockedBuffer is a race-free io.Writer for capturing log output.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
