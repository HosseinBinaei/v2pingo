package tester

import (
	"context"
	"net"
	"testing"
	"time"

	"v2pingo/pkg/types"
)

// Phase 1 (method A): cheap TCP dial without sing-box.
// Closed ports must fail fast without building a box.
func TestTCPPrecheck_ClosedPortFails(t *testing.T) {
	// Bind then close to get a guaranteed-closed local port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	var port uint16
	for _, c := range portStr {
		port = port*10 + uint16(c-'0')
	}

	cfg := types.ProxyConfig{Index: 1, Protocol: "vless", Host: host, Port: port}
	ctx := context.Background()
	if err := TCPPrecheck(ctx, cfg, 2*time.Second); err == nil {
		t.Fatal("expected TCPPrecheck to fail on a closed port")
	}
}

func TestTCPPrecheck_OpenPortPasses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	addr := ln.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)
	var port uint16
	for _, c := range portStr {
		port = port*10 + uint16(c-'0')
	}

	cfg := types.ProxyConfig{Index: 1, Protocol: "vless", Host: host, Port: port}
	if err := TCPPrecheck(context.Background(), cfg, 2*time.Second); err != nil {
		t.Fatalf("expected TCPPrecheck to pass on open port, got: %v", err)
	}
}

// UDP-based protocols cannot be validated with a TCP dial; they must skip
// phase 1 and go directly to phase 2.
func TestTCPPrecheck_SkipsUDPProtocols(t *testing.T) {
	for _, proto := range []string{"wireguard", "hysteria2"} {
		cfg := types.ProxyConfig{Index: 1, Protocol: proto, Host: "192.0.2.1", Port: 1}
		if err := TCPPrecheck(context.Background(), cfg, time.Second); err != nil {
			t.Errorf("expected %s to skip TCP precheck, got: %v", proto, err)
		}
		if !ShouldSkipPrecheck(cfg) {
			t.Errorf("expected ShouldSkipPrecheck(%s) = true", proto)
		}
	}
}
