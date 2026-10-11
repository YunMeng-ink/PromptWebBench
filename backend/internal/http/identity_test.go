package api

import (
	"net/netip"
	"testing"
)

func mustPrefixes(t *testing.T, raw ...string) []netip.Prefix {
	t.Helper()
	prefixes, err := ParseTrustedProxies(raw)
	if err != nil {
		t.Fatalf("解析受信反代失败: %v", err)
	}
	return prefixes
}

func TestClientIPWithoutTrustedProxies(t *testing.T) {
	// 无受信反代时必须忽略 X-Forwarded-For，否则可伪造头绕过按地址限流。
	got := ClientIP("203.0.113.9:44321", "1.2.3.4", nil)
	if got != "203.0.113.9" {
		t.Fatalf("客户端地址 = %s，期望 203.0.113.9", got)
	}
}

func TestClientIPWithTrustedProxy(t *testing.T) {
	trusted := mustPrefixes(t, "127.0.0.1/32")

	cases := []struct {
		name     string
		remote   string
		forward  string
		expected string
	}{
		{"单跳", "127.0.0.1:5000", "198.51.100.7", "198.51.100.7"},
		{"多跳取最右非受信", "127.0.0.1:5000", "198.51.100.7, 127.0.0.1", "198.51.100.7"},
		{"右侧首个非受信即客户端", "127.0.0.1:5000", "127.0.0.2, 127.0.0.3", "127.0.0.3"},
		{"无转发头时退回直连地址", "127.0.0.1:5000", "", "127.0.0.1"},
		{"非法跳数被跳过", "127.0.0.1:5000", "not-an-ip, 198.51.100.7", "198.51.100.7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClientIP(tc.remote, tc.forward, trusted); got != tc.expected {
				t.Fatalf("客户端地址 = %s，期望 %s", got, tc.expected)
			}
		})
	}
}

func TestClientIPAllHopsTrustedTakesLeftmost(t *testing.T) {
	trusted := mustPrefixes(t, "127.0.0.0/8")
	// 转发链上全部跳数都受信时，取最左地址，它是链上记录的原始客户端。
	got := ClientIP("127.0.0.1:5000", "127.0.0.2, 127.0.0.3", trusted)
	if got != "127.0.0.2" {
		t.Fatalf("客户端地址 = %s，期望 127.0.0.2", got)
	}
}

func TestClientIPIgnoresForwardedFromUntrustedPeer(t *testing.T) {
	trusted := mustPrefixes(t, "10.0.0.0/8")
	// 直连来源不在受信前缀内，转发头必须被忽略。
	got := ClientIP("198.51.100.20:1234", "10.0.0.1", trusted)
	if got != "198.51.100.20" {
		t.Fatalf("客户端地址 = %s，期望 198.51.100.20", got)
	}
}

func TestParseTrustedProxiesRejectsGarbage(t *testing.T) {
	if _, err := ParseTrustedProxies([]string{"999.1.1.1"}); err == nil {
		t.Fatal("非法地址应当报错")
	}
	if _, err := ParseTrustedProxies([]string{"10.0.0.0/99"}); err == nil {
		t.Fatal("非法前缀长度应当报错")
	}
}
