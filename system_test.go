package grain

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/chenxyzl/grain/message"
)

func BenchmarkCalcPos(b *testing.B) {
	x := &system{addrHash: newAddrHash()}
	var clusterNodes []tNodeState
	for i := 0; i < 20; i++ {
		clusterNodes = append(clusterNodes, tNodeState{
			NodeId:  uint64(i + 1),
			Address: "aaaaa" + strconv.Itoa(i+1),
			Version: "aaaaa" + strconv.Itoa(i+1),
			Time:    "aaaaa" + strconv.Itoa(i+1),
			Kinds:   []string{"player1", "player2", "player3", "player4", "player5"},
		})
	}
	b.ResetTimer()
	var v string
	for n := 0; n < b.N; n++ {
		tmp := x.getAddrHash().CalcAddrByKind8Name(clusterNodes, "player3", "testname")
		if v == "" {
			v = tmp
		}
		if v != tmp {
			b.Error("CalcAddrByKind8Name failed", v)
		}
	}
}

// TestSystemHotFieldsAreOnSeparateCacheLines guards the padding around askId.
//
// askId takes an atomic.AddUint64 on every send (nextSnId), while addr and logger are READ
// on every send. Co-resident in one 64-byte line, the write invalidated the line for every
// core reading the others — measured at 24.1 vs 14.2 ns/op under 16-core contention.
//
// A comment cannot hold this: adding one field above askId shifts the whole tail and quietly
// undoes it. This asserts the property instead of the byte offsets, so harmless reordering
// stays legal and a regression does not.
func TestSystemHotFieldsAreOnSeparateCacheLines(t *testing.T) {
	const cacheLine = 64
	var s system

	line := func(off uintptr) uintptr { return off / cacheLine }

	askId := unsafe.Offsetof(s.askId)
	for _, other := range []struct {
		name string
		off  uintptr
		size uintptr
	}{
		{"addr", unsafe.Offsetof(s.addr), unsafe.Sizeof(s.addr)},
		{"logger", unsafe.Offsetof(s.logger), unsafe.Sizeof(s.logger)},
		{"draining", unsafe.Offsetof(s.draining), unsafe.Sizeof(s.draining)},
		{"pending", unsafe.Offsetof(s.pending), unsafe.Sizeof(s.pending)},
	} {
		// a field spans lines [off/64, (off+size-1)/64]; askId must be in none of them
		first, last := line(other.off), line(other.off+other.size-1)
		if l := line(askId); l >= first && l <= last {
			t.Errorf("askId (offset %d) shares cache line %d with %s (offset %d, size %d): "+
				"askId is atomically incremented per send and %s is read per send, so this is "+
				"false sharing — check the padding around askId",
				askId, l, other.name, other.off, other.size, other.name)
		}
	}
}

// TestAskToUnhostedClusterKindFailsFast pins that an Ask whose target kind no node hosts
// fails immediately instead of waiting out askTimeout.
//
// tellWithSender's `cacheAddr == ""` arm used to log and return, replying nothing. Every
// other undeliverable path already answers a waiting Ask — sendToLocal replies
// errActorNotFound for an actor that does not exist, toDeadLetter does the same for a
// saturated or stopped mailbox — so this was the one place where a statically decidable
// failure (a typo'd kind name, or a send during the startup window before the member set
// has loaded) was the SLOWEST failure in the system instead of the fastest.
func TestAskToUnhostedClusterKindFailsFast(t *testing.T) {
	sys := newTestSystem(t)
	// Generous on purpose: a correct fast-fail never waits, so a regression shows up as
	// this test timing out rather than quietly passing after askTimeout.
	sys.config.askTimeout = 30 * time.Second

	// the fakeProvider hosts only kind "player"
	target := newClusterActorRef("nobody_hosts_this", "1", sys)

	done := make(chan *message.ErrCode, 1)
	go func() {
		_, err := NoReentryAsk[*message.Unsubscribe](target, &message.Subscribe{EventName: "x"})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an Ask to a kind that no node hosts must fail")
		}
		// Same code as a missing actor, so a caller needs only the one check...
		if !errors.Is(err, message.CodeActorNotFound) {
			t.Errorf("want CodeActorNotFound, got code %d: %q", err.Code, err.Des)
		}
		// ...but a description that names this cause, because the fix is different: a
		// missing WithConfigKind, not a grain that happens to be inactive.
		if !strings.Contains(err.Des, "kind") {
			t.Errorf("the description should say the KIND is unhosted, got %q", err.Des)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Ask to an unhosted cluster kind neither replied nor failed — it is waiting " +
			"out askTimeout, so the fast-fail reply is missing")
	}
}

// And the happy path must be untouched: a kind the cluster DOES host still routes.
func TestAskToHostedClusterKindStillRoutes(t *testing.T) {
	sys := newTestSystem(t)
	ref := newClusterActorRef("player", "42", sys)
	if got := ref.getRemoteAddrCache(); got != sys.getAddr() {
		t.Fatalf("a hosted kind must still resolve to the owning node, got %q want %q",
			got, sys.getAddr())
	}
}
