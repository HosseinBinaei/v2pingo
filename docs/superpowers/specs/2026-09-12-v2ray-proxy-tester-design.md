# Design Specification: V2Ray-Compatible Proxy Tester (sing-box Core MVP)

- **Date:** 2026-09-12
- **Language:** Go
- **Target Platform:** Windows (single runnable `.exe`)
- **Status:** Approved

---

## 1. Overview & Goals

The goal is to provide a clean, high-performance, single-binary CLI application in Go to test V2Ray-compatible proxy configurations extracted from subscription endpoints using the **sing-box** engine in-process.

### Key Capabilities
- Fetch default and custom subscriptions (handling raw text, Base64, and mixed formats).
- Automatically rewrite GitHub `/blob/` subscription URLs to raw endpoints.
- Parse standard proxy share links: **VLESS** (with Reality, TLS, TCP, WS, gRPC, Vision), **VMess** (TCP, WS, gRPC, TLS), **Trojan** (TLS, TCP, WS, gRPC), and **Shadowsocks** (SIP002 and legacy base64 format).
- Normalize and deduplicate configurations before testing.
- Test proxies concurrently using an in-process sing-box memory dialer without opening local TCP listening ports.
- Issue real HTTP GET requests to a target URL (e.g. `https://x.com`), verify proxy connectivity, measure latency, and print sanitized progress.
- Support graceful cancellation on `Ctrl+C` and output a clean execution summary.

---

## 2. Architecture & Modules

```
                    ┌─────────────────────────┐
                    │       CLI Runner        │
                    │ (flags, signal handler) │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │   Subscription Fetcher   │
                    │  (download, base64/raw) │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │ Parser & Deduplicator   │
                    │  (vless, vmess, trojan, │
                    │   shadowsocks configs)  │
                    └────────────┬────────────┘
                                 │ jobs channel
                    ┌────────────▼────────────┐
                    │   Fixed Worker Pool     │
                    │ (N concurrent workers)  │
                    └────────────┬────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          ▼                      ▼                      ▼
┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐
│ Worker 1          │  │ Worker 2          │  │ Worker N          │
│ - Parse Options   │  │ - Parse Options   │  │ - Parse Options   │
│ - In-Process Box  │  │ - In-Process Box  │  │ - In-Process Box  │
│ - HTTP DialContext│  │ - HTTP DialContext│  │ - HTTP DialContext│
│ - GET & Latency   │  │ - GET & Latency   │  │ - GET & Latency   │
│ - Cleanup / Close │  │ - Cleanup / Close │  │ - Cleanup / Close │
└─────────┬─────────┘  └─────────┬─────────┘  └─────────┬─────────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │ results channel
                    ┌────────────▼────────────┐
                    │    Reporter / Summary   │
                    │ (sanitized live + final)│
                    └─────────────────────────┘
```

### Module Breakdown
1. **`cmd/v2pingo/main.go`**: Entry point, CLI flag parsing, signal routing, coordination.
2. **`pkg/subscription`**: Fetching subscription URLs (with 30s timeout and GitHub blob normalization), payload decoding (safe Base64 and line splitting), and extraction of candidate share links.
3. **`pkg/parser`**: Parsing individual share URIs (`vless://`, `vmess://`, `trojan://`, `ss://`) into structured configuration models and converting them into sing-box native `option.Outbound` definitions. Includes URI-fragment-stripped deduplication logic.
4. **`pkg/tester`**: Fixed worker pool executor, in-process sing-box instance lifecycle (`box.New`, outbound dialer extraction, context timeouts), HTTP transport wiring, and latency measurement.
5. **`pkg/reporter`**: Live sanitized output formatter and final summary aggregation.

---

## 3. Detailed Component Specifications

### 3.1 Subscriptions & Link Extraction
* **Default Subscriptions:**
  1. `https://raw.githubusercontent.com/Epodonios/v2ray-configs/main/All_Configs_Sub.txt`
  2. `https://raw.githubusercontent.com/barry-far/V2ray-Config/main/All_Configs_Sub.txt`
  3. `https://github.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt` (auto-normalized to raw)
  4. `https://raw.githubusercontent.com/MatinGhanbari/v2ray-configs/main/subscriptions/v2ray/all_sub.txt`
  5. `https://raw.githubusercontent.com/0xRadikal/Free-v2ray-Configs/main/all/configs.txt`
* **URL Normalization:** Transforms `github.com/<owner>/<repo>/blob/<branch>/<path>` to `raw.githubusercontent.com/<owner>/<repo>/<branch>/<path>`.
* **Decoding Logic:**
  * Checks for scheme prefixes directly (`vless://`, `vmess://`, `trojan://`, `ss://`).
  * If the string or line contains no scheme prefix, attempts Base64 decoding (standard and URL-safe, with or without padding). If the decoded result contains supported schemes, extracts all valid lines.
  * Preserves raw links including their original remarks/fragments.

### 3.2 Configuration Normalization & Deduplication
* **Signature Generation:** Strip `#<remark>` fragment from URI before hashing / mapping to deduplicate identical servers.
* **Model:**
  ```go
  type ProxyConfig struct {
      Index    int
      Protocol string // "vless", "vmess", "trojan", "shadowsocks"
      Host     string
      Port     uint16
      RawLink  string
      Options  option.Outbound
  }
  ```

### 3.3 Link Conversion to sing-box `option.Outbound`
* **VLESS:**
  * Maps UUID, Host, Port.
  * Security:
    * `reality`: TLS options with `Reality.Enabled = true`, `PublicKey`, `ShortID`.
    * `tls`: Standard TLS options with `ServerName` (SNI).
    * `none`: Plain TCP / unencrypted transport.
  * Flow: `xtls-rprx-vision` mapped to `Flow`.
  * Transport: TCP, WebSocket (`transport.WebSocketsOptions`), gRPC (`transport.GRPCOptions`), HTTP.
* **VMess:**
  * Decodes v2rayN JSON (`add`, `port`, `id`, `aid`, `net`, `type`, `host`, `path`, `tls`, `sni`).
  * Maps UUID, AlterId, TLS options, and corresponding transport options.
* **Trojan:**
  * Maps Password, Host, Port, TLS SNI, and transport options.
* **Shadowsocks:**
  * Parses SIP002 / legacy format (`method:password@host:port`).
  * Maps Method, Password, Host, Port.

### 3.4 In-Process sing-box Testing & Lifecycle
* **Box Construction:**
  ```go
  boxOptions := option.Options{
      Outbounds: []option.Outbound{config.Options},
  }
  ```
* **Test Cycle per Proxy:**
  1. Create context: `ctx, cancel := context.WithTimeout(rootCtx, timeout)` (default 15s).
  2. Initialize sing-box instance: `instance, err := box.New(box.Options{Options: boxOptions, Context: ctx})`.
  3. Start instance: `err = instance.Start()`.
  4. Obtain outbound dialer: `dialer, err := instance.Outbound().Get("proxy")` (or default outbound).
  5. Setup HTTP Transport:
     ```go
     transport := &http.Transport{
         DialContext: dialer.DialContext,
         DisableKeepAlives: true,
         TLSHandshakeTimeout: timeout,
     }
     ```
  6. Execute HTTP GET:
     ```go
     req, _ := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
     resp, err := client.Do(req)
     ```
  7. Read small body slice (up to 4KB) to verify end-to-end transport.
  8. Close response body, close idle connections, close sing-box box instance.
  9. Record result (Proxy success if HTTP response received, HTTP status code, latency).

### 3.5 CLI Flags & Output
* Flags:
  * `-url`: Target test URL (default `https://x.com`).
  * `-c`: Concurrency (default `20`).
  * `-timeout`: Timeout per test (default `15s`).
  * `-default-subs`: Enable built-in subscriptions (default `true`).
  * `-sub`: Additional subscription URL (repeatable / comma-separated).
  * `-v` / `-verbose`: Show failures in live stream (default `false`).
* Sanitization: Never log passwords, tokens, UUIDs, or Reality keys.
* Final summary output format:
  ```text
  ==================================================
  TEST SUMMARY
  ==================================================
  Total configs: 2500
  Tested:        2500
  Success:        180
  Failed:        2320
  Duration:      8m 12s
  ==================================================
  ```

---

## 4. Error Handling & Edge Cases
* **Subscription Failures:** Non-fatal; log warning and continue with remaining subscriptions.
* **Malformed Links:** Skipped safely without worker panic.
* **Context Timeouts:** Context cancellation terminates in-flight dial attempts immediately and marks the config as failed.
* **Signal Interruption (Ctrl+C):** Closes the job channel, cancels pending contexts, awaits in-flight worker cleanup, and prints partial summary before exiting.
