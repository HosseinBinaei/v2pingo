package parser_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
