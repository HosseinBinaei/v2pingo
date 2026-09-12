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

func parseVLESS(rawLink string, index int) (*types.ProxyConfig, error) {
	u, err := url.Parse(rawLink)
	if err != nil {
		return nil, fmt.Errorf("invalid VLESS URL: %w", err)
	}

	if u.User == nil {
		return nil, fmt.Errorf("missing userinfo/UUID in VLESS URL")
	}
	uuid := u.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("empty UUID in VLESS URL")
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("missing host in VLESS URL")
	}

	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("missing port in VLESS URL")
	}
	portNum, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q in VLESS URL: %w", portStr, err)
	}
	port := uint16(portNum)

	q := u.Query()
	security := strings.ToLower(q.Get("security"))
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	flow := q.Get("flow")
	typeParam := strings.ToLower(q.Get("type"))
	path := q.Get("path")
	serviceName := q.Get("serviceName")
	if serviceName == "" {
		serviceName = q.Get("service_name")
	}
	wsHost := q.Get("host")
	pbk := q.Get("pbk")
	sid := q.Get("sid")
	fp := q.Get("fp")
	insecure := q.Get("insecure") == "1" || q.Get("insecure") == "true" || q.Get("allowInsecure") == "1" || q.Get("allowInsecure") == "true"

	var tlsOpt *option.OutboundTLSOptions
	if security == "tls" || security == "reality" {
		tlsOpt = &option.OutboundTLSOptions{
			Enabled:    true,
			ServerName: sni,
			Insecure:   insecure,
		}
		if security == "reality" {
			tlsOpt.Reality = &option.OutboundRealityOptions{
				Enabled:   true,
				PublicKey: pbk,
				ShortID:   sid,
			}
		}
		if fp != "" {
			tlsOpt.UTLS = &option.OutboundUTLSOptions{
				Enabled:     true,
				Fingerprint: fp,
			}
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

	vlessOpt := option.VLESSOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     host,
			ServerPort: port,
		},
		UUID:      uuid,
		Flow:      flow,
		Transport: transportOpt,
	}
	if tlsOpt != nil {
		vlessOpt.TLS = tlsOpt
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "vless",
		Host:     host,
		Port:     port,
		RawLink:  rawLink,
		Options: option.Outbound{
			Type:    "vless",
			Tag:     "proxy",
			Options: &vlessOpt,
		},
	}, nil
}
