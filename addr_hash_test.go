package grain

import (
	"hash/fnv"
	"slices"
	"strconv"
	"testing"
)

// calcAddrReference is the previous implementation, kept verbatim as an oracle. The
// rewrite is a pure optimisation, so any difference in the chosen owner is a bug that
// would silently re-shard a live cluster.
func calcAddrReference(clusterNodes []tNodeState, kind string, name string) string {
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
		h := fnv.New32a()
		_, _ = h.Write(keyBytes)
		_, _ = h.Write([]byte(node.Address))
		score := h.Sum32()
		if score > maxScore || (score == maxScore && (maxAddr == "" || node.Address < maxAddr)) {
			maxScore = score
			maxAddr = node.Address
		}
	}
	return maxAddr
}

// TestFnv32aTwoMatchesStdlib pins the inlined hash against hash/fnv.
func TestFnv32aTwoMatchesStdlib(t *testing.T) {
	cases := []struct{ a, b string }{
		{"", ""},
		{"a", ""},
		{"", "b"},
		{"484024768387878912", "10.10.108.145:50685"},
		{"player-1", "10.0.0.1:1234"},
		{"\x00\xff\x80", "\x7f\x01"},
	}
	for i := range 512 {
		cases = append(cases, struct{ a, b string }{
			strconv.Itoa(i * 7919),
			"10.10." + strconv.Itoa(i%256) + "." + strconv.Itoa((i*3)%256) + ":" + strconv.Itoa(30000+i),
		})
	}
	for _, c := range cases {
		h := fnv.New32a()
		_, _ = h.Write([]byte(c.a))
		_, _ = h.Write([]byte(c.b))
		want := h.Sum32()
		if got := fnv32aTwo(c.a, c.b); got != want {
			t.Fatalf("fnv32aTwo(%q,%q) = %d, stdlib = %d", c.a, c.b, got, want)
		}
	}
}

// TestCalcAddrMatchesReferenceImplementation: the rewrite must choose the same owner as
// the old code for every input shape — no candidates, one candidate, many, mixed kinds.
func TestCalcAddrMatchesReferenceImplementation(t *testing.T) {
	h := newAddrHash()

	mkNodes := func(n int, kinds func(i int) []string) []tNodeState {
		out := make([]tNodeState, 0, n)
		for i := range n {
			out = append(out, tNodeState{
				NodeId:  uint64(i + 1),
				Address: "10.10." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256) + ":" + strconv.Itoa(50000+i),
				Kinds:   kinds(i),
			})
		}
		return out
	}

	sets := map[string][]tNodeState{
		"empty":            {},
		"one match":        mkNodes(1, func(int) []string { return []string{"player"} }),
		"one no-match":     mkNodes(1, func(int) []string { return []string{"other"} }),
		"three all match":  mkNodes(3, func(int) []string { return []string{"player"} }),
		"twenty all match": mkNodes(20, func(int) []string { return []string{"player"} }),
		"mixed kinds": mkNodes(20, func(i int) []string {
			if i%3 == 0 {
				return []string{"player", "room"}
			}
			return []string{"room"}
		}),
		"none match": mkNodes(20, func(int) []string { return []string{"room"} }),
		"empty kinds": mkNodes(5, func(i int) []string {
			if i == 2 {
				return nil
			}
			return []string{"player"}
		}),
	}

	for label, nodes := range sets {
		for i := range 2000 {
			name := strconv.Itoa(i * 104729)
			want := calcAddrReference(nodes, "player", name)
			got := h.CalcAddrByKind8Name(nodes, "player", name)
			if got != want {
				t.Fatalf("%s: name=%q -> %q, reference -> %q", label, name, got, want)
			}
		}
	}
}

// TestCalcAddrIsOrderIndependent: GetNodes returns nodes in random map order, so every
// node in the cluster must still agree on the owner.
func TestCalcAddrIsOrderIndependent(t *testing.T) {
	h := newAddrHash()
	nodes := make([]tNodeState, 0, 12)
	for i := range 12 {
		nodes = append(nodes, tNodeState{
			NodeId:  uint64(i + 1),
			Address: "10.0.0." + strconv.Itoa(i+1) + ":5000",
			Kinds:   []string{"player"},
		})
	}
	shuffled := slices.Clone(nodes)
	for i := range 500 {
		name := strconv.Itoa(i * 31337)
		want := h.CalcAddrByKind8Name(nodes, "player", name)
		// rotate to get a different ordering without a RNG (Math.random is unavailable
		// in some sandboxes and determinism is nicer anyway).
		shuffled = append(shuffled[1:], shuffled[0])
		if got := h.CalcAddrByKind8Name(shuffled, "player", name); got != want {
			t.Fatalf("name=%q: order changed the owner (%q vs %q)", name, got, want)
		}
	}
}
