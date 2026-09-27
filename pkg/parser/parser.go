package parser

import (
	"fmt"
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

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESS(trimmed, index)
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMess(trimmed, index)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojan(trimmed, index)
	case strings.HasPrefix(lower, "ss://"):
		return parseShadowsocks(trimmed, index)
	case strings.HasPrefix(lower, "socks://"), strings.HasPrefix(lower, "socks5://"):
		return parseSOCKS(trimmed, index)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2(trimmed, index)
	case strings.HasPrefix(lower, "wireguard://"), strings.HasPrefix(lower, "wg://"):
		return parseWireGuard(trimmed, index)
	default:
		return nil, fmt.Errorf("unsupported protocol scheme in link: %s", rawLink)
	}
}

// DeduplicateConfigs parses raw links, removes duplicates based on their stripped URI,
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
		stripped := StripRemark(raw)
		if seen[stripped] {
			continue
		}

		cfg, err := ParseAndConvert(raw, currentIndex)
		if err != nil {
			continue
		}

		seen[stripped] = true
		configs = append(configs, *cfg)
		currentIndex++
	}

	return configs, currentIndex
}
