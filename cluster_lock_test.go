package grain

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/ringbuffer"
)

var errFakeRegister = errors.New("fake: etcd said no")

// newTestProcessorWithOpts mirrors newTestProcessor but takes the tOpts verbatim, so a
// test can supply its own registerToCluster / unRegisterFromCluster and a cluster self-ref.
func newTestProcessorWithOpts(sys *fakeSys, r IActor, mailbox int, opts tOpts) *processorMailBox {
	p := &processorMailBox{
		system:     sys,
		rb:         ringbuffer.New[Context](int64(mailbox), int64(mailbox)*1024),
		procStatus: idle,
		turn:       make(chan struct{}, 1),
		receiver:   r,
	}
	p.tOpts = opts
	p.turn <- struct{}{}
	p.receiver._init(opts._self)
	p.receiver._bindTurn(p)
	p.rb.Push(newContext(opts._self, opts._self, msgInitialize, sys.nextSnId(), nil))
	sys.reg.lookup.Set(opts._self.GetId(), p)
	return p
}

func (x *processorMailBox) procStatusIs(want int32) bool {
	return atomic.LoadInt32(&x.procStatus) == want
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting: %s", what)
}

// txnProvider implements just the two txn primitives, with etcd's real semantics: setTxn
// is create-only (If CreateRevision == 0 Then Put), removeTxn is compare-value-then-delete
// (If Value == val Then Delete). Everything else is the embedded nil iProvider and would
// panic if these tests touched it.
type txnProvider struct {
	iProvider
	kv map[string]string
}

func newTxnProvider() *txnProvider { return &txnProvider{kv: map[string]string{}} }

func (f *txnProvider) setTxn(key, val string) bool {
	if _, exists := f.kv[key]; exists {
		return false
	}
	f.kv[key] = val
	return true
}

func (f *txnProvider) removeTxn(key, val string) bool {
	if cur, exists := f.kv[key]; !exists || cur != val {
		return false
	}
	delete(f.kv, key)
	return true
}

// nodeCfg is a config as it looks on a node hosting kind "player" at addr.
func nodeCfg(addr string) *config {
	c := newConfig("c", "v", nil)
	c.state.Address = addr
	c.state.Kinds = []string{"player"}
	return c
}

// TestGrainRegistrationIsExclusive pins the single-activation guarantee: the key is
// created with a create-only txn, so two nodes racing to activate the same cluster actor
// cannot both win.
func TestGrainRegistrationIsExclusive(t *testing.T) {
	prov := newTxnProvider()
	nodeA, nodeB := nodeCfg("10.0.0.1:5000"), nodeCfg("10.0.0.2:5000")
	ref := newClusterActorRef("player", "123", nil)

	if err := defaultRegisterToCluster(prov, nodeA, ref); err != nil {
		t.Fatalf("node A must win the registration: %v", err)
	}
	if err := defaultRegisterToCluster(prov, nodeB, ref); err == nil {
		t.Fatal("node B must LOSE while A holds the grain — this txn is the only thing " +
			"keeping a cluster grain single-activation")
	}
}

// TestGrainLockValueIdentifiesTheOwner: the lock value must be the owning node's address,
// because removeTxn compares against it. It used to be ref.GetDirectAddr(), which is ""
// for a cluster ref ("cluster/kind/name" has no "@addr"), so every node wrote the same
// empty string and the compare-and-delete matched unconditionally.
func TestGrainLockValueIdentifiesTheOwner(t *testing.T) {
	prov := newTxnProvider()
	nodeA := nodeCfg("10.0.0.1:5000")
	ref := newClusterActorRef("player", "123", nil)

	if err := defaultRegisterToCluster(prov, nodeA, ref); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := prov.kv[nodeA.getActorRegisterName(ref)]; got != "10.0.0.1:5000" {
		t.Errorf("lock value = %q, want the owning node's address; %q would make "+
			"removeTxn's compare-and-delete match any node", got, "")
	}
}

// TestLoserCannotDeleteWinnersGrainLock is the regression for the double-activation bug.
//
// Node B loses the race, so start() stops it, and stop()'s deferred unregister used to run
// regardless. With the old empty-string lock value that compare-and-delete matched, so B
// deleted the lock A was holding: the grain was then unlocked in etcd while A still hosted
// it, the next activation anywhere succeeded, and the same grain ran twice with each copy
// persisting its own state.
//
// Two independent fixes, so this asserts the outcome both of them protect.
func TestLoserCannotDeleteWinnersGrainLock(t *testing.T) {
	prov := newTxnProvider()
	nodeA, nodeB := nodeCfg("10.0.0.1:5000"), nodeCfg("10.0.0.2:5000")
	ref := newClusterActorRef("player", "123", nil)
	key := nodeA.getActorRegisterName(ref)

	if err := defaultRegisterToCluster(prov, nodeA, ref); err != nil {
		t.Fatalf("node A register: %v", err)
	}
	if err := defaultRegisterToCluster(prov, nodeB, ref); err == nil {
		t.Fatal("node B should have lost the race")
	}
	// Fix 1: even if the loser does attempt an unregister (an un-upgraded peer, or any
	// other path that reaches it), the value no longer matches so the delete is refused.
	_ = defaultUnregisterFromCluster(prov, nodeB, ref)

	if _, held := prov.kv[key]; !held {
		t.Fatalf("node B deleted node A's grain lock (%q is gone). A still hosts the "+
			"actor, so the grain is now unlocked and a second activation elsewhere "+
			"would succeed — the same grain would run twice", key)
	}
	// and the owner can still release its own lock
	if err := defaultUnregisterFromCluster(prov, nodeA, ref); err != nil {
		t.Errorf("the owning node must still be able to unregister: %v", err)
	}
	if _, held := prov.kv[key]; held {
		t.Error("owner's unregister did not release the lock")
	}
}

// TestFailedRegistrationSkipsUnregister covers fix 2 at the processor level: `registered`
// gates stop()'s unregister the same way lifeStarted gates PreStop. A transient etcd error
// (setTxn returns false without the key being taken) must not lead to an unregister for a
// lock this actor never held.
func TestFailedRegistrationSkipsUnregister(t *testing.T) {
	sys := newFakeSys()
	sys.cfg = nodeCfg("10.0.0.1:5000")

	var unregisterCalls int
	ref := newClusterActorRef("player", "123", sys)
	opts := newOpts(func() IActor { return &replier{} }, withOptsClusterSelf(ref))
	opts.registerToCluster = func(iProvider, *config, ActorRef) error {
		return errFakeRegister
	}
	opts.unRegisterFromCluster = func(iProvider, *config, ActorRef) error {
		unregisterCalls++
		return nil
	}

	p := newTestProcessorWithOpts(sys, &replier{}, 8, opts)
	p.init()
	waitFor(t, func() bool { return p.procStatusIs(stopped) },
		"a failed cluster registration must stop the actor")

	if unregisterCalls != 0 {
		t.Errorf("unregister ran %d times after a FAILED registration; it must not touch "+
			"a lock this actor never acquired", unregisterCalls)
	}
}

// TestSuccessfulRegistrationDoesUnregister guards against over-correcting: the gate must
// not suppress the unregister on the normal path, or every stopped grain would leave a
// stale lock behind until the node's lease expired.
func TestSuccessfulRegistrationDoesUnregister(t *testing.T) {
	sys := newFakeSys()
	sys.cfg = nodeCfg("10.0.0.1:5000")

	var unregisterCalls int
	ref := newClusterActorRef("player", "456", sys)
	opts := newOpts(func() IActor { return &replier{} }, withOptsClusterSelf(ref))
	opts.registerToCluster = func(iProvider, *config, ActorRef) error { return nil }
	opts.unRegisterFromCluster = func(iProvider, *config, ActorRef) error {
		unregisterCalls++
		return nil
	}

	p := newTestProcessorWithOpts(sys, &replier{}, 8, opts)
	p.init()
	p.poison()
	waitFor(t, func() bool { return p.procStatusIs(stopped) }, "poison must stop the actor")

	if unregisterCalls != 1 {
		t.Errorf("unregister ran %d times, want exactly 1 on the normal stop path",
			unregisterCalls)
	}
}
