package tester

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"

	"v2pingo/pkg/types"
)

func TestCreateBoxContext(t *testing.T) {
	ctx := context.Background()
	boxCtx := CreateBoxContext(ctx)
	if boxCtx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestTestProxyDirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello proxy"))
	}))
	defer srv.Close()

	cfg := types.ProxyConfig{
		Index:    1,
		Protocol: "direct",
		Host:     "127.0.0.1",
		Port:     80,
		Options: option.Outbound{
			Type: "direct",
			Tag:  "proxy",
		},
	}

	ctx := context.Background()
	res := TestProxy(ctx, cfg, srv.URL, 5*time.Second)

	if !res.Success {
		t.Fatalf("expected success, got error: %v", res.Error)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}
}

func TestRunPool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []types.ProxyConfig{
		{
			Index:    1,
			Protocol: "direct",
			Options: option.Outbound{
				Type: "direct",
				Tag:  "proxy",
			},
		},
		{
			Index:    2,
			Protocol: "direct",
			Options: option.Outbound{
				Type: "direct",
				Tag:  "proxy",
			},
		},
	}

	var mu sync.Mutex
	var results []types.TestResult

	ctx := context.Background()
	RunPool(ctx, configs, srv.URL, 2, 5*time.Second, func(res types.TestResult) {
		mu.Lock()
		results = append(results, res)
		mu.Unlock()
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("expected proxy test success for index %d, got error: %v", r.Config.Index, r.Error)
		}
	}
}

// randomWireGuardKey returns a valid 32 byte WireGuard key in base64.
func randomWireGuardKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate a WireGuard key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// TestTestProxyWireGuardEndpoint verifies that endpoint based nodes (WireGuard) are
// accepted by the in-process box and reach the dial stage instead of failing with
// a configuration error.
func TestTestProxyWireGuardEndpoint(t *testing.T) {
	cfg := types.ProxyConfig{
		Index:    1,
		Protocol: "wireguard",
		Host:     "127.0.0.1",
		Port:     2408,
		Endpoint: &option.Endpoint{
			Type: "wireguard",
			Tag:  "proxy",
			Options: &option.WireGuardEndpointOptions{
				MTU:        1408,
				Address:    badoption.Listable[netip.Prefix]{netip.MustParsePrefix("10.0.0.2/32")},
				PrivateKey: randomWireGuardKey(t),
				Peers: []option.WireGuardPeer{
					{
						Address:    "127.0.0.1",
						Port:       2408,
						PublicKey:  randomWireGuardKey(t),
						AllowedIPs: badoption.Listable[netip.Prefix]{netip.MustParsePrefix("0.0.0.0/0")},
					},
				},
			},
		},
	}

	// The peer address is a black hole, so the handshake never completes and the
	// test has to fail through the dial timeout instead of a schema error.
	res := TestProxy(context.Background(), cfg, "http://10.0.0.1:80/", 2*time.Second)

	if res.Success {
		t.Fatal("expected the WireGuard test to fail without a reachable peer")
	}
	if res.Error == nil {
		t.Fatal("expected an error result")
	}

	for _, configError := range []string{
		"missing allowed ips",
		"invalid reserved value",
		"not ready yet",
		"not found in box",
		"unknown endpoint type",
		"missing endpoint registry",
	} {
		if strings.Contains(res.Error.Error(), configError) {
			t.Errorf("unexpected endpoint configuration error: %v", res.Error)
		}
	}
}

// TestResolveDialerUnknownTag verifies the descriptive error path when a node does
// not expose the expected tag.
func TestResolveDialerUnknownTag(t *testing.T) {
	ctx := CreateBoxContext(context.Background())

	b, err := box.New(box.Options{
		Context: ctx,
		Options: option.Options{
			Log:       &option.LogOptions{Disabled: true, Level: "panic"},
			Outbounds: []option.Outbound{{Type: "direct", Tag: "proxy"}},
		},
	})
	if err != nil {
		t.Fatalf("failed to create box: %v", err)
	}
	defer b.Close()

	if err := b.Start(); err != nil {
		t.Fatalf("failed to start box: %v", err)
	}

	if _, err := resolveDialer(b, types.ProxyConfig{Options: option.Outbound{Tag: "missing"}}); err == nil {
		t.Error("expected an error for a missing outbound tag")
	}

	dialer, err := resolveDialer(b, types.ProxyConfig{Options: option.Outbound{Tag: "proxy"}})
	if err != nil {
		t.Fatalf("expected the direct outbound to resolve, got: %v", err)
	}
	if dialer == nil {
		t.Error("expected a non-nil dialer")
	}
}
