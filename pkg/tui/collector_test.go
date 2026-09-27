package tui

import (
	"testing"

	"v2pingo/pkg/types"
)

func TestCollector_KeepsOnly2xxLinks(t *testing.T) {
	c := &Collector{}
	mk := func(code int, ok bool, link string) types.TestResult {
		return types.TestResult{
			Config:     types.ProxyConfig{RawLink: link},
			Success:    ok,
			StatusCode: code,
		}
	}
	c.OnResult(mk(200, true, "vless://a#200"))
	c.OnResult(mk(201, true, "vless://a#201"))
	c.OnResult(mk(299, true, "vless://a#299"))
	c.OnResult(mk(199, true, "vless://a#199"))
	c.OnResult(mk(300, true, "vless://a#300"))
	c.OnResult(mk(404, true, "vless://a#404"))
	c.OnResult(mk(201, false, "vless://a#201-fail"))
	c.OnResult(mk(204, true, ""))

	tested, success, failed, links := c.Snapshot()
	if tested != 8 {
		t.Fatalf("expected tested=8, got %d", tested)
	}
	if success != 7 {
		t.Fatalf("expected success=7, got %d", success)
	}
	if failed != 1 {
		t.Fatalf("expected failed=1, got %d", failed)
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 saved links, got %v", links)
	}
}
