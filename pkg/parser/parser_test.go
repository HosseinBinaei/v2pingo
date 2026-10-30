package parser_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"v2pingo/pkg/parser"

	"github.com/sagernet/sing-box/option"
)

func TestParseVLESSReality(t *testing.T) {
	raw := "vless://12345678-1234-1234-1234-123456789012@example.com:443?security=reality&sni=sni.example.com&fp=chrome&pbk=publickey123&sid=sid123&flow=xtls-rprx-vision&type=grpc&serviceName=mygrpcservice#VLESS-Reality"

	cfg, err := parser.ParseAndConvert(raw, 1)
	if err != nil {
		t.Fatalf("ParseAndConvert returned error: %v", err)
	}

	if cfg.Index != 1 {
		t.Errorf("expected Index 1, got %d", cfg.Index)
	}
	if cfg.Protocol != "vless" {
		t.Errorf("expected Protocol vless, got %s", cfg.Protocol)
	}
	if cfg.Host != "example.com" {
		t.Errorf("expected Host example.com, got %s", cfg.Host)
	}
	if cfg.Port != 443 {
		t.Errorf("expected Port 443, got %d", cfg.Port)
	}
	if cfg.RawLink != raw {
		t.Errorf("expected RawLink %s, got %s", raw, cfg.RawLink)
	}

	opt := cfg.Options
	if opt.Type != "vless" || opt.Tag != "proxy" {
		t.Errorf("expected Type vless, Tag proxy; got Type %s, Tag %s", opt.Type, opt.Tag)
	}

	vlessOpt, ok := opt.Options.(*option.VLESSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.VLESSOutboundOptions, got %T", opt.Options)
	}

	if vlessOpt.Server != "example.com" || vlessOpt.ServerPort != 443 {
		t.Errorf("server mismatch: %s:%d", vlessOpt.Server, vlessOpt.ServerPort)
	}
	if vlessOpt.UUID != "12345678-1234-1234-1234-123456789012" {
		t.Errorf("UUID mismatch: %s", vlessOpt.UUID)
	}
	if vlessOpt.Flow != "xtls-rprx-vision" {
		t.Errorf("Flow mismatch: %s", vlessOpt.Flow)
	}

	if vlessOpt.TLS == nil || !vlessOpt.TLS.Enabled {
		t.Fatal("expected TLS options to be enabled")
	}
	if vlessOpt.TLS.ServerName != "sni.example.com" {
		t.Errorf("TLS ServerName mismatch: %s", vlessOpt.TLS.ServerName)
	}
	if vlessOpt.TLS.Reality == nil || !vlessOpt.TLS.Reality.Enabled {
		t.Fatal("expected Reality options to be enabled")
	}
	if vlessOpt.TLS.Reality.PublicKey != "publickey123" || vlessOpt.TLS.Reality.ShortID != "sid123" {
		t.Errorf("Reality details mismatch: %+v", vlessOpt.TLS.Reality)
	}
	if vlessOpt.TLS.UTLS == nil || vlessOpt.TLS.UTLS.Fingerprint != "chrome" {
		t.Errorf("uTLS fingerprint mismatch: %+v", vlessOpt.TLS.UTLS)
	}

	if vlessOpt.Transport == nil || vlessOpt.Transport.Type != "grpc" {
		t.Fatalf("expected gRPC transport, got %+v", vlessOpt.Transport)
	}
	if vlessOpt.Transport.GRPCOptions.ServiceName != "mygrpcservice" {
		t.Errorf("gRPC ServiceName mismatch: %s", vlessOpt.Transport.GRPCOptions.ServiceName)
	}
}

func TestParseVLESSWS(t *testing.T) {
	raw := "vless://12345678-1234-1234-1234-123456789012@example.com:8443?security=tls&sni=sni.example.com&type=ws&path=/vless-ws&host=ws.example.com#VLESS-WS"

	cfg, err := parser.ParseAndConvert(raw, 2)
	if err != nil {
		t.Fatalf("ParseAndConvert returned error: %v", err)
	}

	vlessOpt, ok := cfg.Options.Options.(*option.VLESSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.VLESSOutboundOptions, got %T", cfg.Options.Options)
	}

	if vlessOpt.Transport == nil || vlessOpt.Transport.Type != "ws" {
		t.Fatalf("expected WS transport, got %+v", vlessOpt.Transport)
	}
	if vlessOpt.Transport.WebsocketOptions.Path != "/vless-ws" {
		t.Errorf("WS Path mismatch: %s", vlessOpt.Transport.WebsocketOptions.Path)
	}
	headers := vlessOpt.Transport.WebsocketOptions.Headers
	if len(headers["Host"]) == 0 || headers["Host"][0] != "ws.example.com" {
		t.Errorf("WS Host header mismatch: %+v", headers)
	}
}

func TestParseVMess(t *testing.T) {
	vmessMap := map[string]any{
		"v":    "2",
		"ps":   "vmess-test",
		"add":  "1.2.3.4",
		"port": 443,
		"id":   "12345678-1234-1234-1234-123456789012",
		"aid":  0,
		"net":  "ws",
		"type": "none",
		"host": "ws.example.com",
		"path": "/vmess",
		"tls":  "tls",
		"sni":  "sni.example.com",
	}
	data, _ := json.Marshal(vmessMap)
	raw := "vmess://" + base64.StdEncoding.EncodeToString(data)

	cfg, err := parser.ParseAndConvert(raw, 3)
	if err != nil {
		t.Fatalf("ParseAndConvert VMess error: %v", err)
	}

	if cfg.Protocol != "vmess" {
		t.Errorf("expected Protocol vmess, got %s", cfg.Protocol)
	}
	if cfg.Host != "1.2.3.4" || cfg.Port != 443 {
		t.Errorf("host/port mismatch: %s:%d", cfg.Host, cfg.Port)
	}

	vmessOpt, ok := cfg.Options.Options.(*option.VMessOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.VMessOutboundOptions, got %T", cfg.Options.Options)
	}
	if vmessOpt.UUID != "12345678-1234-1234-1234-123456789012" {
		t.Errorf("UUID mismatch: %s", vmessOpt.UUID)
	}
	if vmessOpt.TLS == nil || vmessOpt.TLS.ServerName != "sni.example.com" {
		t.Errorf("TLS SNI mismatch: %+v", vmessOpt.TLS)
	}
	if vmessOpt.Transport == nil || vmessOpt.Transport.Type != "ws" {
		t.Fatalf("WS transport mismatch: %+v", vmessOpt.Transport)
	}
}

func TestParseTrojan(t *testing.T) {
	raw := "trojan://mysecretpassword@trojan.example.com:443?security=tls&sni=trojan.example.com&type=ws&path=/trojan-ws#Trojan-WS"

	cfg, err := parser.ParseAndConvert(raw, 4)
	if err != nil {
		t.Fatalf("ParseAndConvert Trojan error: %v", err)
	}

	if cfg.Protocol != "trojan" {
		t.Errorf("expected Protocol trojan, got %s", cfg.Protocol)
	}
	trojanOpt, ok := cfg.Options.Options.(*option.TrojanOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.TrojanOutboundOptions, got %T", cfg.Options.Options)
	}
	if trojanOpt.Password != "mysecretpassword" {
		t.Errorf("Password mismatch: %s", trojanOpt.Password)
	}
	if trojanOpt.TLS == nil || trojanOpt.TLS.ServerName != "trojan.example.com" {
		t.Errorf("TLS mismatch: %+v", trojanOpt.TLS)
	}
	if trojanOpt.Transport == nil || trojanOpt.Transport.Type != "ws" {
		t.Fatalf("Transport mismatch: %+v", trojanOpt.Transport)
	}
}

func TestParseShadowsocksSIP002(t *testing.T) {
	userInfo := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:password123"))
	raw := fmt.Sprintf("ss://%s@ss.example.com:8388#SS-SIP002", userInfo)

	cfg, err := parser.ParseAndConvert(raw, 5)
	if err != nil {
		t.Fatalf("ParseAndConvert SS SIP002 error: %v", err)
	}

	if cfg.Protocol != "shadowsocks" {
		t.Errorf("expected Protocol shadowsocks, got %s", cfg.Protocol)
	}
	ssOpt, ok := cfg.Options.Options.(*option.ShadowsocksOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.ShadowsocksOutboundOptions, got %T", cfg.Options.Options)
	}
	if ssOpt.Method != "aes-256-gcm" || ssOpt.Password != "password123" {
		t.Errorf("Method/Password mismatch: %s / %s", ssOpt.Method, ssOpt.Password)
	}
}

func TestParseShadowsocksLegacy(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:password123@ss.example.com:8388"))
	raw := fmt.Sprintf("ss://%s#SS-Legacy", b64)

	cfg, err := parser.ParseAndConvert(raw, 6)
	if err != nil {
		t.Fatalf("ParseAndConvert SS Legacy error: %v", err)
	}

	if cfg.Protocol != "shadowsocks" {
		t.Errorf("expected Protocol shadowsocks, got %s", cfg.Protocol)
	}
	ssOpt, ok := cfg.Options.Options.(*option.ShadowsocksOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.ShadowsocksOutboundOptions, got %T", cfg.Options.Options)
	}
	if ssOpt.Server != "ss.example.com" || ssOpt.ServerPort != 8388 {
		t.Errorf("Server/Port mismatch: %s:%d", ssOpt.Server, ssOpt.ServerPort)
	}
	if ssOpt.Method != "aes-256-gcm" || ssOpt.Password != "password123" {
		t.Errorf("Method/Password mismatch: %s / %s", ssOpt.Method, ssOpt.Password)
	}
}

func TestStripRemark(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"vless://uuid@host:443?foo=bar#remark", "vless://uuid@host:443?foo=bar"},
		{"vmess://ey...", "vmess://ey..."},
		{"ss://abc#", "ss://abc"},
	}

	for _, tt := range tests {
		got := parser.StripRemark(tt.input)
		if got != tt.expected {
			t.Errorf("StripRemark(%q) = %q; expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestDeduplicateConfigs(t *testing.T) {
	userInfo := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:password123"))
	link1 := fmt.Sprintf("ss://%s@ss.example.com:8388#Remark1", userInfo)
	link2 := fmt.Sprintf("ss://%s@ss.example.com:8388#Remark2", userInfo) // duplicate host/port/method/pass after strip remark
	link3 := "trojan://pass@trojan.example.com:443?security=tls#Trojan1"
	invalidLink := "invalid-link://foo"

	raws := []string{link1, link2, invalidLink, link3}

	configs := parser.DeduplicateConfigs(raws)

	if len(configs) != 2 {
		t.Fatalf("expected 2 unique configs, got %d", len(configs))
	}

	if configs[0].Index != 1 || configs[0].Protocol != "shadowsocks" {
		t.Errorf("config 0 mismatch: %+v", configs[0])
	}
	if configs[0].RawLink != link1 {
		t.Errorf("config 0 RawLink mismatch: %s", configs[0].RawLink)
	}

	if configs[1].Index != 2 || configs[1].Protocol != "trojan" {
		t.Errorf("config 1 mismatch: %+v", configs[1])
	}
	if configs[1].RawLink != link3 {
		t.Errorf("config 1 RawLink mismatch: %s", configs[1].RawLink)
	}
}

func TestParseErrors(t *testing.T) {
	invalidLinks := []string{
		"",
		"http://google.com",
		"vless://invalid-url",
		"vmess://invalid-base64-json!!!",
		"trojan://no-host",
		"ss://invalid-user-info",
	}

	for _, raw := range invalidLinks {
		_, err := parser.ParseAndConvert(raw, 1)
		if err == nil {
			t.Errorf("expected error for link %q, got nil", raw)
		}
	}
}

func TestParseWireGuard(t *testing.T) {
	privateKey := base64.StdEncoding.EncodeToString([]byte("private-key-private-key-private-1"))
	publicKey := base64.StdEncoding.EncodeToString([]byte("public-key-public-key-public-key1"))
	preSharedKey := base64.StdEncoding.EncodeToString([]byte("preshared-key-preshared-key-pres1"))

	raw := fmt.Sprintf(
		"wireguard://%s@wg.example.com:51820?publickey=%s&presharedkey=%s&address=10.0.0.2/32&mtu=1420&reserved=1,2,3#WG-Node",
		privateKey, publicKey, preSharedKey,
	)

	cfg, err := parser.ParseAndConvert(raw, 7)
	if err != nil {
		t.Fatalf("ParseAndConvert WireGuard error: %v", err)
	}

	if cfg.Protocol != "wireguard" {
		t.Errorf("expected Protocol wireguard, got %s", cfg.Protocol)
	}
	if cfg.Host != "wg.example.com" || cfg.Port != 51820 {
		t.Errorf("Host/Port mismatch: %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.RawLink != raw {
		t.Errorf("expected RawLink %s, got %s", raw, cfg.RawLink)
	}
	if cfg.Endpoint == nil {
		t.Fatal("expected a WireGuard endpoint, got nil")
	}
	if cfg.Endpoint.Type != "wireguard" || cfg.Endpoint.Tag != "proxy" {
		t.Errorf("endpoint type/tag mismatch: %s/%s", cfg.Endpoint.Type, cfg.Endpoint.Tag)
	}

	wgOpt, ok := cfg.Endpoint.Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("expected *option.WireGuardEndpointOptions, got %T", cfg.Endpoint.Options)
	}
	if wgOpt.PrivateKey != privateKey {
		t.Errorf("PrivateKey mismatch: %s", wgOpt.PrivateKey)
	}
	if wgOpt.MTU != 1420 {
		t.Errorf("MTU mismatch: %d", wgOpt.MTU)
	}
	if len(wgOpt.Address) != 1 || wgOpt.Address[0].String() != "10.0.0.2/32" {
		t.Errorf("Address mismatch: %v", wgOpt.Address)
	}
	if len(wgOpt.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(wgOpt.Peers))
	}

	peer := wgOpt.Peers[0]
	if peer.Address != "wg.example.com" || peer.Port != 51820 {
		t.Errorf("peer address mismatch: %s:%d", peer.Address, peer.Port)
	}
	if peer.PublicKey != publicKey {
		t.Errorf("peer PublicKey mismatch: %s", peer.PublicKey)
	}
	if peer.PreSharedKey != preSharedKey {
		t.Errorf("peer PreSharedKey mismatch: %s", peer.PreSharedKey)
	}
	if len(peer.AllowedIPs) == 0 {
		t.Error("expected default allowed IPs to be set")
	}
	if len(peer.Reserved) != 3 || peer.Reserved[0] != 1 || peer.Reserved[1] != 2 || peer.Reserved[2] != 3 {
		t.Errorf("Reserved mismatch: %v", peer.Reserved)
	}
}

func TestParseWireGuardBareAddress(t *testing.T) {
	privateKey := base64.StdEncoding.EncodeToString([]byte("private-key-private-key-private-2"))
	publicKey := base64.StdEncoding.EncodeToString([]byte("public-key-public-key-public-key2"))

	raw := fmt.Sprintf("wg://%s@1.2.3.4:2408?publickey=%s&address=172.16.0.2#WG-Alias", privateKey, publicKey)

	cfg, err := parser.ParseAndConvert(raw, 8)
	if err != nil {
		t.Fatalf("ParseAndConvert WireGuard alias error: %v", err)
	}
	if cfg.Endpoint == nil {
		t.Fatal("expected a WireGuard endpoint, got nil")
	}

	wgOpt, ok := cfg.Endpoint.Options.(*option.WireGuardEndpointOptions)
	if !ok {
		t.Fatalf("expected *option.WireGuardEndpointOptions, got %T", cfg.Endpoint.Options)
	}
	if len(wgOpt.Address) != 1 || wgOpt.Address[0].String() != "172.16.0.2/32" {
		t.Errorf("bare address should be normalized to /32, got %v", wgOpt.Address)
	}
	if wgOpt.Peers[0].Reserved != nil {
		t.Errorf("expected no reserved bytes, got %v", wgOpt.Peers[0].Reserved)
	}
}

func TestParseSOCKSBase64UserInfo(t *testing.T) {
	userInfo := base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	raw := fmt.Sprintf("socks://%s@1.2.3.4:1080#SOCKS-Node", userInfo)

	cfg, err := parser.ParseAndConvert(raw, 9)
	if err != nil {
		t.Fatalf("ParseAndConvert SOCKS error: %v", err)
	}

	if cfg.Protocol != "socks" {
		t.Errorf("expected Protocol socks, got %s", cfg.Protocol)
	}
	if cfg.Host != "1.2.3.4" || cfg.Port != 1080 {
		t.Errorf("Host/Port mismatch: %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.Options.Type != "socks" || cfg.Options.Tag != "proxy" {
		t.Errorf("expected Type socks, Tag proxy; got Type %s, Tag %s", cfg.Options.Type, cfg.Options.Tag)
	}

	socksOpt, ok := cfg.Options.Options.(*option.SOCKSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.SOCKSOutboundOptions, got %T", cfg.Options.Options)
	}
	if socksOpt.Server != "1.2.3.4" || socksOpt.ServerPort != 1080 {
		t.Errorf("Server/Port mismatch: %s:%d", socksOpt.Server, socksOpt.ServerPort)
	}
	if socksOpt.Version != "5" {
		t.Errorf("Version mismatch: %s", socksOpt.Version)
	}
	if socksOpt.Username != "user1" || socksOpt.Password != "pass1" {
		t.Errorf("credentials mismatch: %s / %s", socksOpt.Username, socksOpt.Password)
	}
}

func TestParseSOCKSPlainUserInfo(t *testing.T) {
	raw := "socks5://user2:pass2@5.6.7.8:1080#SOCKS-Node2"

	cfg, err := parser.ParseAndConvert(raw, 10)
	if err != nil {
		t.Fatalf("ParseAndConvert SOCKS5 error: %v", err)
	}

	socksOpt, ok := cfg.Options.Options.(*option.SOCKSOutboundOptions)
	if !ok {
		t.Fatalf("expected *option.SOCKSOutboundOptions, got %T", cfg.Options.Options)
	}
	if socksOpt.Server != "5.6.7.8" || socksOpt.ServerPort != 1080 {
		t.Errorf("Server/Port mismatch: %s:%d", socksOpt.Server, socksOpt.ServerPort)
	}
	if socksOpt.Username != "user2" || socksOpt.Password != "pass2" {
		t.Errorf("credentials mismatch: %s / %s", socksOpt.Username, socksOpt.Password)
	}
}

// TestParseBareAddressAsSOCKS pins the rule that a bare "ip:port" entry — one
// with no scheme or any other protocol information — is classified as SOCKS5.
func TestParseBareAddressAsSOCKS(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantHost  string
		wantPort  uint16
		wantError bool
	}{
		{name: "IPv4 with common SOCKS5 port", raw: "1.2.3.4:1080", wantHost: "1.2.3.4", wantPort: 1080},
		{name: "IPv4 with max port", raw: "255.255.255.255:65535", wantHost: "255.255.255.255", wantPort: 65535},
		{name: "bracketed IPv6", raw: "[2001:db8::1]:1080", wantHost: "2001:db8::1", wantPort: 1080},
		{name: "surrounding whitespace is trimmed", raw: "  5.6.7.8:1080  ", wantHost: "5.6.7.8", wantPort: 1080},
		{name: "hostname is not a bare address", raw: "example.com:1080", wantError: true},
		{name: "missing port", raw: "1.2.3.4", wantError: true},
		{name: "zero port", raw: "1.2.3.4:0", wantError: true},
		{name: "port out of range", raw: "1.2.3.4:70000", wantError: true},
		{name: "non-numeric port", raw: "1.2.3.4:https", wantError: true},
		{name: "trailing junk after port", raw: "1.2.3.4:1080:9", wantError: true},
		{name: "remark makes it a share link, not a bare address", raw: "1.2.3.4:1080#node", wantError: true},
		{name: "incomplete IPv4 literal", raw: "1.2.3:1080", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parser.ParseAndConvert(tt.raw, 20)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ParseAndConvert(%q) unexpectedly succeeded", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAndConvert(%q) error: %v", tt.raw, err)
			}

			if cfg.Protocol != "socks" {
				t.Errorf("expected Protocol socks, got %s", cfg.Protocol)
			}
			if cfg.Host != tt.wantHost || cfg.Port != tt.wantPort {
				t.Errorf("Host/Port mismatch: %s:%d, want %s:%d", cfg.Host, cfg.Port, tt.wantHost, tt.wantPort)
			}
			if cfg.RawLink != strings.TrimSpace(tt.raw) {
				t.Errorf("RawLink mismatch: %q, want %q", cfg.RawLink, strings.TrimSpace(tt.raw))
			}

			socksOpt, ok := cfg.Options.Options.(*option.SOCKSOutboundOptions)
			if !ok {
				t.Fatalf("expected *option.SOCKSOutboundOptions, got %T", cfg.Options.Options)
			}
			if socksOpt.Server != tt.wantHost || socksOpt.ServerPort != tt.wantPort {
				t.Errorf("Server/Port mismatch: %s:%d", socksOpt.Server, socksOpt.ServerPort)
			}
			if socksOpt.Version != "5" {
				t.Errorf("Version mismatch: %s", socksOpt.Version)
			}
			if socksOpt.Username != "" || socksOpt.Password != "" {
				t.Errorf("bare address must be unauthenticated, got %s / %s", socksOpt.Username, socksOpt.Password)
			}
		})
	}
}

func TestIsBareAddress(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1.2.3.4:1080", true},
		{"0.0.0.0:1", true},
		{"127.0.0.1:8080", true},
		{"[::1]:1080", true},
		{"[2001:db8::1]:443", true},
		{"", false},
		{"1.2.3.4", false},
		{"1.2.3.4:", false},
		{"1.2.3.4:0", false},
		{"1.2.3.4:65536", false},
		{"256.1.1.1:1080", false},
		{"1.2.3:1080", false},
		{"example.com:1080", false},
		{"localhost:1080", false},
		{"::1:1080", false}, // unbracketed IPv6 is ambiguous and rejected
		{"socks5://1.2.3.4:1080", false},
		{"ss://1.2.3.4:1080", false},
		{"user@1.2.3.4:1080", false},
		{"1.2.3.4:1080/path", false},
		{"1.2.3.4:1080?query=1", false},
		{"1.2.3.4:1080#remark", false},
	}

	for _, tt := range tests {
		if got := parser.IsBareAddress(tt.input); got != tt.want {
			t.Errorf("IsBareAddress(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestDeduplicateConfigsBareAddresses checks that bare addresses flow through
// the normal pipeline — deduplicated, indexed, and kept distinct from an
// explicit socks5:// link to the same server.
func TestDeduplicateConfigsBareAddresses(t *testing.T) {
	rawLinks := []string{
		"1.2.3.4:1080",
		"ss://YWVzLTEyOC1nY206cGFzc0AxLjEuMS4xOjQ0Mw==#Node4",
		"1.2.3.4:1080", // duplicate bare address
		"socks5://user:pass@1.2.3.4:1080#ExplicitSOCKS",
		"[2001:db8::1]:1080",
	}

	configs := parser.DeduplicateConfigs(rawLinks)
	if len(configs) != 4 {
		t.Fatalf("expected 4 configs, got %d: %+v", len(configs), configs)
	}

	wantProtocols := []string{"socks", "shadowsocks", "socks", "socks"}
	for i, want := range wantProtocols {
		if configs[i].Protocol != want {
			t.Errorf("configs[%d].Protocol = %s, want %s", i, configs[i].Protocol, want)
		}
	}
	if configs[0].Index != 1 || configs[0].Host != "1.2.3.4" || configs[0].Port != 1080 {
		t.Errorf("bare address config mismatch: index=%d %s:%d", configs[0].Index, configs[0].Host, configs[0].Port)
	}
	if configs[2].RawLink != "socks5://user:pass@1.2.3.4:1080#ExplicitSOCKS" {
		t.Errorf("explicit socks5 link should stay distinct from the bare address, got %q", configs[2].RawLink)
	}
}

func TestDeduplicateConfigsBareAddressMatchesSchemelessSOCKS(t *testing.T) {
	cases := []struct {
		name  string
		raws  []string
		count int
	}{
		{"socks5 scheme before bare", []string{"socks5://148.251.68.232:8888", "148.251.68.232:8888"}, 1},
		{"bare before socks5 scheme", []string{"148.251.68.232:8888", "socks5://148.251.68.232:8888"}, 1},
		{"socks scheme before bare", []string{"socks://1.2.3.4:1080", "1.2.3.4:1080"}, 1},
		{"uppercase scheme before bare", []string{"SOCKS5://1.2.3.4:1080", "1.2.3.4:1080"}, 1},
		{"ipv6 scheme before bare", []string{"socks5://[2001:db8::1]:1080", "[2001:db8::1]:1080"}, 1},
		{"remark on scheme link", []string{"socks5://1.2.3.4:1080#Node", "1.2.3.4:1080"}, 1},
		{"different ports stay distinct", []string{"socks5://1.2.3.4:1080", "1.2.3.4:1081"}, 2},
		{"userinfo link stays distinct from bare", []string{"socks5://user:pass@1.2.3.4:1080", "1.2.3.4:1080"}, 2},
		{"non-socks links unaffected", []string{"trojan://pass@example.com:443#A", "example.com:443"}, 1},
	}

	for _, tc := range cases {
		configs := parser.DeduplicateConfigs(tc.raws)
		if len(configs) != tc.count {
			t.Errorf("%s: expected %d configs, got %d: %+v", tc.name, tc.count, len(configs), configs)
		}
	}
}

func TestDeduplicateAppendBareAddressMatchesSchemelessSOCKSAcrossBatches(t *testing.T) {
	seen := make(map[string]bool)

	first, next := parser.DeduplicateAppend([]string{"148.251.68.232:8888"}, seen, 1)
	second, _ := parser.DeduplicateAppend([]string{"socks5://148.251.68.232:8888"}, seen, next)

	if len(first) != 1 || len(second) != 0 {
		t.Fatalf("expected the schemeless socks link to be dropped across batches, got first=%d second=%d", len(first), len(second))
	}
}

func TestParseHysteria2(t *testing.T) {
	raw := "hysteria2://mypassword@hy2.example.com:8443?obfs=salamander&obfs-password=obfspass&sni=sni.example.com&insecure=1&up=50&down=120#HY2-Node"

	cfg, err := parser.ParseAndConvert(raw, 11)
	if err != nil {
		t.Fatalf("ParseAndConvert Hysteria2 error: %v", err)
	}

	if cfg.Protocol != "hysteria2" {
		t.Errorf("expected Protocol hysteria2, got %s", cfg.Protocol)
	}
	if cfg.Host != "hy2.example.com" || cfg.Port != 8443 {
		t.Errorf("Host/Port mismatch: %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.Options.Type != "hysteria2" || cfg.Options.Tag != "proxy" {
		t.Errorf("expected Type hysteria2, Tag proxy; got Type %s, Tag %s", cfg.Options.Type, cfg.Options.Tag)
	}

	hysteria2Opt, ok := cfg.Options.Options.(*option.Hysteria2OutboundOptions)
	if !ok {
		t.Fatalf("expected *option.Hysteria2OutboundOptions, got %T", cfg.Options.Options)
	}
	if hysteria2Opt.Server != "hy2.example.com" || hysteria2Opt.ServerPort != 8443 {
		t.Errorf("Server/Port mismatch: %s:%d", hysteria2Opt.Server, hysteria2Opt.ServerPort)
	}
	if hysteria2Opt.Password != "mypassword" {
		t.Errorf("Password mismatch: %s", hysteria2Opt.Password)
	}
	if hysteria2Opt.TLS == nil || !hysteria2Opt.TLS.Enabled {
		t.Fatal("expected TLS options to be enabled")
	}
	if hysteria2Opt.TLS.ServerName != "sni.example.com" {
		t.Errorf("TLS ServerName mismatch: %s", hysteria2Opt.TLS.ServerName)
	}
	if !hysteria2Opt.TLS.Insecure {
		t.Error("expected insecure TLS to be enabled")
	}
	if hysteria2Opt.Obfs == nil || hysteria2Opt.Obfs.Type != "salamander" || hysteria2Opt.Obfs.Password != "obfspass" {
		t.Errorf("Obfs mismatch: %+v", hysteria2Opt.Obfs)
	}
	if hysteria2Opt.UpMbps != 50 || hysteria2Opt.DownMbps != 120 {
		t.Errorf("bandwidth mismatch: up=%d down=%d", hysteria2Opt.UpMbps, hysteria2Opt.DownMbps)
	}
	if len(hysteria2Opt.ServerPorts) != 0 {
		t.Errorf("expected no port hopping, got %v", hysteria2Opt.ServerPorts)
	}
}

func TestParseHysteria2DefaultsAndPortHopping(t *testing.T) {
	t.Run("userpass auth with default port and mport", func(t *testing.T) {
		raw := "hy2://user:secret@hy2.example.com/?mport=443-8443#HY2-Hopping"

		cfg, err := parser.ParseAndConvert(raw, 12)
		if err != nil {
			t.Fatalf("ParseAndConvert Hysteria2 hopping error: %v", err)
		}

		hysteria2Opt, ok := cfg.Options.Options.(*option.Hysteria2OutboundOptions)
		if !ok {
			t.Fatalf("expected *option.Hysteria2OutboundOptions, got %T", cfg.Options.Options)
		}
		if cfg.Port != 443 || hysteria2Opt.ServerPort != 443 {
			t.Errorf("expected default port 443, got %d", cfg.Port)
		}
		if hysteria2Opt.Password != "user:secret" {
			t.Errorf("expected full userpass auth, got %s", hysteria2Opt.Password)
		}
		if len(hysteria2Opt.ServerPorts) != 1 || hysteria2Opt.ServerPorts[0] != "443:8443" {
			t.Errorf("server_ports mismatch: %v", hysteria2Opt.ServerPorts)
		}
	})

	t.Run("multi port authority", func(t *testing.T) {
		raw := "hysteria2://secret@hy2.example.com:5000,5001-5003#HY2-Multi"

		cfg, err := parser.ParseAndConvert(raw, 13)
		if err != nil {
			t.Fatalf("ParseAndConvert Hysteria2 multi port error: %v", err)
		}

		hysteria2Opt, ok := cfg.Options.Options.(*option.Hysteria2OutboundOptions)
		if !ok {
			t.Fatalf("expected *option.Hysteria2OutboundOptions, got %T", cfg.Options.Options)
		}
		if cfg.Port != 5000 || hysteria2Opt.ServerPort != 5000 {
			t.Errorf("expected primary port 5000, got %d", cfg.Port)
		}
		expected := []string{"5000:5000", "5001:5003"}
		if len(hysteria2Opt.ServerPorts) != len(expected) {
			t.Fatalf("server_ports mismatch: %v", hysteria2Opt.ServerPorts)
		}
		for i, want := range expected {
			if hysteria2Opt.ServerPorts[i] != want {
				t.Errorf("server_ports[%d] = %s, want %s", i, hysteria2Opt.ServerPorts[i], want)
			}
		}
	})
}

func TestParseHysteria2PinSHA256(t *testing.T) {
	pin := strings.Repeat("ab", 32)
	raw := fmt.Sprintf("hysteria2://secret@1.2.3.4:443?pinSHA256=%s#HY2-Pinned", pin)

	cfg, err := parser.ParseAndConvert(raw, 14)
	if err != nil {
		t.Fatalf("ParseAndConvert Hysteria2 pin error: %v", err)
	}

	hysteria2Opt, ok := cfg.Options.Options.(*option.Hysteria2OutboundOptions)
	if !ok {
		t.Fatalf("expected *option.Hysteria2OutboundOptions, got %T", cfg.Options.Options)
	}
	if len(hysteria2Opt.TLS.CertificatePublicKeySHA256) != 1 {
		t.Fatalf("expected a pinned public key hash, got %v", hysteria2Opt.TLS.CertificatePublicKeySHA256)
	}
	if len(hysteria2Opt.TLS.CertificatePublicKeySHA256[0]) != 32 {
		t.Errorf("expected a 32 byte digest, got %d bytes", len(hysteria2Opt.TLS.CertificatePublicKeySHA256[0]))
	}
}

func TestParseErrorsNewProtocols(t *testing.T) {
	invalidLinks := []string{
		"socks://1.2.3.4",
		"hysteria2://1.2.3.4:443",
		"hysteria2://secret@1.2.3.4:443?obfs=unknown&obfs-password=x",
		"hysteria2://secret@1.2.3.4:0",
		"wireguard://privatekey@1.2.3.4:51820?publickey=publickey",
		"wireguard://privatekey@1.2.3.4:51820?address=10.0.0.2/32",
	}

	for _, raw := range invalidLinks {
		if _, err := parser.ParseAndConvert(raw, 1); err == nil {
			t.Errorf("expected error for link %q, got nil", raw)
		}
	}
}
