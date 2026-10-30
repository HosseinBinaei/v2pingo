package parser

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"v2pingo/pkg/types"
)

// StripRemark removes the fragment component (#...) from a raw share link.
func StripRemark(rawLink string) string {
	if idx := strings.Index(rawLink, "#"); idx != -1 {
		return rawLink[:idx]
	}
	return rawLink
}

// stripSchemePrefix removes one of the provided scheme prefixes from a share
// link, matching the scheme case-insensitively and returning the remaining body.
func stripSchemePrefix(rawLink string, schemes ...string) (string, bool) {
	lower := strings.ToLower(rawLink)
	for _, scheme := range schemes {
		if strings.HasPrefix(lower, scheme) {
			return rawLink[len(scheme):], true
		}
	}
	return rawLink, false
}

// unescapeUserInfo percent-decodes a share link userinfo component and keeps the
// original value when it is not valid percent-encoded text.
func unescapeUserInfo(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

// splitUserInfo splits a "user:password" userinfo component into its parts.
func splitUserInfo(value string) (username string, password string) {
	username, password, _ = strings.Cut(value, ":")
	return username, password
}

// parsePortNumber parses a single non-zero TCP/UDP port.
func parsePortNumber(value string) (uint16, error) {
	port, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
	if err != nil || port == 0 {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return uint16(port), nil
}

// ParseAndConvert parses a raw proxy share link and converts it to a sing-box
// option.Outbound ProxyConfig (or a sing-box endpoint for WireGuard).
func ParseAndConvert(rawLink string, index int) (*types.ProxyConfig, error) {
	trimmed := strings.TrimSpace(rawLink)
	if trimmed == "" {
		return nil, fmt.Errorf("empty raw link")
	}

	typ, ok := LinkType(trimmed)
	if !ok {
		return nil, fmt.Errorf("unsupported protocol scheme in link: %s", rawLink)
	}
	switch typ {
	case "vless":
		return parseVLESS(trimmed, index)
	case "vmess":
		return parseVMess(trimmed, index)
	case "trojan":
		return parseTrojan(trimmed, index)
	case "shadowsocks":
		return parseShadowsocks(trimmed, index)
	case "socks":
		if IsBareAddress(trimmed) {
			return parseBareSOCKS(trimmed, index)
		}
		return parseSOCKS(trimmed, index)
	case "hysteria2":
		return parseHysteria2(trimmed, index)
	case "wireguard":
		return parseWireGuard(trimmed, index)
	default:
		return nil, fmt.Errorf("unsupported protocol scheme in link: %s", rawLink)
	}
}

// IsBareAddress reports whether s is exactly a bare "ip:port" address — an
// IPv4 dotted quad or a bracketed IPv6 literal followed by a valid non-zero
// port — with no scheme, userinfo, path or remark. A subscription entry in
// this shape carries no protocol information, so it is treated as an
// unauthenticated SOCKS5 proxy.
func IsBareAddress(s string) bool {
	host, portText, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return false
	}
	return net.ParseIP(host) != nil
}

// dedupKey returns the identity key used to detect duplicate links. A bare
// "ip:port" address and a "socks5://" or "socks://" link without userinfo
// parse to the same unauthenticated SOCKS5 proxy, so they share one key;
// every other link is keyed by its remark-stripped form. Links carrying
// userinfo keep their raw key, because credentials make them a different
// proxy than the bare address of the same host:port.
func dedupKey(rawLink string) string {
	stripped := StripRemark(rawLink)
	if IsBareAddress(stripped) {
		return "socks5://" + stripped
	}

	body, ok := stripSchemePrefix(stripped, "socks5://", "socks://")
	if !ok || body == "" {
		return stripped
	}
	if queryIndex := strings.Index(body, "?"); queryIndex != -1 {
		body = body[:queryIndex]
	}
	if strings.Contains(body, "@") {
		return stripped
	}
	return "socks5://" + body
}

// DeduplicateConfigs parses raw links, removes duplicates based on their normalized identity —
// so a bare "ip:port" address and its equivalent SOCKS scheme link count as one —
// and returns normalized ProxyConfig items with 1-based sequential indices.
func DeduplicateConfigs(rawLinks []string) []types.ProxyConfig {
	configs, _ := DeduplicateAppend(rawLinks, make(map[string]bool), 1)
	return configs
}

// DeduplicateAppend parses one batch of raw links, skipping duplicates already
// present in seen (shared across batches), and assigns sequential indices
// starting at startIndex. It returns the batch configs and the next free
// index, so callers can process arbitrarily large inputs one batch at a time
// while keeping only a single batch of ProxyConfig values in RAM.
func DeduplicateAppend(rawLinks []string, seen map[string]bool, startIndex int) ([]types.ProxyConfig, int) {
	var configs []types.ProxyConfig
	currentIndex := startIndex

	for _, raw := range rawLinks {
		key := dedupKey(raw)
		if seen[key] {
			continue
		}

		cfg, err := ParseAndConvert(raw, currentIndex)
		if err != nil {
			continue
		}

		seen[key] = true
		configs = append(configs, *cfg)
		currentIndex++
	}

	return configs, currentIndex
}
