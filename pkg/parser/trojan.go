package parser

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"v2pingo/pkg/types"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

func parseTrojan(rawLink string, index int) (*types.ProxyConfig, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("invalid Trojan URL: %w", err)
	}

	if u.User == nil {
		return nil, fmt.Errorf("missing password in Trojan URL")
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("empty password in Trojan URL")
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("missing host in Trojan URL")
	}

	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("missing port in Trojan URL")
	}
	portNum, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q in Trojan URL: %w", portStr, err)
	}
	port := uint16(portNum)

	q := u.Query()
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	if sni == "" {
		sni = host
	}

	typeParam := strings.ToLower(q.Get("type"))
	path := q.Get("path")
	serviceName := q.Get("serviceName")
	if serviceName == "" {
		serviceName = q.Get("service_name")
	}
	wsHost := q.Get("host")
	fp := q.Get("fp")
	insecure := q.Get("insecure") == "1" || q.Get("insecure") == "true" || q.Get("allowInsecure") == "1" || q.Get("allowInsecure") == "true"

	tlsOpt := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: sni,
		Insecure:   insecure,
	}
	if fp != "" {
		tlsOpt.UTLS = &option.OutboundUTLSOptions{
			Enabled:     true,
			Fingerprint: fp,
		}
	}

	var transportOpt *option.V2RayTransportOptions
	switch typeParam {
	case "ws":
		wsOpt := option.V2RayWebsocketOptions{
			Path: path,
		}
		if wsHost != "" {
			wsOpt.Headers = badoption.HTTPHeader{
				"Host": badoption.Listable[string]{wsHost},
			}
		}
		transportOpt = &option.V2RayTransportOptions{
			Type:             "ws",
			WebsocketOptions: wsOpt,
		}
	case "grpc":
		transportOpt = &option.V2RayTransportOptions{
			Type: "grpc",
			GRPCOptions: option.V2RayGRPCOptions{
				ServiceName: serviceName,
			},
		}
	case "http", "h2":
		httpOpt := option.V2RayHTTPOptions{
			Path: path,
		}
		if wsHost != "" {
			httpOpt.Host = badoption.Listable[string]{wsHost}
		}
		transportOpt = &option.V2RayTransportOptions{
			Type:        "http",
			HTTPOptions: httpOpt,
		}
	}

	trojanOpt := option.TrojanOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     host,
			ServerPort: port,
		},
		Password:  password,
		Transport: transportOpt,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
			TLS: tlsOpt,
		},
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "trojan",
		Host:     host,
		Port:     port,
		RawLink:  rawLink,
		Options: option.Outbound{
			Type:    "trojan",
			Tag:     "proxy",
			Options: &trojanOpt,
		},
	}, nil
}
