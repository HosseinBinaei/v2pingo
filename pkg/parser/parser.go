package parser

import (
	"fmt"
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

// ParseAndConvert parses a raw proxy share link and converts it to a sing-box option.Outbound ProxyConfig.
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
	default:
		return nil, fmt.Errorf("unsupported protocol scheme in link: %s", rawLink)
	}
}

// DeduplicateConfigs parses raw links, removes duplicates based on their stripped URI,
// and returns normalized ProxyConfig items with 1-based sequential indices.
func DeduplicateConfigs(rawLinks []string) []types.ProxyConfig {
	seen := make(map[string]bool)
	var configs []types.ProxyConfig
	currentIndex := 1

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

	return configs
}
