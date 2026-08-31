package gonet

import (
	"bytes"
	"net"
	"sort"
	"sync/atomic"
)

// innerIPsCache holds the enumerated addresses once a NON-EMPTY answer has been found.
var innerIPsCache atomic.Pointer[[]net.IP]

// innerIPs returns the host's usable non-loopback IPv4 addresses, ordered so the first
// is the one to advertise to the cluster.
//
// Enumerated on first use, not at package load. It used to run in init(), which froze
// the answer before main: a host that acquires its address later — DHCP lease still
// pending, container network being attached, VPN coming up — kept an empty list forever,
// so GetTopInnerIP returned nil and the rpc server silently advertised 127.0.0.1 to the
// cluster. Peers could then never dial this node, with no error logged anywhere.
//
// A FAILURE IS NOT CACHED. Caching an empty result (as sync.OnceValue would) reproduces
// exactly that freeze, merely later: GetTopInnerIP is exported, so a caller that invokes
// it before the network is up would poison the cache for the subsequent
// RpcService.Start(). Only a non-empty answer is memoised; until then every call
// re-enumerates, which costs a few syscalls on a path that runs once per process.
//
// Concurrent callers may enumerate at the same time and both store; the results are
// equivalent, so last-writer-wins is fine.
func innerIPs() []net.IP {
	if cached := innerIPsCache.Load(); cached != nil {
		return *cached
	}
	found := enumerateInnerIPs()
	if len(found) == 0 {
		return nil
	}
	innerIPsCache.Store(&found)
	return found
}

func enumerateInnerIPs() []net.IP {
	var inners []net.IP
	// 用 net.Interfaces() 而非 net.InterfaceAddrs()，后者会丢失地址与网卡的归属关系，
	// 导致挂在环回口上的地址（如 WSL2 给 lo 加的 10.255.255.254/32）被误当成内网网卡 IP
	ifaces, err := net.Interfaces()
	if err != nil {
		// 受限网络环境（seccomp/异常 netns）下返回空，由调用方（如 rpc server）回退到
		// loopback，而不是让整个进程起不来。
		return nil
	}

	// 这里是**接口级**过滤，和下面 selectInnerIPs 的**地址级**过滤不重复：
	//
	//   - FlagUp: 「接口是否启用」无法从 IP 地址推导出来，只能在这一层判断。
	//   - FlagLoopback: 看似和 ip4.IsLoopback() 重复，其实抓的是不同情况 ——
	//     环回口上可能挂着**非 127** 的地址（WSL2 会给 lo 加 10.255.255.254/32），
	//     而 IsLoopback() 对它返回 false，只有接口级检查能排掉。删掉这行的后果实测
	//     过：那个地址是 RFC1918 私网地址，会和真实网卡竞争排序 —— eth0 若是
	//     172.17.x 或 192.168.x，它就会排到第一位被广告出去，对端根本连不上。
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip4 net.IP
			switch ipaddr := addr.(type) {
			case *net.IPNet:
				ip4 = ipaddr.IP.To4()
			case *net.IPAddr:
				ip4 = ipaddr.IP.To4()
			}
			if ip4 == nil {
				continue
			}
			inners = append(inners, ip4)
		}
	}
	return selectInnerIPs(inners)
}

// selectInnerIPs filters and orders candidate IPv4 addresses. Split out from the
// enumeration above purely so it is testable without real network interfaces.
//
// This is **address-level** filtering. The interface-level checks above are not
// duplicated here (an address cannot tell you whether its interface is up, nor that it
// was attached to lo). The IsLoopback() check below is the one genuine overlap: the
// enumeration has already dropped the whole loopback interface, so it is belt-and-braces
// — kept because this function is standalone and must be correct for any input handed to
// it, including by tests.
func selectInnerIPs(candidates []net.IP) []net.IP {
	out := make([]net.IP, 0, len(candidates))
	for _, ip := range candidates {
		ip4 := ip.To4()
		if ip4 == nil {
			continue // IPv6-only address
		}
		if ip4.IsLoopback() {
			continue
		}
		// 跳过 link-local 169.254.0.0/16 (APIPA)：那是「没拿到 DHCP 租约」时自分配的
		// 地址，别的节点路由不到。此前没有过滤，于是一台没租约的主机会把 APIPA 地址
		// 广告给集群。
		if ip4.IsLinkLocalUnicast() {
			continue
		}
		if ip4.IsUnspecified() || ip4.IsMulticast() {
			continue
		}
		out = append(out, ip4)
	}

	// 确定性排序：RFC1918 私网地址优先（集群节点间通常走内网），其次按字节序，
	// 避免多网卡（eth0 + docker0/br-* + VPN）时因网卡枚举顺序不定而广播出错误地址。
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := isPrivate(out[i]), isPrivate(out[j])
		if pi != pj {
			return pi // private 排前
		}
		return bytes.Compare(out[i], out[j]) < 0
	})
	return out
}

// isPrivate reports whether ip is an RFC1918 private IPv4 address.
func isPrivate(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	switch {
	case ip4[0] == 10:
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		return true
	case ip4[0] == 192 && ip4[1] == 168:
		return true
	default:
		return false
	}
}

// GetTopInnerIP returns the preferred inner IPv4 (RFC1918 private first,
// deterministic), or nil when the host has no usable non-loopback IPv4.
//
// The host is enumerated on the first call and cached from then on.
func GetTopInnerIP() net.IP {
	inners := innerIPs()
	if len(inners) == 0 {
		return nil
	}
	return inners[0]
}
