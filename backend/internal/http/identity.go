package api

import (
	"net"
	"net/netip"
	"strings"
)

// ClientIP 依据受信反代列表解析客户端地址（开发方案 §8.2）。
// 仅当直连来源落在受信前缀内时才采信 X-Forwarded-For，防止伪造头绕过登录限流。
// 解析 X-Forwarded-For 时自右向左跳过受信跳数，取第一个非受信地址为客户端。
func ClientIP(remoteAddr, forwardedFor string, trusted []netip.Prefix) string {
	host := hostOf(remoteAddr)
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if !containsPrefix(trusted, remote) {
		return remote.String()
	}

	hops := splitForwardedFor(forwardedFor)
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(hops[i])
		if err != nil {
			continue
		}
		if !containsPrefix(trusted, addr) {
			return addr.String()
		}
	}
	if len(hops) > 0 {
		return hops[0]
	}
	return remote.String()
}

func hostOf(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func splitForwardedFor(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func containsPrefix(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ParseTrustedProxies 把 CIDR 或裸 IP 字符串解析为前缀列表，非法项直接报错。
func ParseTrustedProxies(raw []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(raw))
	for _, item := range raw {
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, err
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}
