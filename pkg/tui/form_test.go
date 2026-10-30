package tui

import (
	"testing"
	"time"
)

func TestValidateForm_AcceptsGoodInput(t *testing.T) {
	cfg, errMsg := ValidateForm([]string{"https://a.com/sub.txt"}, "", "https://cloudflare.com", "20", "15s")
	if errMsg != "" {
		t.Fatalf("expected valid form, got error %q", errMsg)
	}
	if cfg.Workers != 20 || cfg.Timeout != 15*time.Second || cfg.TargetURL != "https://cloudflare.com" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if len(cfg.Subs) != 1 {
		t.Fatalf("expected 1 sub, got %+v", cfg)
	}
}

func TestValidateForm_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		subs    []string
		extra   string
		target  string
		workers string
		timeout string
	}{
		{"no subs", nil, "", "https://cloudflare.com", "20", "15s"},
		{"bad workers zero", []string{"https://a.com/s"}, "", "https://cloudflare.com", "0", "15s"},
		{"bad workers huge", []string{"https://a.com/s"}, "", "https://cloudflare.com", "500", "15s"},
		{"bad workers alpha", []string{"https://a.com/s"}, "", "https://cloudflare.com", "abc", "15s"},
		{"bad timeout", []string{"https://a.com/s"}, "", "https://cloudflare.com", "20", "soon"},
		{"empty target", []string{"https://a.com/s"}, "", "", "20", "15s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, errMsg := ValidateForm(tc.subs, tc.extra, tc.target, tc.workers, tc.timeout); errMsg == "" {
				t.Fatalf("expected error for case %q", tc.name)
			}
		})
	}
}

func TestValidateForm_ExtraSubAppended(t *testing.T) {
	cfg, errMsg := ValidateForm([]string{"https://a.com/s"}, "https://b.com/s", "https://cloudflare.com", "5", "10s")
	if errMsg != "" {
		t.Fatalf("expected valid form, got %q", errMsg)
	}
	if len(cfg.Subs) != 2 || cfg.Subs[1] != "https://b.com/s" {
		t.Fatalf("expected extra sub appended, got %+v", cfg.Subs)
	}
}
