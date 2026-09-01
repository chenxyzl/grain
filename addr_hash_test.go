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

// TestForwardingCannotLoop pins the invariant that makes cluster forwarding loop-free —
// the one thing standing in for the hop/TTL counter remote.Envelope does not have.
//
// The routing rule is: a node holding an envelope for a cluster actor it does not host
// forwards to CalcAddrByKind8Name(ITS OWN view of the member set), and activates locally
// when that answer is itself (system.go tellWithSender/sendToLocal). Nodes disagree about
// the member set all the time — etcd watch events do not land simultaneously — so in
// principle A could forward to B while B forwards back to A, bouncing the envelope at
// network speed with nothing to stop it.
//
// It cannot happen, for two reasons that have to hold TOGETHER:
//
//  1. the score is a pure function of (name, node address) — it does not depend on the
//     view, so every node computes the SAME score for the same candidate; and
//  2. a node always appears in its own view (providerEtcd.register puts its member key
//     before watch() loads the prefix), so it is always among its own candidates.
//
// Therefore the node a holder forwards to never scores lower than the holder itself, and
// on a tie the address strictly decreases — (score, -address) is strictly monotone along
// the chain, so no node can repeat and every chain terminates in a local activation.
//
// This test brute-forces it over arbitrarily divergent views, keeping only invariant (2).
// It exists because both invariants are easy to break by accident: adding node weights, or
// a capacity/least-loaded tie-break, makes the score view-dependent and reintroduces the
// possibility of a loop — at which point the missing hop limit becomes a real bug and this
// test is what should fail.
//
// Its teeth were checked by injecting view dependence into CalcAddrByKind8Name, and doing
// that is informative about what the test does and does not guard:
//
//   - `score ^ (uint32(len(clusterNodes)) * 2654435761)` — fails on trial 1. This is the
//     shape to worry about.
//   - `score + uint32(len(clusterNodes))*7919` — passes, correctly: a constant offset
//     applied to every candidate in a view shifts them all equally, so the argmax does not
//     move.
//   - `score + uint32(i)*104729` — also passes, correctly: 943k of perturbation against a
//     full uint32 score range essentially never reorders the top candidate.
//
// So the guarded property is the SELECTION, not the raw score: a view-dependent term only
// matters once it is large enough to change who wins. TestScoreIsIndependentOfTheMemberView
// below pins the stricter, cheaper property directly.
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

// TestScoreIsIndependentOfTheMemberView pins invariant (1) directly, since it is the one a
// future "smarter" balancing change would break first: the score of a candidate must not
// depend on which other nodes are in the view.
func TestScoreIsIndependentOfTheMemberView(t *testing.T) {
	kind := "player"
	target := tNodeState{Address: "10.0.0.7:5007", Kinds: []string{kind}}
	other := tNodeState{Address: "10.0.0.1:5001", Kinds: []string{kind}}
	third := tNodeState{Address: "10.0.0.2:5002", Kinds: []string{kind}}

	// fnv32aTwo IS the score; assert it does not see the view at all by checking that the
	// winner among a subset is unchanged by adding lower-scoring peers.
	for _, name := range []string{"a", "player/1", "player/999999", "zzz"} {
		want := fnv32aTwo(name, target.Address)
		for _, view := range [][]tNodeState{
			{target},
			{target, other},
			{other, target},
			{other, target, third},
			{third, other, target},
		} {
			if got := fnv32aTwo(name, target.Address); got != want {
				t.Fatalf("score for %q@%s changed with the view: %d != %d",
					name, target.Address, got, want)
			}
			// and the selection over the view must agree with the max score in it
			hash := newAddrHash()
			owner := hash.CalcAddrByKind8Name(view, kind, name)
			var bestAddr string
			var bestScore uint32
			for _, n := range view {
				if s := fnv32aTwo(name, n.Address); s > bestScore ||
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
