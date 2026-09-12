# v2pingo

A fast, lightweight, and robust CLI tool for testing V2Ray-compatible proxy configurations extracted from subscriptions using the **sing-box core in-process**.

Built in Go for Windows (single runnable `.exe`), it tests configurations directly in memory without binding local TCP listening ports, measuring true round-trip HTTP latency with strict concurrency and timeout controls.

---

## Features

- **In-Process sing-box Core:** Direct in-memory dialer (`DialContext`) routing with zero local proxy listening ports (`127.0.0.1:XXXX`), avoiding local port exhaustion and firewall interference.
- **Multiple Protocol Support:**
  - **VLESS:** TCP, WebSocket, gRPC, TLS, Reality (`pbk`, `sid`, `fp`), and Vision flow (`xtls-rprx-vision`).
  - **VMess:** Standard v2rayN JSON format, TCP, WebSocket, gRPC, and TLS.
  - **Trojan:** TLS, WebSocket, and gRPC transports.
  - **Shadowsocks:** Standard SIP002 (plain / base64 userinfo) and legacy base64 URI formats.
- **Flexible Subscription Handling:**
  - Includes 4 curated default subscriptions out of the box.
  - Automatically rewrites GitHub `/blob/` URLs to `raw.githubusercontent.com`.
  - Decodes raw plain-text links, single/multiline Base64 (standard and URL-safe, padded and unpadded), and mixed subscription payloads.
  - Normalizes and deduplicates duplicate configurations (ignoring remark `#fragments` for uniqueness).
  - Resilient network fetching: failed subscriptions emit warnings without aborting the test batch.
- **Controlled Concurrency & Timeouts:**
  - Fixed worker pool (default: `20` concurrent workers).
  - Strict per-test context timeouts (default: `15s`) covering handshake, connection setup, and HTTP GET request.
  - Safe resource cleanup (`defer` for box instances, transports, idle connections, and HTTP bodies).
- **Clean & Sanitized Output:**
  - Real-time live status updates without leaking credentials, passwords, UUIDs, or Reality keys.
  - Optional verbose mode (`-v` / `-verbose`) for inspecting failures.
  - Graceful shutdown on `Ctrl+C` (`SIGINT` / `SIGTERM`) that halts workers safely and prints a summary for completed tests.

---

## Installation & Build

### Prerequisites
- [Go 1.22+](https://go.dev/dl/) installed on Windows.

### Build Executable
```powershell
# Clone or navigate to the repository
cd D:\v2-pingo

# Build the Windows executable
go build -o v2pingo.exe ./cmd/v2pingo
```

---

## Quick Start & Usage

### 1. Default Run
Fetches the 5 default subscriptions, extracts all supported configurations, deduplicates them, and tests against `https://x.com` with 20 workers:
```powershell
.\v2pingo.exe
```

### 2. Custom Target URL & Concurrency
Test against Google with 30 concurrent workers and a 10-second timeout:
```powershell
.\v2pingo.exe -url https://www.google.com/generate_204 -c 30 -timeout 10s
```

### 3. Custom Subscriptions
Disable the default subscriptions and test only your own subscription URLs:
```powershell
.\v2pingo.exe -default-subs=false -sub "https://example.com/mysub.txt"
```

You can pass multiple custom subscriptions either by repeating `-sub` or using comma-separated values:
```powershell
.\v2pingo.exe -sub "https://site1.com/sub.txt" -sub "https://site2.com/sub.txt"
```

### 4. Verbose Logging
Show failed tests in addition to successful ones:
```powershell
.\v2pingo.exe -v
```

---

## CLI Options

| Flag | Type | Default | Description |
|---|---|---|---|
| `-url` | `string` | `https://x.com` | Target test URL for end-to-end HTTP GET verification. |
| `-c` | `int` | `20` | Concurrency level (number of worker goroutines). |
| `-timeout` | `duration` | `15s` | Timeout for each proxy configuration test. |
| `-default-subs` | `bool` | `true` | Whether to include the 4 built-in subscriptions. |
| `-sub` | `string` | *(empty)* | Custom subscription URL(s) (repeatable or comma-separated). |
| `-v`, `-verbose` | `bool` | `false` | Enable verbose output (prints failed tests). |

---

## Output Examples

### Live Testing Output
```text
[✓ SUCCESS] [#14] [vless] 104.21.58.12:443 - 182ms (HTTP 200)
[✓ SUCCESS] [#28] [trojan] sg.proxy-node.net:443 - 245ms (HTTP 200)
[✓ SUCCESS] [#35] [vmess] us-east.server.org:80 - 310ms (HTTP 204)
```

*(When `-v` is enabled, failures are displayed with sanitized reasons)*:
```text
[✗ FAILED] [#15] [vless] 198.51.100.2:443 - context deadline exceeded
```

### Final Summary
```text
==================================================
TEST SUMMARY
==================================================
Total configs: 2480
Tested:        2480
Success:        185
Failed:        2295
Duration:      3m 42s
==================================================
```

---

## Built-In Default Subscriptions

1. `https://raw.githubusercontent.com/barry-far/V2ray-Config/main/All_Configs_Sub.txt`
2. `https://github.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt` (auto-rewritten to raw)
3. `https://raw.githubusercontent.com/MatinGhanbari/v2ray-configs/main/subscriptions/v2ray/all_sub.txt`
4. `https://raw.githubusercontent.com/0xRadikal/Free-v2ray-Configs/main/all/configs.txt`

---

## Running Tests

Run the full Go unit and integration test suite:
```powershell
go test -v ./...
```

---

## Architecture Overview

```
Subscription URLs (default + custom)
       │
       ▼
 [Fetcher & Normalizer]  ──► Plain / Base64 decode, GitHub blob rewrite
       │
       ▼
 [Parser & Deduplicator] ──► Parse VLESS / VMess / Trojan / SS & build sing-box options
       │
       ▼
 [Worker Pool (Fixed N)] ──► Ephemeral in-process Box instance (zero open listening ports)
       │                  ──► HTTP Client over outbound.DialContext
       ▼
 [Reporter & Sanitizer]  ──► Live progress & final summary
```

---

## License

MIT License.
