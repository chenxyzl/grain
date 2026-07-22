package gonet

import (
	"bytes"
	"net"
	"sort"
)

var (
	inners []net.IP
)

func init() {
	// 用 net.Interfaces() 而非 net.InterfaceAddrs()，后者会丢失地址与网卡的归属关系，
	// 导致挂在环回口上的地址（如 WSL2 给 lo 加的 10.255.255.254/32）被误当成内网网卡 IP
	ifaces, err := net.Interfaces()
	if err != nil {
		// 不要在包加载期 panic：受限网络环境（seccomp/异常 netns）下会在 main 之前
		// 崩溃且无法 recover。留空 inners，由调用方（如 rpc server）回退到 loopback。
		return
	}

	for _, iface := range ifaces {
		// 跳过未启用及环回接口，环回口上可能挂着非 127 的地址（WSL2 场景）
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

	// 确定性排序：RFC1918 私网地址优先（集群节点间通常走内网），其次按字节序，
	// 避免多网卡（eth0 + docker0/br-* + VPN）时因网卡枚举顺序不定而广播出错误地址。
	sort.SliceStable(inners, func(i, j int) bool {
		pi, pj := isPrivate(inners[i]), isPrivate(inners[j])
		if pi != pj {
			return pi // private 排前
		}
		return bytes.Compare(inners[i], inners[j]) < 0
	})
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
func GetTopInnerIP() net.IP {
	if len(inners) == 0 {
		return nil
	}
	return inners[0]
}
