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

// fnv32aStart and fnv32aAdd are FNV-1a split at a resume point: FNV-1a is a streaming hash
// whose state is just the accumulator, so
//
//	fnv32aAdd(fnv32aStart(a), b) == fnv32aTwo(a, b)
//
// bit for bit. That matters because CalcAddrByKind8Name hashes the SAME name against every
// candidate address: hashing the name once and resuming per candidate halves the bytes it
// touches, and being an exact identity it cannot move any key to a different node.
// Re-hashing the name per candidate cost 589 -> 332 ns/op over 20 candidates (5784 -> 3232 at
// 200), i.e. ~44% of the function.
//
// Written out rather than using hash/fnv so the per-node work is a tight loop over one
// string: the hasher form needed an h.Write([]byte(s)) per node, each an interface call plus
// a string->[]byte conversion.
//
// These must stay bit-identical to hash/fnv.New32a. Not for interop — nothing else reads
// these hashes — but because the score built on them decides which node owns a cluster actor,
// so a silent change re-shards the whole cluster, and matching a stdlib reference is what lets
// TestFnv32aTwoMatchesStdlib and TestCalcAddrMatchesReferenceImplementation detect that.
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

// fnv32aTwo is FNV-1a over the concatenation of a and b. The hot path uses the split form
// above; this names the whole-input hash for the tests that pin it against hash/fnv.
func fnv32aTwo(a, b string) uint32 {
	return fnv32aAdd(fnv32aStart(a), b)
}

// fmix32 is murmur3's finalizer, applied to the FNV-1a score below.
//
// It is not decoration. Rendezvous hashing takes the ARGMAX over per-candidate scores, so it
// needs the top bits to be well mixed — and FNV-1a's last step is just `h ^= c; h *= prime`,
// which leaves the high bits weakly avalanched and correlated between inputs that share a
// prefix. Every candidate here shares the same `name` prefix and differs only in the address
// suffix, which is exactly the worst case: the winner is biased, so keys pile up on some
// nodes.
//
// Measured over 1M keys, deviation from an even share:
//
//	nodes   fnv32a alone        + fmix32
//	   10   +11.2% / -16.5%     +0.3% / -0.4%
//	   50   +14.6% /  -8.9%     +1.3% / -2.0%
//	  200   +23.1% / -15.3%     +3.9% / -3.9%
//
// It is also a bijection — every step is invertible (xor-shift, and multiply by an odd
// constant mod 2^32) — so it relabels scores without ever merging two of them. It cannot add
// ties to the argmax, verified collision-free over 2^22 inputs.
//
// Costs ~3% on its own; the prefix hoist in CalcAddrByKind8Name more than pays for it.
//
// ⚠️ This CHANGES which node owns which key versus any build without it, so it cannot be
// rolled out node-by-node — a cluster running both would disagree about every owner. Deployed
// pre-release, when there was nothing live to coordinate with.
func fmix32(h uint32) uint32 {
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

// CalcAddrByKind8Name picks the node that owns (kind, name) by rendezvous hashing:
// the candidate scoring highest on fmix32(fnv32a(name + address)) wins, so a membership
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
	// Hash the name once and resume per candidate — exactly equivalent to hashing
	// name+address each time (see fnv32aStart), and it halves the bytes hashed.
	nameHash := fnv32aStart(name)
	var maxScore uint32
	var maxAddr string
	for i := range clusterNodes {
		// index, not `for _, state := range`: ranging by value copies each ~80-byte
		// tNodeState.
		node := &clusterNodes[i]
		if !slices.Contains(node.Kinds, kind) {
			continue
		}
		// An empty address cannot be routed to, and admitting one would make the maxAddr
		// sentinel below mean two different things ("nothing chosen yet" and "chose a node
		// whose address is empty"). A member value can carry one: parseWatch only rejects
		// values that fail to parse as JSON, not ones that parse to an empty Address, so a
		// corrupted or legacy entry would otherwise win ~1/N of keys and return "" — which
		// every caller reads as "this kind is hosted nowhere", silently dropping messages
		// for those keys.
		if node.Address == "" {
			continue
		}
		score := fmix32(fnv32aAdd(nameHash, node.Address))
		// Deterministic tie-break: on an equal score pick the lexicographically smaller
		// address, so every node computes the same owner (GetNodes returns nodes in random
		// map order). maxAddr == "" means "nothing chosen yet" and nothing else, since an
		// empty address is skipped above — that is what lets a legitimate score of 0 win.
		//
		// The winner is therefore max-by-score, then min-by-address: a total order on values
		// fixed per candidate, so the result cannot depend on iteration order.
		if score > maxScore || (score == maxScore && (maxAddr == "" || node.Address < maxAddr)) {
			maxScore = score
			maxAddr = node.Address
		}
	}
	return maxAddr
}
