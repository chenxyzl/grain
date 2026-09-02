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

// TestSystemHotFieldsAreOnSeparateCacheLines guards the padding around askId: it is bumped
// atomically per send while addr/logger are read per send, so sharing a 64-byte line is false
// sharing. Asserts the property, not byte offsets, so harmless reordering stays legal.
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

// TestAskToUnhostedClusterKindFailsFast: an Ask whose target kind no node hosts must fail
// immediately, like every other undeliverable path, instead of waiting out askTimeout.
func TestAskToUnhostedClusterKindFailsFast(t *testing.T) {
	sys := newTestSystem(t)
	// generous on purpose: a regression times out here instead of quietly passing
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
		// same code as a missing actor, so a caller needs only the one check...
		if !errors.Is(err, message.CodeActorNotFound) {
			t.Errorf("want CodeActorNotFound, got code %d: %q", err.Code, err.Des)
		}
		// ...but a description naming the kind, since the fix is a missing WithConfigKind
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
