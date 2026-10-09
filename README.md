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
  - **SOCKS:** `socks://` and `socks5://` links with plain, base64 and legacy fully-encoded userinfo.
  - **Hysteria2:** `hysteria2://` and `hy2://` links with URI scheme parameters (`sni`, `insecure`, `obfs`/`obfs-password`, `pinSHA256`, `up`/`down`) and port hopping (`mport`, multi-port authority).
  - **WireGuard:** `wireguard://` and `wg://` links (private key, `publickey`, `presharedkey`, `address`, `mtu`, `reserved`). WireGuard is tested through a sing-box **endpoint**, which is the only form sing-box still supports since the legacy outbound was removed.
- **Flexible Subscription Handling:**
  - Subscription URLs are read from [`sub-input.txt`](sub-input.txt) in the project root, one URL per line, so the list can be edited without rebuilding.
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
- **Shareable Results:**
  - After the results file is written, it is uploaded to [paste.rs](https://paste.rs) and the returned link is printed (both in the CLI and on the TUI done screen).
  - Disable publishing with `-paste=false`, or point it at another paste service with `-paste-url`.
  - Empty runs (no HTTP 2xx configurations) are skipped, so no link is produced and no request is sent.

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
Reads the subscription URLs listed in `sub-input.txt`, extracts all supported configurations, deduplicates them, and tests against `https://x.com` with 20 workers:
```powershell
.\v2pingo.exe
```

### 2. Custom Target URL & Concurrency
Test against Google with 30 concurrent workers and a 10-second timeout:
```powershell
.\v2pingo.exe -url https://www.google.com/generate_204 -c 30 -timeout 10s
```

### 3. Custom Subscriptions
Disable the subscription file and test only your own subscription URLs:
```powershell
.\v2pingo.exe -default-subs=false -sub "https://example.com/mysub.txt"
```

You can pass multiple custom subscriptions either by repeating `-sub` or using comma-separated values:
```powershell
.\v2pingo.exe -sub "https://site1.com/sub.txt" -sub "https://site2.com/sub.txt"
```

### 4. Use Another Subscription File
Point the tool at a subscription list stored somewhere else:
```powershell
.\v2pingo.exe -sub-file "D:\my-subs.txt"
```

### 5. Verbose Logging
Show failed tests in addition to successful ones:
```powershell
.\v2pingo.exe -v
```

### 6. Publishing Results
By default, the saved results file is uploaded to `paste.rs` once the run finishes and the link is printed:
```text
Saved 42 HTTP 2xx configuration(s) to results\results_20261009_183039.md
Paste link: https://paste.rs/AbCdE
```

Disable the upload, or send it to another paste service that follows the same API:
```powershell
.\v2pingo.exe -paste=false
.\v2pingo.exe -paste-url "https://my-paste.example.com/"
```

> The uploaded file contains working proxy share links (credentials included). Publishing is enabled by default as requested, but the link is public — keep `-paste=false` when the results must stay private.

---

## Subscription List File (`sub-input.txt`)

The list of subscriptions to test lives in a plain-text file in the project root. One URL per line:

```text
# comments and blank lines are ignored
https://raw.githubusercontent.com/barry-far/V2ray-Config/main/All_Configs_Sub.txt
https://github.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt
```

Add or remove lines to change which subscriptions are fetched — every remaining line is downloaded and tested. If the file does not exist next to the working directory, the directory of `v2pingo.exe` is checked as a fallback, and `-sub-file <path>` can point anywhere.

---

## CLI Options

| Flag | Type | Default | Description |
|---|---|---|---|
| `-url` | `string` | `https://x.com` | Target test URL for end-to-end HTTP GET verification. |
| `-c` | `int` | `20` | Concurrency level (number of worker goroutines). |
| `-timeout` | `duration` | `15s` | Timeout for each proxy configuration test. |
| `-default-subs` | `bool` | `true` | Include the subscription URLs listed in the subscription file. |
| `-sub-file` | `string` | `sub-input.txt` | Path to the subscription list file (one URL per line). |
| `-sub` | `string` | *(empty)* | Custom subscription URL(s) (repeatable or comma-separated). |
| `-v`, `-verbose` | `bool` | `false` | Enable verbose output (prints failed tests). |
| `-paste` | `bool` | `true` | Upload the saved results file to the paste service and print the link. |
| `-paste-url` | `string` | `https://paste.rs/` | Paste service endpoint used when `-paste` is enabled. |

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

### Paste Upload
```text
Saved 42 HTTP 2xx configuration(s) to results\results_20261009_183039.md
Paste link: https://paste.rs/AbCdE
```

On the TUI's done screen the same link is shown as a `Paste:` line.

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

## Default Subscription List

The shipped `sub-input.txt` contains these curated public subscriptions (edit the file to add or remove entries):

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
sub-input.txt (one URL per line) + custom -sub URLs
       │
       ▼
 [Fetcher & Normalizer]  ──► Plain / Base64 decode, GitHub blob rewrite
       │
       ▼
 [Parser & Deduplicator] ──► Parse VLESS / VMess / Trojan / SS / SOCKS / Hysteria2 / WireGuard
       │                     & build sing-box outbound (or endpoint) options
       ▼
 [Worker Pool (Fixed N)] ──► Ephemeral in-process Box instance (zero open listening ports)
       │                  ──► HTTP Client over outbound/endpoint DialContext
       ▼
 [Reporter & Sanitizer]  ──► Live progress & final summary
       │
       ▼
 [Paste Publisher]       ──► POST results_*.md to paste.rs, print the short link
```

---

## License

MIT License.
