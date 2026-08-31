package grain

import (
	"slices"
)

type AddrHash struct{}

func newAddrHash() *AddrHash {
	return &AddrHash{}
}

// FNV-1a 32-bit constants, matching hash/fnv's New32a.
const (
	fnvOffsetBasis32 uint32 = 2166136261
	fnvPrime32       uint32 = 16777619
)

// fnv32aTwo is FNV-1a over the concatenation of a and b, without allocating.
//
// Inlined rather than using hash/fnv so the per-node work is a tight loop over two
// strings: the hasher form needed two h.Write([]byte(s)) calls per node, each an
// interface call plus a string->[]byte conversion.
//
// Must stay bit-identical to hash/fnv.New32a — the score decides which node owns a
// cluster actor, so any divergence silently re-shards the whole cluster.
// TestCalcAddrMatchesReferenceImplementation pins that.
func fnv32aTwo(a, b string) uint32 {
	h := fnvOffsetBasis32
	for i := 0; i < len(a); i++ {
		h ^= uint32(a[i])
		h *= fnvPrime32
	}
	for i := 0; i < len(b); i++ {
		h ^= uint32(b[i])
		h *= fnvPrime32
	}
	return h
}

// CalcAddrByKind8Name picks the node that owns (kind, name) by rendezvous hashing:
// the candidate whose fnv32a(name + address) scores highest wins, so a membership
// change only moves the keys that belonged to the departed node.
//
// Single pass, zero allocation. It used to build an intermediate slice of matching
// nodes — copying whole tNodeState structs — which was the function's only allocation
// (1792 B for 20 nodes). This runs on the cluster send path per cache miss, and once
// per actor on every membership change, so at 10k actors that was 10k allocations per
// etcd event.
//
// The old "exactly one candidate" shortcut is gone as redundant: rendezvous over a
// single candidate selects it anyway — score > 0 wins outright, and a score of 0 is
// still taken by the maxAddr == "" tie-break.
func (x *AddrHash) CalcAddrByKind8Name(clusterNodes []tNodeState, kind string, name string) string {
	var maxScore uint32
	var maxAddr string
	for i := range clusterNodes {
		// index, not `for _, state := range`: ranging by value copies each ~80-byte
		// tNodeState.
		node := &clusterNodes[i]
		if !slices.Contains(node.Kinds, kind) {
			continue
		}
		score := fnv32aTwo(name, node.Address)
		// deterministic tie-break: on equal score pick the lexicographically
		// smaller address so every node computes the same owner (GetNodes
		// returns nodes in random map order).
		if score > maxScore || (score == maxScore && (maxAddr == "" || node.Address < maxAddr)) {
			maxScore = score
			maxAddr = node.Address
		}
	}
	return maxAddr
}
