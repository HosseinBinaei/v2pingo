package tester

import (
	"context"
	"fmt"
	"net"
	"time"

	"v2pingo/pkg/types"
)

// ShouldSkipPrecheck reports whether phase-1 TCP probing is meaningless for
// the given protocol. UDP-based transports (WireGuard, Hysteria2) never
// complete a TCP handshake, and local outbounds (direct) have no remote
// server, so they must go directly to phase 2.
func ShouldSkipPrecheck(cfg types.ProxyConfig) bool {
	switch cfg.Protocol {
	case "wireguard", "hysteria2", "direct":
		return true
	default:
		return false
	}
}

// TCPPrecheck is phase 1 (method A): a cheap raw TCP dial to the proxy's own
// Host:Port without building a sing-box instance. A nil return means the node
// is worth testing end-to-end in phase 2; any error means fail fast here.
func TCPPrecheck(ctx context.Context, cfg types.ProxyConfig, timeout time.Duration) error {
	if ShouldSkipPrecheck(cfg) {
		return nil
	}
	if cfg.Host == "" || cfg.Port == 0 {
		return fmt.Errorf("precheck: empty target %q", cfg.Target())
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	preCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := dialer.DialContext(preCtx, "tcp", cfg.Target())
	if err != nil {
		return fmt.Errorf("precheck: %w", err)
	}
	_ = conn.Close()
	return nil
}

// precheckTimeout caps the cheap phase-1 probe so dead nodes fail fast
// instead of consuming the full end-to-end timeout.
func precheckTimeout(timeout time.Duration) time.Duration {
	const maxPrecheck = 2 * time.Second
	if timeout <= 0 {
		return maxPrecheck
	}
	if timeout < maxPrecheck {
		return timeout
	}
	return maxPrecheck
}
