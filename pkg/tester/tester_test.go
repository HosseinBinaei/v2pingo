package tester

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/option"

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
