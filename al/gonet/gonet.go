package gonet

import (
	"bytes"
	"net"
	"sort"
	"sync/atomic"
)

// innerIPsCache holds the enumerated addresses once a NON-EMPTY answer has been found.
var innerIPsCache atomic.Pointer[[]net.IP]

// innerIPs returns the host's usable non-loopback IPv4 addresses, first = the one to advertise
// to the cluster. Enumerated on first use, not at package load, and a FAILURE IS NOT CACHED: a
// host whose address arrives late (DHCP pending, container network attaching, VPN coming up) has
// to be able to recover, and GetTopInnerIP is exported, so an early caller must not poison the
// cache for RpcService.Start(), which would then advertise 127.0.0.1. Concurrent callers may
// both store — equivalent results, so last-writer-wins is fine.
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
	// 用 net.Interfaces() 而非 InterfaceAddrs(): 后者丢失地址与网卡的归属关系, 会把环回口上的
	// 地址(如 WSL2 给 lo 加的 10.255.255.254/32)误当成内网网卡 IP
	ifaces, err := net.Interfaces()
	if err != nil {
		// 受限网络环境(seccomp/异常 netns)下返回空, 由调用方回退到 loopback, 而不是起不来。
		return nil
	}

	// 接口级过滤, 与下面 selectInnerIPs 的地址级过滤不重复: FlagUp 无法从 IP 地址推导; 而环回口
	// 上可能挂着非 127 的地址(WSL2 给 lo 加 10.255.255.254/32), IsLoopback() 对它返回 false, 但
	// 它是 RFC1918 私网地址, 会排到真实网卡前面被广告出去 —— 只有接口级检查能排掉。
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

// selectInnerIPs filters and orders candidate IPv4 addresses; split out from the enumeration so
// it is testable without real network interfaces. ADDRESS-level filtering only (an address cannot
// tell you whether its interface is up, or was lo), plus a belt-and-braces IsLoopback().
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
		// 跳过 link-local 169.254.0.0/16 (APIPA): 没拿到 DHCP 租约时自分配, 别的节点路由不到。
		if ip4.IsLinkLocalUnicast() {
			continue
		}
		if ip4.IsUnspecified() || ip4.IsMulticast() {
			continue
		}
		out = append(out, ip4)
	}

	// 确定性排序: RFC1918 私网优先(集群走内网), 其次按字节序 —— 多网卡时枚举顺序不定, 否则
	// 会广播出错误地址。
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

// GetTopInnerIP returns the preferred inner IPv4 (RFC1918 private first, deterministic), or nil
// when the host has no usable non-loopback IPv4. Enumerated on the first call, then cached.
func GetTopInnerIP() net.IP {
	inners := innerIPs()
	if len(inners) == 0 {
		return nil
	}
	return inners[0]
}
