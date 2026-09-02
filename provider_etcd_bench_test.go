package grain

import (
	"io"
	"log/slog"
	"strconv"
	"testing"

	"go.etcd.io/etcd/api/v3/mvccpb"
)

// discardLogger keeps the member-change logging out of the measurement.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// A cluster ref built per message — which is the documented pattern, since GetClusterActorRef
// keys on things like a player id — starts with an empty address cache, so every send resolves
// the owning node from scratch. That path must not copy the node table.
func benchProvider(tb testing.TB, n int) *providerEtcd {
	tb.Helper()
	p := &providerEtcd{config: newConfig("bench", "0.0.1", nil)}
	p.logger = discardLogger()
	p.nodeMap = make(map[string]tNodeState)
	p.localProviderVersion.Store(1)
	for i := 0; i < n; i++ {
		id := uint64(i + 1)
		p.nodeMap[strconv.FormatUint(id, 10)] = tNodeState{
			NodeId: id, Address: "10.0.0." + strconv.FormatUint(id, 10) + ":5001",
			Version: "0.0.1", Time: "2026-09-02 10:00:00",
			Kinds: []string{"player", "room", "guild"},
		}
	}
	p.publishNodesLocked()
	return p
}

// GetNodes must hand out the shared snapshot: lock-free and allocation-free.
func BenchmarkProviderGetNodes(b *testing.B) {
	p := benchProvider(b, 10)
	b.ReportAllocs()
	for b.Loop() {
		nodes, _ := p.GetNodes()
		if len(nodes) != 10 {
			b.Fatal("bad snapshot")
		}
	}
}

// The full per-message cluster address resolution: fresh ref, empty cache, cold every time.
func BenchmarkClusterAddrResolveColdRef(b *testing.B) {
	sys := newTestSystemTB(b)
	p := benchProvider(b, 10)
	sys.clusterProvider = p
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		ref := newClusterActorRef("player", "1234567890123456789", sys)
		if ref.getRemoteAddrCache() == "" {
			b.Fatal("no owner")
		}
	}
}

// A membership change must invalidate cached addresses while costing ONE snapshot rebuild for the
// whole node, not one node-table copy per live ref. Measures the write + re-resolve cycle, so the
// number includes parseWatch's JSON decode; it is a no-regression guard, not a read-path figure.
func BenchmarkMembershipChangeAndReresolve(b *testing.B) {
	sys := newTestSystemTB(b)
	p := benchProvider(b, 10)
	sys.clusterProvider = p
	ref := newClusterActorRef("player", "1234567890123456789", sys)
	state := []byte(`{"NodeId":99,"Address":"10.0.0.99:5001","Version":"0.0.1",` +
		`"Time":"2026-09-02 10:00:00","Kinds":["player"]}`)
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		// flip one member so the cached version is stale on every iteration
		op := mvccpb.PUT
		if i%2 == 1 {
			op = mvccpb.DELETE
		}
		p.parseWatch(op, "/bench/member/99", state)
		if ref.getRemoteAddrCache() == "" {
			b.Fatal("no owner")
		}
	}
}
