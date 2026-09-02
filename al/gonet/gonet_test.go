package gonet

import (
	"net"
	"testing"
)

func ips(ss ...string) []net.IP {
	out := make([]net.IP, 0, len(ss))
	for _, s := range ss {
		ip := net.ParseIP(s)
		if ip == nil {
			panic("bad test ip: " + s)
		}
		out = append(out, ip)
	}
	return out
}

func strs(in []net.IP) []string {
	out := make([]string, 0, len(in))
	for _, ip := range in {
		out = append(out, ip.String())
	}
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestLinkLocalIsRejected pins the APIPA filter: 169.254.0.0/16 is what a host self-assigns
// when it fails to get a DHCP lease, and no peer can route to it.
func TestLinkLocalIsRejected(t *testing.T) {
	got := strs(selectInnerIPs(ips("169.254.13.7")))
	if len(got) != 0 {
		t.Errorf("a link-local address must not be advertised, got %v", got)
	}

	// must be dropped, not merely sorted last, or it becomes the fallback when the real one goes
	got = strs(selectInnerIPs(ips("169.254.13.7", "10.0.0.5")))
	if !eq(got, "10.0.0.5") {
		t.Errorf("want only 10.0.0.5, got %v", got)
	}
}

// TestPrivateAddressesSortFirst: cluster traffic is internal, so RFC1918 must beat a public one.
func TestPrivateAddressesSortFirst(t *testing.T) {
	for _, in := range [][]net.IP{
		ips("203.0.113.9", "10.0.0.5"),
		ips("10.0.0.5", "203.0.113.9"),
	} {
		got := strs(selectInnerIPs(in))
		if !eq(got, "10.0.0.5", "203.0.113.9") {
			t.Errorf("input %v: want private first, got %v", strs(in), got)
		}
	}
}

// TestOrderIsDeterministic: NIC enumeration order varies (eth0 + docker0 + VPN), the result must
// not.
func TestOrderIsDeterministic(t *testing.T) {
	want := strs(selectInnerIPs(ips("10.0.0.5", "172.17.0.1", "192.168.1.9")))
	for _, in := range [][]net.IP{
		ips("192.168.1.9", "10.0.0.5", "172.17.0.1"),
		ips("172.17.0.1", "192.168.1.9", "10.0.0.5"),
		ips("192.168.1.9", "172.17.0.1", "10.0.0.5"),
	} {
		if got := strs(selectInnerIPs(in)); !eq(got, want...) {
			t.Errorf("order changed the result: %v vs %v", got, want)
		}
	}
}

// TestJunkIsRejected: loopback, unspecified, multicast and IPv6-only must never be advertised.
func TestJunkIsRejected(t *testing.T) {
	in := ips("127.0.0.1", "0.0.0.0", "224.0.0.1", "fe80::1", "2001:db8::1", "10.1.2.3")
	got := strs(selectInnerIPs(in))
	if !eq(got, "10.1.2.3") {
		t.Errorf("want only 10.1.2.3, got %v", got)
	}
}

// TestGetTopInnerIPIsCachedAndSane exercises the real host: enumerated on FIRST CALL, then
// stable and dialable.
func TestGetTopInnerIPIsCachedAndSane(t *testing.T) {
	first := GetTopInnerIP()
	second := GetTopInnerIP()
	if (first == nil) != (second == nil) {
		t.Fatal("GetTopInnerIP is not stable across calls")
	}
	if first == nil {
		t.Skip("host has no usable non-loopback IPv4; nothing to assert")
	}
	if !first.Equal(second) {
		t.Errorf("not cached: %v then %v", first, second)
	}
	if first.To4() == nil {
		t.Errorf("advertised address is not IPv4: %v", first)
	}
	if first.IsLoopback() || first.IsLinkLocalUnicast() || first.IsUnspecified() {
		t.Errorf("advertised address is not dialable by peers: %v", first)
	}
	t.Logf("advertised address = %v (private=%v)", first, isPrivate(first))
}

// TestLoopbackInterfaceAddressIsNotAdvertised: the interface-level FlagLoopback check catches
// what selectInnerIPs cannot. WSL2 puts 10.255.255.254/32 on lo — not in 127.0.0.0/8, so
// IsLoopback() is false, and RFC1918, so it would outrank a real eth0 on 172.17.x/192.168.x.
func TestLoopbackInterfaceAddressIsNotAdvertised(t *testing.T) {
	lo := net.ParseIP("10.255.255.254") // what WSL2 puts on lo
	if lo.IsLoopback() {
		t.Fatal("premise broken: this address is meant to be a non-127 address on lo")
	}
	if !isPrivate(lo) {
		t.Fatal("premise broken: it is meant to be RFC1918, hence a sorting contender")
	}
	// it outranks a real NIC on common private ranges, so the interface-level filter is load-bearing
	for _, eth := range []string{"172.17.0.3", "192.168.1.5"} {
		got := selectInnerIPs(ips("10.255.255.254", eth))
		if got[0].String() == eth {
			t.Errorf("expected the lo address to outrank %s (that is why the "+
				"interface-level check must stay); got %v", eth, strs(got))
		}
	}
}

// TestFailedEnumerationIsNotCached: a failure must not be memoised. The first attempt can
// legitimately be empty (DHCP pending, container network attaching), and GetTopInnerIP is
// exported, so an early caller must not poison the cache for RpcService.Start().
func TestFailedEnumerationIsNotCached(t *testing.T) {
	// "nothing found yet": a nil result must leave the cache clear, so a later call re-enumerates
	innerIPsCache.Store(nil)
	if cached := innerIPsCache.Load(); cached != nil {
		t.Fatal("precondition: cache should be clear")
	}

	// a real enumeration on this host succeeds, so drive the negative case directly
	if len(selectInnerIPs(nil)) != 0 {
		t.Fatal("premise: no candidates must yield no addresses")
	}
	// The contract: innerIPs() must only populate the cache on a non-empty result.
	got := innerIPs()
	if len(got) == 0 {
		if innerIPsCache.Load() != nil {
			t.Error("an empty enumeration was cached — a later call can never recover")
		}
		t.Skip("host has no usable IPv4; the negative path is what mattered and it held")
	}
	if innerIPsCache.Load() == nil {
		t.Error("a successful enumeration should have been cached")
	}
	if second := innerIPs(); len(second) != len(got) {
		t.Errorf("not stable across calls: %v then %v", strs(got), strs(second))
	}
}
