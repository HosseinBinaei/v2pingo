package parser

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"v2pingo/pkg/types"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

type vmessJSON struct {
	V    any    `json:"v"`
	Ps   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"`
	Id   string `json:"id"`
	Aid  any    `json:"aid"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return b, nil
	}
	return nil, errors.New("invalid base64 encoding")
}

func parsePortVal(val any) (uint16, error) {
	switch v := val.(type) {
	case float64:
		return uint16(v), nil
	case string:
		p, err := strconv.ParseUint(v, 10, 16)
		if err != nil {
			return 0, err
		}
		return uint16(p), nil
	case int:
		return uint16(v), nil
	default:
		return 0, fmt.Errorf("invalid port format: %v", val)
	}
}

func parseAidVal(val any) int {
	switch v := val.(type) {
	case float64:
		return int(v)
	case string:
		a, _ := strconv.Atoi(v)
		return a
	case int:
		return v
	default:
		return 0
	}
}

func parseVMess(rawLink string, index int) (*types.ProxyConfig, error) {
	stripped := StripRemark(rawLink)
	b64Data := strings.TrimPrefix(stripped, "vmess://")
	b64Data = strings.TrimPrefix(b64Data, "VMESS://")

	jsonBytes, err := decodeBase64(b64Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode VMess base64: %w", err)
	}

	var vObj vmessJSON
	if err := json.Unmarshal(jsonBytes, &vObj); err != nil {
		return nil, fmt.Errorf("failed to parse VMess JSON: %w", err)
	}

	if vObj.Add == "" {
		return nil, fmt.Errorf("missing address in VMess JSON")
	}
	if vObj.Id == "" {
		return nil, fmt.Errorf("missing id/UUID in VMess JSON")
	}

	port, err := parsePortVal(vObj.Port)
	if err != nil || port == 0 {
		return nil, fmt.Errorf("invalid or missing port in VMess JSON: %v", vObj.Port)
	}

	aid := parseAidVal(vObj.Aid)

	var tlsOpt *option.OutboundTLSOptions
	tlsVal := strings.ToLower(strings.TrimSpace(vObj.TLS))
	if tlsVal == "tls" || tlsVal == "1" {
		sni := vObj.SNI
		if sni == "" {
			sni = vObj.Host
		}
		tlsOpt = &option.OutboundTLSOptions{
			Enabled:    true,
			ServerName: sni,
		}
	}

	var transportOpt *option.V2RayTransportOptions
	netType := strings.ToLower(strings.TrimSpace(vObj.Net))
	switch netType {
	case "ws":
		wsOpt := option.V2RayWebsocketOptions{
			Path: vObj.Path,
		}
		if vObj.Host != "" {
			wsOpt.Headers = badoption.HTTPHeader{
				"Host": badoption.Listable[string]{vObj.Host},
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
				ServiceName: vObj.Path,
			},
		}
	case "h2", "http":
		httpOpt := option.V2RayHTTPOptions{
			Path: vObj.Path,
		}
		if vObj.Host != "" {
			httpOpt.Host = badoption.Listable[string]{vObj.Host}
		}
		transportOpt = &option.V2RayTransportOptions{
			Type:        "http",
			HTTPOptions: httpOpt,
		}
	}

	vmessOpt := option.VMessOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     vObj.Add,
			ServerPort: port,
		},
		UUID:      vObj.Id,
		Security:  "auto",
		AlterId:   aid,
		Transport: transportOpt,
	}
	if tlsOpt != nil {
		vmessOpt.TLS = tlsOpt
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "vmess",
		Host:     vObj.Add,
		Port:     port,
		RawLink:  rawLink,
		Options: option.Outbound{
			Type:    "vmess",
			Tag:     "proxy",
			Options: vmessOpt,
		},
	}, nil
}
