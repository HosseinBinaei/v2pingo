package parser

import "testing"

func TestLinkType(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"vless://uuid@1.2.3.4:443?x=1#n", "vless", true},
		{"vmess://eyJ2IjoiMiJ9", "vmess", true},
		{"trojan://pass@h:443#n", "trojan", true},
		{"ss://YWVzLTI1Ni1nY206cGFzcw@h:8388#n", "shadowsocks", true},
		{"socks://h:1080#n", "socks", true},
		{"socks5://h:1080#n", "socks", true},
		{"hysteria2://pass@h:443#n", "hysteria2", true},
		{"hy2://pass@h:443#n", "hysteria2", true},
		{"wireguard://h:51820#n", "wireguard", true},
		{"wg://h:51820#n", "wireguard", true},
		{"1.2.3.4:1080", "socks", true},
		{"[2001:db8::1]:1080", "socks", true},
		{"VMESS://eyJ2IjoiMiJ9", "vmess", true}, // scheme is case-insensitive
		{"  socks5://h:1080  ", "socks", true},  // leading/trailing spaces tolerated
		{"http://example.com", "", false},
		{"", "", false},
		{"1.2.3.4", "", false},  // no port
		{"1.2.3.4:0", "", false}, // zero port
		{"example.com:1080", "", false},
	}
	for _, tc := range cases {
		got, ok := LinkType(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Errorf("LinkType(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalizeTypeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"vmess", "vmess", true},
		{"SOCKS", "socks", true},
		{"ss", "shadowsocks", true},
		{"Socks5", "socks", true},
		{"hy2", "hysteria2", true},
		{"wg", "wireguard", true},
		{" vless ", "vless", true},
		{"bogus", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeTypeName(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NormalizeTypeName(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCountByType(t *testing.T) {
	raw := []string{
		"socks5://h:1080#1",
		"1.2.3.4:1080",
		"vless://uuid@h:443#2",
		"vless://uuid@h:443#3",
		"vmess://eyJ2IjoiMiJ9",
	}
	got := CountByType(raw)
	want := []TypeCount{
		{Type: "vmess", Count: 1},
		{Type: "vless", Count: 2},
		{Type: "socks", Count: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("CountByType = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CountByType[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if counts := CountByType(nil); len(counts) != 0 {
		t.Errorf("CountByType(nil) = %+v, want empty", counts)
	}
}

func TestFilterByTypes(t *testing.T) {
	raw := []string{
		"socks5://h:1080#1",
		"1.2.3.4:1080",
		"vless://uuid@h:443#2",
		"vmess://eyJ2IjoiMiJ9",
	}
	if got := FilterByTypes(raw, nil); len(got) != len(raw) {
		t.Errorf("nil types must keep all links, got %d", len(got))
	}
	if got := FilterByTypes(raw, []string{}); len(got) != len(raw) {
		t.Errorf("empty types must keep all links, got %d", len(got))
	}
	got := FilterByTypes(raw, []string{"socks"})
	if len(got) != 2 || got[0] != raw[0] || got[1] != raw[1] {
		t.Errorf("socks filter = %v, want the two socks links in order", got)
	}
	got = FilterByTypes(raw, []string{"SOCKS", "vmess"})
	if len(got) != 3 {
		t.Errorf("case-insensitive multi-type filter = %v, want 3 links", got)
	}
	if got := FilterByTypes(raw, []string{"wireguard"}); len(got) != 0 {
		t.Errorf("unmatched type must filter everything out, got %v", got)
	}
}
