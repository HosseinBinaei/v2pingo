package tui

import (
	"sync"

	"v2pingo/pkg/types"
)

// Collector is the TUI-side equivalent of reporter.Reporter accounting.
// It is safe for concurrent use from tester.RunPool worker callbacks.
type Collector struct {
	mu      sync.Mutex
	tested  int
	success int
	failed  int
	links   []string
}

// OnResult records one TestResult. Only Success results with a 2xx status
// and a non-empty RawLink are kept for the results file.
func (c *Collector) OnResult(res types.TestResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tested++
	if res.Success {
		c.success++
		if res.StatusCode >= 200 && res.StatusCode < 300 && res.Config.RawLink != "" {
			c.links = append(c.links, res.Config.RawLink)
		}
		return
	}
	c.failed++
}

// Snapshot returns a copy of the current counters and saved links.
func (c *Collector) Snapshot() (tested, success, failed int, links []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.links))
	copy(out, c.links)
	return c.tested, c.success, c.failed, out
}
