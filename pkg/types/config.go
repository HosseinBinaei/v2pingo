package types

import (
	"fmt"
	"time"

	"github.com/sagernet/sing-box/option"
)

// ProxyConfig holds normalized configuration details for a parsed proxy node.
type ProxyConfig struct {
	Index    int
	Protocol string // "vless", "vmess", "trojan", "shadowsocks"
	Host     string
	Port     uint16
	RawLink  string
	Options  option.Outbound
}

// Target returns the formatted host:port string for the proxy destination.
func (c ProxyConfig) Target() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// TestResult represents the outcome of an end-to-end connectivity test for a proxy node.
type TestResult struct {
	Config     ProxyConfig
	Success    bool
	StatusCode int
	Latency    time.Duration
	Error      error
}
