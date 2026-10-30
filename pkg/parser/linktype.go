package parser

import "strings"

// AllTypes lists the canonical config type names in display order. The names
// match types.ProxyConfig.Protocol and are what the -types flag, the TUI type
// filter and FilterByTypes all share.
var AllTypes = []string{"vmess", "vless", "trojan", "shadowsocks", "socks", "hysteria2", "wireguard"}

// schemePrefixes maps every supported share-link scheme prefix to its canonical
// type; none of the prefixes is a prefix of another, so order is irrelevant.
var schemePrefixes = []struct{ prefix, typ string }{
	{"vless://", "vless"},
	{"vmess://", "vmess"},
	{"trojan://", "trojan"},
	{"ss://", "shadowsocks"},
	{"socks://", "socks"},
	{"socks5://", "socks"},
	{"hysteria2://", "hysteria2"},
	{"hy2://", "hysteria2"},
	{"wireguard://", "wireguard"},
	{"wg://", "wireguard"},
}

// typeAliases maps alternative -types spellings to canonical names.
var typeAliases = map[string]string{
	"ss":     "shadowsocks",
	"socks5": "socks",
	"hy2":    "hysteria2",
	"wg":     "wireguard",
}

// LinkType classifies a raw share link by its scheme prefix (case-insensitive)
// and reports the canonical type name. A bare "ip:port" entry carries no
// protocol information and is treated as an unauthenticated SOCKS5 proxy,
// matching ParseAndConvert. It reports false for anything v2pingo cannot test.
func LinkType(rawLink string) (string, bool) {
	trimmed := strings.TrimSpace(rawLink)
	lower := strings.ToLower(trimmed)
	for _, p := range schemePrefixes {
		if strings.HasPrefix(lower, p.prefix) {
			return p.typ, true
		}
	}
	if IsBareAddress(trimmed) {
		return "socks", true
	}
	return "", false
}

// NormalizeTypeName resolves a user-supplied type name (flag or UI input) to
// its canonical form, accepting the documented aliases case-insensitively.
func NormalizeTypeName(name string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, typ := range AllTypes {
		if lower == typ {
			return typ, true
		}
	}
	if canonical, ok := typeAliases[lower]; ok {
		return canonical, true
	}
	return "", false
}
