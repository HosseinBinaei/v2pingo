package parser

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"v2pingo/pkg/types"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// defaultWireGuardAllowedIPs returns the peer routing table used when a link
// does not carry allowed IPs. sing-box refuses to start a WireGuard endpoint
// without at least one allowed prefix.
func defaultWireGuardAllowedIPs() badoption.Listable[netip.Prefix] {
	return badoption.Listable[netip.Prefix]{
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("::/0"),
	}
}

// parseWireGuard converts a wireguard:// (or wg://) share link into a sing-box
// WireGuard endpoint. sing-box removed the legacy WireGuard outbound in 1.13,
// so WireGuard nodes are tested through an endpoint instead of an outbound.
//
// Supported link format (v2rayN compatible):
//
//	wireguard://<privateKey>@<host>:<port>?publickey=<key>&address=<ip/prefix>&mtu=<n>&reserved=<a,b,c>#remark
func parseWireGuard(rawLink string, index int) (*types.ProxyConfig, error) {
	stripped := StripRemark(rawLink)
	body, ok := stripSchemePrefix(stripped, "wireguard://", "wg://")
	if !ok {
		return nil, fmt.Errorf("unsupported WireGuard link scheme: %s", stripped)
	}
	if body == "" {
		return nil, fmt.Errorf("empty WireGuard link body")
	}

	queryPart := ""
	if queryIndex := strings.Index(body, "?"); queryIndex != -1 {
		queryPart = body[queryIndex+1:]
		body = body[:queryIndex]
	}

	atIndex := strings.LastIndex(body, "@")
	if atIndex <= 0 {
		return nil, fmt.Errorf("missing private key in WireGuard link")
	}

	// The private key is base64 and may contain '/' and '+', so the authority is
	// split manually instead of relying on net/url.
	privateKey := unescapeUserInfo(body[:atIndex])
	if privateKey == "" {
		return nil, fmt.Errorf("empty private key in WireGuard link")
	}

	host, portText, err := net.SplitHostPort(body[atIndex+1:])
	if err != nil {
		return nil, fmt.Errorf("invalid host:port in WireGuard link: %w", err)
	}
	port, err := parsePortNumber(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid port in WireGuard link: %w", err)
	}

	query, err := url.ParseQuery(queryPart)
	if err != nil {
		return nil, fmt.Errorf("invalid query in WireGuard link: %w", err)
	}

	publicKey := query.Get("publickey")
	if publicKey == "" {
		return nil, fmt.Errorf("missing publickey in WireGuard link")
	}

	addresses := parseWireGuardAddresses(query.Get("address"))
	if len(addresses) == 0 {
		return nil, fmt.Errorf("missing interface address in WireGuard link")
	}

	mtu := 0
	if mtuText := query.Get("mtu"); mtuText != "" {
		if parsedMTU, err := strconv.Atoi(strings.TrimSpace(mtuText)); err == nil && parsedMTU > 0 {
			mtu = parsedMTU
		}
	}

	wireGuardOptions := option.WireGuardEndpointOptions{
		MTU:        uint32(mtu),
		Address:    addresses,
		PrivateKey: privateKey,
		Peers: []option.WireGuardPeer{
			{
				Address:      host,
				Port:         port,
				PublicKey:    publicKey,
				PreSharedKey: query.Get("presharedkey"),
				AllowedIPs:   defaultWireGuardAllowedIPs(),
				Reserved:     parseWireGuardReserved(query.Get("reserved")),
			},
		},
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "wireguard",
		Host:     host,
		Port:     port,
		RawLink:  rawLink,
		Endpoint: &option.Endpoint{
			Type:    "wireguard",
			Tag:     "proxy",
			Options: &wireGuardOptions,
		},
	}, nil
}

// parseWireGuardAddresses parses the comma separated interface address list of a
// WireGuard link, accepting both the "10.0.0.2/32" and the bare "10.0.0.2" form.
func parseWireGuardAddresses(value string) badoption.Listable[netip.Prefix] {
	var prefixes badoption.Listable[netip.Prefix]
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(part); err == nil {
			prefixes = append(prefixes, prefix)
			continue
		}
		if address, err := netip.ParseAddr(part); err == nil {
			prefixes = append(prefixes, netip.PrefixFrom(address, address.BitLen()))
		}
	}
	return prefixes
}

// parseWireGuardReserved parses the optional Cloudflare WARP "reserved" value,
// which sing-box only accepts as exactly three bytes.
func parseWireGuardReserved(value string) []uint8 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '/'
	})
	if len(fields) == 3 {
		reserved := make([]uint8, 0, 3)
		for _, field := range fields {
			number, err := strconv.ParseUint(field, 10, 8)
			if err != nil {
				return nil
			}
			reserved = append(reserved, uint8(number))
		}
		return reserved
	}

	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil && len(decoded) == 3 {
			return []uint8{decoded[0], decoded[1], decoded[2]}
		}
	}
	return nil
}
