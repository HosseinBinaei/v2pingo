# V2Ray-Compatible Proxy Tester (sing-box Core MVP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a lightweight, high-performance Go CLI program compiled to a single Windows `.exe` to fetch subscriptions, parse/deduplicate VLESS, VMess, Trojan, and Shadowsocks proxies, test them concurrently through in-process sing-box dialers, and print a sanitized progress and summary report.

**Architecture:** A producer-consumer pipeline with fixed-size worker pool:
1. Subscription Fetcher: downloads plain/base64 subscriptions and converts github blob URLs.
2. Parser & Converter: parses URIs into normalized models and builds sing-box `option.Outbound` structs.
3. Tester: spins up ephemeral in-process `box.Box` instances with custom context registries and routes `http.Client` traffic via `outbound.DialContext` without binding local ports.
4. Reporter: thread-safe live logger and summary output with strict sanitization of sensitive credentials.

**Tech Stack:** Go 1.22+, `github.com/sagernet/sing-box` (v1.14.0), Go standard library (`net/http`, `crypto`, `context`, `flag`, `os/signal`).

**Spec:** `docs/superpowers/specs/2026-09-12-v2ray-proxy-tester-design.md`

## Global Constraints

- Platform: Windows amd64 `.exe` runnable binary.
- Dependencies: Pure Go in-process sing-box library (no external binaries or child processes).
- Output: Sanitized live progress and final summary; never expose passwords, UUIDs, tokens, or Reality keys.
- Defaults: 5 default subscriptions, concurrency = 20, timeout = 15s, target URL = `https://x.com`.

---

### Task 1: Project Scaffolding & Core Domain Types

**Files:**
- Modify: `go.mod`
- Create: `pkg/types/config.go`
- Test: `pkg/types/config_test.go`

**Interfaces:**
- Produces:
  ```go
  type ProxyConfig struct {
      Index    int
      Protocol string // "vless", "vmess", "trojan", "shadowsocks"
      Host     string
      Port     uint16
      RawLink  string
      Options  option.Outbound
  }
  type TestResult struct {
      Config     ProxyConfig
      Success    bool
      StatusCode int
      Latency    time.Duration
      Error      error
  }
  ```

- [ ] **Step 1: Write test for core domain models**
- [ ] **Step 2: Run test to verify it fails**
- [ ] **Step 3: Implement domain types**
- [ ] **Step 4: Run test to verify it passes**

---

### Task 2: Subscription Fetcher & Link Extractor

**Files:**
- Create: `pkg/subscription/fetcher.go`
- Test: `pkg/subscription/fetcher_test.go`

**Interfaces:**
- Produces:
  ```go
  var DefaultSubscriptions = []string{...}
  func NormalizeURL(rawURL string) string
  func DecodePayload(content []byte) []string
  func FetchSubscriptions(ctx context.Context, urls []string, client *http.Client) []string
  ```

- [ ] **Step 1: Write failing unit tests for GitHub blob URL rewriting, base64 / plain text decoding, and extraction of `vless://`, `vmess://`, `trojan://`, `ss://` links**
- [ ] **Step 2: Run tests to verify failures**
- [ ] **Step 3: Implement subscription fetcher, URL normalizer, and decoder**
- [ ] **Step 4: Run tests to verify they pass**

---

### Task 3: Share Link Parser & sing-box Outbound Converter

**Files:**
- Create: `pkg/parser/parser.go`
- Create: `pkg/parser/vless.go`
- Create: `pkg/parser/vmess.go`
- Create: `pkg/parser/trojan.go`
- Create: `pkg/parser/shadowsocks.go`
- Test: `pkg/parser/parser_test.go`

**Interfaces:**
- Consumes: `pkg/types.ProxyConfig`
- Produces:
  ```go
  func ParseAndConvert(rawLink string, index int) (*types.ProxyConfig, error)
  func DeduplicateConfigs(rawLinks []string) []types.ProxyConfig
  func StripRemark(rawLink string) string
  ```

- [ ] **Step 1: Write tests for VLESS (TCP, WS, gRPC, TLS, Reality, Vision), VMess (JSON format, WS, TLS), Trojan, and Shadowsocks link parsing and deduplication**
- [ ] **Step 2: Run tests to verify failures**
- [ ] **Step 3: Implement parser and outbound converter for all 4 protocols**
- [ ] **Step 4: Run tests to verify all test cases pass**

---

### Task 4: In-Process sing-box Tester & Worker Pool

**Files:**
- Create: `pkg/tester/box_context.go`
- Create: `pkg/tester/tester.go`
- Create: `pkg/tester/pool.go`
- Test: `pkg/tester/tester_test.go`

**Interfaces:**
- Consumes: `pkg/types.ProxyConfig`, `pkg/types.TestResult`
- Produces:
  ```go
  func TestProxy(ctx context.Context, cfg types.ProxyConfig, targetURL string, timeout time.Duration) types.TestResult
  func RunPool(ctx context.Context, configs []types.ProxyConfig, targetURL string, concurrency int, timeout time.Duration, onResult func(types.TestResult))
  ```

- [ ] **Step 1: Write tests for single proxy testing and worker pool concurrency / context cancellation**
- [ ] **Step 2: Run tests to verify failures**
- [ ] **Step 3: Implement minimal box context registry, in-process box lifecycle, HTTP dialer, and worker pool**
- [ ] **Step 4: Run tests to verify they pass**

---

### Task 5: Live Reporter & Final Summary

**Files:**
- Create: `pkg/reporter/reporter.go`
- Test: `pkg/reporter/reporter_test.go`

**Interfaces:**
- Consumes: `pkg/types.TestResult`
- Produces:
  ```go
  type Reporter struct { ... }
  func NewReporter(totalConfigs int, verbose bool) *Reporter
  func (r *Reporter) Record(res types.TestResult)
  func (r *Reporter) PrintSummary()
  ```

- [ ] **Step 1: Write tests verifying credential sanitization and summary calculation**
- [ ] **Step 2: Run tests to verify failures**
- [ ] **Step 3: Implement thread-safe live reporter and summary printer**
- [ ] **Step 4: Run tests to verify they pass**

---

### Task 6: CLI Runner & Binary Build

**Files:**
- Create: `cmd/v2pingo/main.go`
- Test: End-to-end integration test / binary compilation

**Interfaces:**
- Produces: `v2pingo.exe` Windows executable.

- [ ] **Step 1: Implement `main.go` with CLI flag parsing, signal handling (`os.Interrupt`, `syscall.SIGTERM`), subscription pipeline execution, and summary reporting**
- [ ] **Step 2: Build executable (`go build -o v2pingo.exe ./cmd/v2pingo`)**
- [ ] **Step 3: Verify CLI runs and shows help/flags and executes live test against default/custom URLs**
- [ ] **Step 4: Run full test suite (`go test ./...`)**
