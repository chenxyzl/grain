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
		score := fmix32(h.Sum32())
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

// TestForwardingCannotLoop pins what makes cluster forwarding loop-free — the stand-in for
// the hop/TTL counter remote.Envelope does not have. A node holding an envelope for a grain it
// does not host forwards to the owner per ITS OWN view, and views diverge constantly, so A
// could in principle forward to B while B forwards back.
//
// It cannot, because two things hold together: the score depends only on (name, address), not
// on the view; and a node always appears in its own view (register puts its member key before
// watch loads the prefix). So each hop is strictly monotone in (score, -address) and no node
// can repeat. Adding node weights or a least-loaded tie-break breaks the first and this test
// is what should fail. It guards the SELECTION, so a view-dependent term only trips it once it
// is large enough to change the argmax.
func TestForwardingCannotLoop(t *testing.T) {
	const (
		nodeCount = 10
		trials    = 20000
		kind      = "player"
	)
	all := make([]tNodeState, nodeCount)
	for i := range all {
		all[i] = tNodeState{
			NodeId:  uint64(i + 1),
			Address: "10.0.0." + strconv.Itoa(i+1) + ":" + strconv.Itoa(5000+i),
			Kinds:   []string{kind},
		}
	}
	hash := newAddrHash()
	// deterministic pseudo-random view/key selection: no rand, so a failure reproduces
	rnd := uint32(2166136261)
	next := func() uint32 { rnd ^= rnd << 13; rnd ^= rnd >> 17; rnd ^= rnd << 5; return rnd }

	longest := 0
	for trial := range trials {
		// one arbitrary view per node; the ONLY thing preserved is "a node sees itself"
		views := make([][]tNodeState, nodeCount)
		for i := range all {
			v := []tNodeState{all[i]}
			for j := range all {
				if j != i && next()%2 == 0 {
					v = append(v, all[j])
				}
			}
			views[i] = v
		}
		name := "player/" + strconv.Itoa(int(next()))

		// walk the forwarding chain from an arbitrary starting node
		cur := int(next()) % nodeCount
		seen := make(map[int]int, nodeCount)
		hops := 0
		for {
			if firstHop, dup := seen[cur]; dup {
				t.Fatalf("forwarding LOOP on trial %d: node %s revisited (first seen at hop %d, "+
					"again at hop %d) for key %q. The (score, -address) monotonicity that makes "+
					"forwarding loop-free is broken — and remote.Envelope has no hop limit to "+
					"catch it, so an envelope would ping-pong between nodes indefinitely",
					trial, all[cur].Address, firstHop, hops, name)
			}
			seen[cur] = hops
			owner := hash.CalcAddrByKind8Name(views[cur], kind, name)
			if owner == all[cur].Address {
				break // this node activates locally: chain ends
			}
			nextIdx := -1
			for j := range all {
				if all[j].Address == owner {
					nextIdx = j
					break
				}
			}
			if nextIdx < 0 {
				t.Fatalf("node %s forwarded to %q, which is not a known node", all[cur].Address, owner)
			}
			cur = nextIdx
			hops++
		}
		if hops > longest {
			longest = hops
		}
	}
	t.Logf("%d trials over arbitrarily divergent views: no loops, longest chain %d hops "+
		"across %d nodes", trials, longest, nodeCount)
}

// TestScoreIsIndependentOfTheMemberView pins the first invariant above directly — it is the one
// a "smarter" balancing change breaks first — and that the selection equals the max score.
func TestScoreIsIndependentOfTheMemberView(t *testing.T) {
	kind := "player"
	target := tNodeState{Address: "10.0.0.7:5007", Kinds: []string{kind}}
	other := tNodeState{Address: "10.0.0.1:5001", Kinds: []string{kind}}
	third := tNodeState{Address: "10.0.0.2:5002", Kinds: []string{kind}}

	score := func(name, addr string) uint32 { return fmix32(fnv32aTwo(name, addr)) }
	for _, name := range []string{"a", "player/1", "player/999999", "zzz"} {
		want := score(name, target.Address)
		for _, view := range [][]tNodeState{
			{target},
			{target, other},
			{other, target},
			{other, target, third},
			{third, other, target},
		} {
			if got := score(name, target.Address); got != want {
				t.Fatalf("score for %q@%s changed with the view: %d != %d",
					name, target.Address, got, want)
			}
			// and the selection over the view must agree with the max score in it
			hash := newAddrHash()
			owner := hash.CalcAddrByKind8Name(view, kind, name)
			var bestAddr string
			var bestScore uint32
			for _, n := range view {
				if s := score(name, n.Address); s > bestScore ||
					(s == bestScore && (bestAddr == "" || n.Address < bestAddr)) {
					bestScore, bestAddr = s, n.Address
				}
			}
			if owner != bestAddr {
				t.Fatalf("selection %q disagrees with the max score %q for key %q",
					owner, bestAddr, name)
			}
		}
	}
}

// TestOwnershipIsEvenlySpread is the property fmix32 buys. Few nodes on purpose: spread needs
// many keys PER node or sampling noise swamps it (200 nodes / 200k keys is ~1000 each, where
// ~3% is pure noise); 10 nodes gives 20k each.
func TestOwnershipIsEvenlySpread(t *testing.T) {
	const (
		nodeCount = 10
		keys      = 200000
		// 0.31% with fmix32, 4.3% without (worse with scattered addresses; these fixed ones
		// keep it deterministic). The bound sits between.
		maxDeviationPct = 3.0
	)
	nodes := make([]tNodeState, nodeCount)
	for i := range nodes {
		nodes[i] = tNodeState{
			NodeId:  uint64(i + 1),
			Address: "10.10.0." + strconv.Itoa(i) + ":" + strconv.Itoa(50000+i),
			Kinds:   []string{"player"},
		}
	}

	hash := newAddrHash()
	count := make(map[string]int, nodeCount)
	for k := range keys {
		count[hash.CalcAddrByKind8Name(nodes, "player", "player/"+strconv.Itoa(k))]++
	}

	mean := float64(keys) / float64(nodeCount)
	worst := 0.0
	for _, n := range nodes {
		dev := (float64(count[n.Address])/mean - 1) * 100
		if dev < 0 {
			dev = -dev
		}
		if dev > worst {
			worst = dev
		}
		if dev > maxDeviationPct {
			t.Errorf("node %s holds %d keys, %.1f%% off an even share of %.0f — ownership is "+
				"skewed, which means the score's high bits are not avalanching (is fmix32 "+
				"still applied?)", n.Address, count[n.Address], dev, mean)
		}
	}
	t.Logf("%d nodes / %d keys: worst node is %.2f%% off an even share", nodeCount, keys, worst)
}

// TestFmix32MatchesMurmur3 pins the constants and shifts against a published vector:
// MurmurHash3_x86_32("", seed=1) is 0x514E28B7, which for an empty key reduces to fmix32(1).
// A typo'd constant is invisible otherwise — it still looks random and still distributes
// evenly, so the spread test above passes; it just re-shards every key.
func TestFmix32MatchesMurmur3(t *testing.T) {
	if got := fmix32(1); got != 0x514e28b7 {
		t.Errorf("fmix32(1) = %#x, want 0x514e28b7 (MurmurHash3_x86_32(\"\", seed=1)) — a "+
			"constant or shift in the finalizer is wrong", got)
	}
	// 0 is a fixed point: every step of the finalizer maps 0 to 0.
	if got := fmix32(0); got != 0 {
		t.Errorf("fmix32(0) = %#x, want 0", got)
	}
}

// An empty Address must never be selected: it is unroutable, and callers read "" as "this kind
// is hosted nowhere". A member entry can carry one (parseWatch only drops unparseable JSON).
func TestEmptyAddressIsNeverSelected(t *testing.T) {
	h := newAddrHash()
	nodes := []tNodeState{
		{NodeId: 1, Address: "", Kinds: []string{"player"}},
		{NodeId: 2, Address: "10.0.0.2:5000", Kinds: []string{"player"}},
	}
	for i := range 2000 {
		if got := h.CalcAddrByKind8Name(nodes, "player", strconv.Itoa(i)); got != "10.0.0.2:5000" {
			t.Fatalf("name=%d selected %q; an empty address is unroutable, and returning it "+
				"reads to callers as \"kind hosted nowhere\"", i, got)
		}
	}
	// and with ONLY an empty-address node there is genuinely no owner
	if got := h.CalcAddrByKind8Name(nodes[:1], "player", "x"); got != "" {
		t.Errorf("want no owner, got %q", got)
	}
}
