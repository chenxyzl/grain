package grain

import (
	"strings"
	"testing"
)

// parseCacheSplit is the original strings.Split implementation, kept as the oracle the allocation-
// free rewrite is differentially tested against. parseCache also parses UNTRUSTED wire data
// (newActorRefFromAID on every inbound envelope), so behaviour on garbage has to match exactly,
// not just on well-formed ids.
func parseCacheSplit(fullPath string) (string, string, string, string) {
	a1 := strings.Split(fullPath, "/")
	if len(a1) != 3 {
		return "", "", "", ""
	}
	a2 := strings.Split(a1[2], "@")
	name := a2[0]
	var directAddr string
	if len(a2) > 1 {
		directAddr = a2[1]
	}
	return a1[0], a1[1], name, directAddr
}

var parseCacheCases = []string{
	// real shapes
	"cluster/player/1234567890123456789",
	"direct/local/484024768387878912@10.10.108.145:50685",
	"direct/write_stream/192.168.1.44:41234@192.168.1.43:39001",
	"direct/reply/72057594037928034@10.0.0.7:5001",
	"cluster/room/room-abc-def",
	// degenerate and hostile
	"", "/", "//", "///", "@", "garbage", "a/b", "a/b/c/d", "a//c", "/b/c", "a/b/",
	"a/b/c@", "a/b/@d", "a/b/@", "x/y/z@w@v", "a/b/c@d@", "a/b/c@@d",
	"a@b/c/d", "a/b@c/d", "cluster/player/name with spaces",
	"cluster//name", "/kind/name", "a/b/c/d/e", "@/@/@",
}

func TestParseCacheMatchesSplitImplementation(t *testing.T) {
	for _, s := range parseCacheCases {
		wT, wK, wN, wA := parseCacheSplit(s)
		gT, gK, gN, gA := parseCache(s)
		if gT != wT || gK != wK || gN != wN || gA != wA {
			t.Errorf("parseCache(%q)\n got (%q,%q,%q,%q)\nwant (%q,%q,%q,%q)",
				s, gT, gK, gN, gA, wT, wK, wN, wA)
		}
	}
}

// Results must be substrings of the input, which is what makes the parse allocation-free.
func TestParseCacheAllocatesNothing(t *testing.T) {
	allocs := testing.AllocsPerRun(200, func() {
		for _, s := range parseCacheCases {
			parseCache(s)
		}
	})
	if allocs != 0 {
		t.Errorf("parseCache allocates %v times per run, want 0", allocs)
	}
}

func BenchmarkParseCache(b *testing.B) {
	// the well-formed ids, which is what the inbound path actually sees
	cases := parseCacheCases[:5]
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		parseCache(cases[i%len(cases)])
	}
}

func BenchmarkParseCacheSplitOld(b *testing.B) {
	cases := parseCacheCases[:5]
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		parseCacheSplit(cases[i%len(cases)])
	}
}
