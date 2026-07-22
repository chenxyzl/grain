package grain

import (
	"hash/fnv"
	"slices"
)

type AddrHash struct{}

func newAddrHash() *AddrHash {
	return &AddrHash{}
}

func (x *AddrHash) CalcAddrByKind8Name(clusterNodes []tNodeState, kind string, name string) string {
	var nodes = make([]tNodeState, 0, len(clusterNodes))
	for _, state := range clusterNodes {
		if slices.Contains(state.Kinds, kind) {
			nodes = append(nodes, state)
		}
	}
	l := len(nodes)
	if l == 0 {
		return ""
	}
	if l == 1 {
		return nodes[0].Address
	}
	keyBytes := []byte(name)
	var maxScore uint32
	var maxAddr string
	for _, node := range nodes {
		// local hasher per call: fnv.New32a is a tiny value, avoids the global
		// lock that previously serialized all address computations.
		h := fnv.New32a()
		_, _ = h.Write(keyBytes)
		_, _ = h.Write([]byte(node.Address))
		score := h.Sum32()
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
