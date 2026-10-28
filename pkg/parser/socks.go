package parser

import (
	"fmt"
	"net"
	"strings"

	"v2pingo/pkg/types"

	"github.com/sagernet/sing-box/option"
)

// parseSOCKS converts a socks:// or socks5:// share link into a sing-box SOCKS5
// outbound. The plain "user:password@host:port" userinfo, the base64 encoded
// userinfo used by v2rayN and the legacy fully base64 encoded URI are supported.
func parseSOCKS(rawLink string, index int) (*types.ProxyConfig, error) {
	stripped := StripRemark(rawLink)
	body, ok := stripSchemePrefix(stripped, "socks5://", "socks://")
	if !ok {
		return nil, fmt.Errorf("unsupported SOCKS link scheme: %s", stripped)
	}
	if body == "" {
		return nil, fmt.Errorf("empty SOCKS link body")
	}
	if queryIndex := strings.Index(body, "?"); queryIndex != -1 {
		body = body[:queryIndex]
	}

	var username, password string
	atIndex := strings.LastIndex(body, "@")
	if atIndex == -1 {
		// Legacy form: the whole body is base64 encoded "user:password@host:port".
		if decoded, err := decodeBase64(body); err == nil && strings.Contains(string(decoded), "@") {
			body = string(decoded)
			atIndex = strings.LastIndex(body, "@")
		}
	}
	if atIndex != -1 {
		userInfo := unescapeUserInfo(body[:atIndex])
		hostPort := body[atIndex+1:]
		if strings.Contains(userInfo, ":") {
			username, password = splitUserInfo(userInfo)
		} else if decoded, err := decodeBase64(userInfo); err == nil && strings.Contains(string(decoded), ":") {
			username, password = splitUserInfo(string(decoded))
		} else {
			username = userInfo
		}
		body = hostPort
	}

	host, portText, err := net.SplitHostPort(body)
	if err != nil {
		return nil, fmt.Errorf("invalid host:port in SOCKS link: %w", err)
	}
	port, err := parsePortNumber(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid port in SOCKS link: %w", err)
	}

	return newSOCKSConfig(index, rawLink, host, port, username, password), nil
}

// parseBareSOCKS converts a bare "ip:port" address — a subscription entry
// that carries no protocol information — into an unauthenticated SOCKS5
// proxy. IsBareAddress defines the accepted grammar.
func parseBareSOCKS(rawLink string, index int) (*types.ProxyConfig, error) {
	host, portText, err := net.SplitHostPort(rawLink)
	if err != nil {
		return nil, fmt.Errorf("invalid bare address %q: %w", rawLink, err)
	}
	port, err := parsePortNumber(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid port in bare address: %w", err)
	}
	return newSOCKSConfig(index, rawLink, host, port, "", ""), nil
}

// newSOCKSConfig builds the sing-box SOCKS5 outbound shared by every SOCKS
// variant: scheme-prefixed links and bare addresses alike.
func newSOCKSConfig(index int, rawLink, host string, port uint16, username, password string) *types.ProxyConfig {
	socksOptions := option.SOCKSOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     host,
			ServerPort: port,
		},
		Version:  "5",
		Username: username,
		Password: password,
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "socks",
		Host:     host,
		Port:     port,
		RawLink:  rawLink,
		Options: option.Outbound{
			Type:    "socks",
			Tag:     "proxy",
			Options: &socksOptions,
		},
	}
}
