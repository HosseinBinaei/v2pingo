package tester

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"v2pingo/pkg/types"
)

func CreateBoxContext(ctx context.Context) context.Context {
	return box.Context(
		ctx,
		include.InboundRegistry(),
		include.OutboundRegistry(),
		include.EndpointRegistry(),
		include.DNSTransportRegistry(),
		include.ServiceRegistry(),
		include.CertificateProviderRegistry(),
	)
}

func TestProxy(ctx context.Context, cfg types.ProxyConfig, targetURL string, timeout time.Duration) types.TestResult {
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	boxCtx := CreateBoxContext(ctx)

	boxOptions := option.Options{
		Log: &option.LogOptions{
			Disabled: true,
			Level:    "panic",
		},
	}
	// WireGuard is exposed by sing-box as an endpoint, every other supported
	// protocol is a regular outbound.
	if cfg.Endpoint != nil {
		boxOptions.Endpoints = []option.Endpoint{*cfg.Endpoint}
	} else {
		boxOptions.Outbounds = []option.Outbound{cfg.Options}
	}

	b, err := box.New(box.Options{
		Context: boxCtx,
		Options: boxOptions,
	})
	if err != nil {
		return types.TestResult{
			Config:  cfg,
			Success: false,
			Error:   err,
		}
	}

	if err := b.Start(); err != nil {
		return types.TestResult{
			Config:  cfg,
			Success: false,
			Error:   err,
		}
	}
	defer b.Close()

	dialer, err := resolveDialer(b, cfg)
	if err != nil {
		return types.TestResult{
			Config:  cfg,
			Success: false,
			Error:   err,
		}
	}

	transport := &http.Transport{
		DialContext: func(dc context.Context, network, addr string) (net.Conn, error) {
			destination := M.ParseSocksaddr(addr)
			return dialer.DialContext(dc, network, destination)
		},
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: timeout,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	req, err := http.NewRequestWithContext(tCtx, "GET", targetURL, nil)
	if err != nil {
		return types.TestResult{
			Config:  cfg,
			Success: false,
			Error:   err,
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return types.TestResult{
			Config:  cfg,
			Success: false,
			Error:   err,
		}
	}
	defer resp.Body.Close()

	_, _ = io.CopyN(io.Discard, resp.Body, 4096)
	latency := time.Since(start)

	return types.TestResult{
		Config:     cfg,
		Success:    true,
		StatusCode: resp.StatusCode,
		Latency:    latency,
	}
}

// resolveDialer returns the in-process dialer of the tested node. Nodes backed by
// an endpoint (WireGuard) are resolved through the endpoint manager, every other
// node through the outbound manager.
func resolveDialer(b *box.Box, cfg types.ProxyConfig) (N.Dialer, error) {
	if cfg.Endpoint != nil {
		endpoint, ok := b.Endpoint().Get(cfg.Endpoint.Tag)
		if !ok {
			return nil, fmt.Errorf("endpoint %q not found in box", cfg.Endpoint.Tag)
		}
		return endpoint, nil
	}

	outbound, ok := b.Outbound().Outbound(cfg.Options.Tag)
	if !ok {
		return nil, fmt.Errorf("outbound %q not found in box", cfg.Options.Tag)
	}
	return outbound, nil
}

func RunPool(ctx context.Context, configs []types.ProxyConfig, targetURL string, concurrency int, timeout time.Duration, onResult func(types.TestResult)) {
	if concurrency <= 0 {
		concurrency = 20
	}

	jobs := make(chan types.ProxyConfig, len(configs))
	for _, cfg := range configs {
		jobs <- cfg
	}
	close(jobs)

	results := make(chan types.TestResult, len(configs))

	workerCount := concurrency
	if workerCount > len(configs) && len(configs) > 0 {
		workerCount = len(configs)
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case cfg, ok := <-jobs:
					if !ok {
						return
					}
					res := TestProxy(ctx, cfg, targetURL, timeout)
					select {
					case <-ctx.Done():
						return
					case results <- res:
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case res, ok := <-results:
			if !ok {
				return
			}
			if onResult != nil {
				onResult(res)
			}
		}
	}
}
