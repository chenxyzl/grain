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

// fnv32aStart/fnv32aAdd split FNV-1a at a resume point, so a fixed prefix can be hashed once
// and continued per candidate. Must stay bit-identical to hash/fnv.New32a: the score decides
// grain ownership, so a silent change re-shards the cluster.
func fnv32aStart(s string) uint32 {
	return fnv32aAdd(fnvOffsetBasis32, s)
}

func fnv32aAdd(h uint32, s string) uint32 {
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= fnvPrime32
	}
	return h
}

// fnv32aTwo names the whole-input hash; the hot path uses the split form above.
func fnv32aTwo(a, b string) uint32 {
	return fnv32aAdd(fnv32aStart(a), b)
}

// fmix32 is murmur3's finalizer. Rendezvous takes the argmax, so it needs well-mixed high
// bits, and FNV-1a's are weak and correlated across inputs sharing a prefix — which every
// candidate is, differing only in the address after the same name. Worst node over 1M keys:
// ±16% -> ±0.4% at 10 nodes, ±23% -> ±4% at 200. Bijective, so it adds no ties.
func fmix32(h uint32) uint32 {
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

// CalcAddrByKind8Name picks the node owning (kind, name) by rendezvous hashing: highest
// fmix32(fnv32a(name+address)) wins, so a membership change only moves the departed node's
// keys. Single pass, zero allocation.
func (x *AddrHash) CalcAddrByKind8Name(clusterNodes []tNodeState, kind string, name string) string {
	// hashed once and resumed per candidate: exact same result, half the bytes hashed
	nameHash := fnv32aStart(name)
	var maxScore uint32
	var maxAddr string
	for i := range clusterNodes {
		// index, not range-by-value: tNodeState is ~80 bytes
		node := &clusterNodes[i]
		if !slices.Contains(node.Kinds, kind) {
			continue
		}
		// unroutable, and it would overload the maxAddr == "" sentinel below. parseWatch only
		// rejects values that fail to parse as JSON, so a member entry can carry one.
		if node.Address == "" {
			continue
		}
		score := fmix32(fnv32aAdd(nameHash, node.Address))
		// Max by score, then min by address: a total order on per-candidate values, so every
		// node agrees regardless of iteration order (GetNodes returns random map order).
		// maxAddr == "" means only "nothing chosen yet", which is what lets a score of 0 win.
		if score > maxScore || (score == maxScore && (maxAddr == "" || node.Address < maxAddr)) {
			maxScore = score
			maxAddr = node.Address
		}
	}
	return maxAddr
}
