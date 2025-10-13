package grain

import (
	"hash"
	"hash/fnv"
	"slices"
	"sync"
)

type AddrHash struct {
	hasher     hash.Hash32
	hasherLock sync.Mutex
}

func newAddrHash() *AddrHash {
	ret := &AddrHash{}
	ret.hasher = fnv.New32a()
	return ret
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
	var maxMember *tNodeState
	var score uint32
	//lock hasher
	x.hasherLock.Lock()
	for _, node := range nodes {
		//
		x.hasher.Reset()
		_, _ = x.hasher.Write(keyBytes)
		_, _ = x.hasher.Write([]byte(node.Address))
		score = x.hasher.Sum32()
		//
		if score > maxScore {
			maxScore = score
			maxMember = &node
		}
	}
	x.hasherLock.Unlock()
	//maxMember will not nil
	if maxMember == nil {
		return ""
	}
	return maxMember.Address
}
