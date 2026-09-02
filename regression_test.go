package grain

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenxyzl/grain/message"
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
