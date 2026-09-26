package parser

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"v2pingo/pkg/types"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// parseHysteria2 converts a hysteria2:// (or hy2://) share link into a sing-box
// Hysteria2 outbound following the official URI scheme:
//
//	hysteria2://[auth@]hostname[:port][,port...]/?[key=value]&...#name
func parseHysteria2(rawLink string, index int) (*types.ProxyConfig, error) {
	stripped := StripRemark(rawLink)
	body, ok := stripSchemePrefix(stripped, "hysteria2://", "hy2://")
	if !ok {
		return nil, fmt.Errorf("unsupported Hysteria2 link scheme: %s", stripped)
	}
	if body == "" {
		return nil, fmt.Errorf("empty Hysteria2 link body")
	}

	queryPart := ""
	if queryIndex := strings.Index(body, "?"); queryIndex != -1 {
		queryPart = body[queryIndex+1:]
		body = body[:queryIndex]
	}

	// The path component of the hysteria2:// scheme is always empty, but an
	// unencoded '/' may appear inside the auth component, so the search for the
	// path separator starts after the userinfo.
	pathSearchStart := 0
	if atIndex := strings.LastIndex(body, "@"); atIndex != -1 {
		pathSearchStart = atIndex + 1
	}
	if slashIndex := strings.Index(body[pathSearchStart:], "/"); slashIndex != -1 {
		body = body[:pathSearchStart+slashIndex]
	}

	password := ""
	hostPort := body
	if atIndex := strings.LastIndex(body, "@"); atIndex != -1 {
		password = unescapeUserInfo(body[:atIndex])
		hostPort = body[atIndex+1:]
	}
	if password == "" {
		return nil, fmt.Errorf("missing auth password in Hysteria2 link")
	}

	host, portSpec, err := splitHysteria2HostPort(hostPort)
	if err != nil {
		return nil, err
	}
	port, serverPorts, err := splitHysteria2PortSpec(portSpec)
	if err != nil {
		return nil, err
	}

	query, err := url.ParseQuery(queryPart)
	if err != nil {
		return nil, fmt.Errorf("invalid query in Hysteria2 link: %w", err)
	}

	// v2rayN stores port hopping ranges as "443-8443" inside the mport parameter.
	if hopPorts := query.Get("mport"); hopPorts != "" {
		ports, err := parseHysteria2Ports(strings.ReplaceAll(hopPorts, "-", ":"))
		if err != nil {
			return nil, fmt.Errorf("invalid mport in Hysteria2 link: %w", err)
		}
		serverPorts = append(serverPorts, ports...)
	}

	tlsOptions := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: query.Get("sni"),
		Insecure:   parseBoolFlag(query.Get("insecure")),
	}
	if pin := query.Get("pinSHA256"); pin != "" {
		if digest, ok := decodeSHA256(pin); ok {
			tlsOptions.CertificatePublicKeySHA256 = badoption.Listable[[]byte]{digest}
		} else {
			// Some providers ship an unreadable certificate fingerprint, which can
			// not be verified against a public CA.
			tlsOptions.Insecure = true
		}
	}

	obfs, err := parseHysteria2Obfs(query.Get("obfs"), query.Get("obfs-password"))
	if err != nil {
		return nil, err
	}

	hysteria2Options := option.Hysteria2OutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     host,
			ServerPort: port,
		},
		Password:                    password,
		ServerPorts:                 serverPorts,
		Obfs:                        obfs,
		UpMbps:                      parseNonNegativeInt(query.Get("up")),
		DownMbps:                    parseNonNegativeInt(query.Get("down")),
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tlsOptions},
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "hysteria2",
		Host:     host,
		Port:     port,
		RawLink:  rawLink,
		Options: option.Outbound{
			Type:    "hysteria2",
			Tag:     "proxy",
			Options: &hysteria2Options,
		},
	}, nil
}

// splitHysteria2HostPort splits the authority of a hysteria2:// link into host
// and a port specification that may contain a comma separated hopping list.
func splitHysteria2HostPort(hostPort string) (string, string, error) {
	if hostPort == "" {
		return "", "", fmt.Errorf("missing host in Hysteria2 link")
	}
	if strings.HasPrefix(hostPort, "[") && strings.HasSuffix(hostPort, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(hostPort, "["), "]"), "", nil
	}

	host, portSpec, err := net.SplitHostPort(hostPort)
	if err != nil {
		if strings.Contains(hostPort, ":") {
			return "", "", fmt.Errorf("invalid host:port in Hysteria2 link: %w", err)
		}
		return hostPort, "", nil
	}
	return host, portSpec, nil
}

// splitHysteria2PortSpec converts the authority port of a hysteria2:// link into
// a single port plus the optional port hopping list (sing-box "server_ports").
func splitHysteria2PortSpec(portSpec string) (uint16, badoption.Listable[string], error) {
	portSpec = strings.TrimSpace(portSpec)
	if portSpec == "" {
		// Hysteria 2 defaults to port 443 when the URI omits the port.
		return 443, nil, nil
	}

	if !strings.ContainsAny(portSpec, ",:-") {
		port, err := parsePortNumber(portSpec)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid port in Hysteria2 link: %w", err)
		}
		return port, nil, nil
	}

	ports, err := parseHysteria2Ports(portSpec)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid port list in Hysteria2 link: %w", err)
	}

	startText, _, _ := strings.Cut(ports[0], ":")
	port, err := parsePortNumber(startText)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid port in Hysteria2 link: %w", err)
	}
	return port, ports, nil
}

// parseHysteria2Ports normalizes a comma separated port list into sing-box
// "server_ports" entries, which always have to use the "start:end" form.
func parseHysteria2Ports(portSpec string) (badoption.Listable[string], error) {
	var ports badoption.Listable[string]
	for _, part := range strings.Split(portSpec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		startText, endText, hasRange := strings.Cut(strings.ReplaceAll(part, "-", ":"), ":")
		start, err := parsePortNumber(startText)
		if err != nil {
			return nil, err
		}
		end := start
		if hasRange {
			end, err = parsePortNumber(endText)
			if err != nil {
				return nil, err
			}
		}
		if start > end {
			return nil, fmt.Errorf("invalid port range %q", part)
		}
		ports = append(ports, fmt.Sprintf("%d:%d", start, end))
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("empty port list %q", portSpec)
	}
	return ports, nil
}

// parseHysteria2Obfs builds the sing-box obfuscation options from the URI.
func parseHysteria2Obfs(obfsType string, obfsPassword string) (*option.Hysteria2Obfs, error) {
	obfsType = strings.ToLower(strings.TrimSpace(obfsType))
	if obfsType == "" {
		return nil, nil
	}
	switch obfsType {
	case "salamander", "gecko":
	default:
		return nil, fmt.Errorf("unsupported Hysteria2 obfs type: %s", obfsType)
	}
	if obfsPassword == "" {
		return nil, fmt.Errorf("missing obfs password in Hysteria2 link")
	}
	return &option.Hysteria2Obfs{
		Type:     obfsType,
		Password: obfsPassword,
	}, nil
}

// decodeSHA256 decodes a hex or base64 encoded SHA-256 digest.
func decodeSHA256(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	if digest, err := hex.DecodeString(value); err == nil && len(digest) == 32 {
		return digest, true
	}
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if digest, err := encoding.DecodeString(value); err == nil && len(digest) == 32 {
			return digest, true
		}
	}
	return nil, false
}

// parseBoolFlag parses the "1"/"0"/"true"/"false" style boolean query values.
func parseBoolFlag(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// parseNonNegativeInt parses an optional non-negative integer query value.
func parseNonNegativeInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
