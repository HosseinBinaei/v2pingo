package types_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sagernet/sing-box/option"
	"v2pingo/pkg/types"
)

func TestProxyConfig(t *testing.T) {
	t.Run("Target formatting", func(t *testing.T) {
		cfg := types.ProxyConfig{
			Index:    1,
			Protocol: "vless",
			Host:     "example.com",
			Port:     443,
			RawLink:  "vless://user@example.com:443?type=ws",
			Options:  option.Outbound{},
		}

		expected := "example.com:443"
		if got := cfg.Target(); got != expected {
			t.Errorf("expected Target() = %q, got %q", expected, got)
		}
	})

	t.Run("Empty host and port", func(t *testing.T) {
		cfg := types.ProxyConfig{}
		expected := ":0"
		if got := cfg.Target(); got != expected {
			t.Errorf("expected Target() = %q, got %q", expected, got)
		}
	})
}

func TestTestResult(t *testing.T) {
	cfg := types.ProxyConfig{
		Index:    0,
		Protocol: "vmess",
		Host:     "1.2.3.4",
		Port:     8080,
	}

	resSuccess := types.TestResult{
		Config:     cfg,
		Success:    true,
		StatusCode: 200,
		Latency:    150 * time.Millisecond,
		Error:      nil,
	}

	if !resSuccess.Success {
		t.Errorf("expected Success to be true")
	}
	if resSuccess.StatusCode != 200 {
		t.Errorf("expected StatusCode 200, got %d", resSuccess.StatusCode)
	}
	if resSuccess.Latency != 150*time.Millisecond {
		t.Errorf("expected Latency 150ms, got %v", resSuccess.Latency)
	}
	if resSuccess.Error != nil {
		t.Errorf("expected nil error, got %v", resSuccess.Error)
	}

	testErr := errors.New("connection timed out")
	resFail := types.TestResult{
		Config:     cfg,
		Success:    false,
		StatusCode: 0,
		Latency:    0,
		Error:      testErr,
	}

	if resFail.Success {
		t.Errorf("expected Success to be false")
	}
	if resFail.Error == nil || resFail.Error.Error() != "connection timed out" {
		t.Errorf("expected 'connection timed out' error, got %v", resFail.Error)
	}
}
