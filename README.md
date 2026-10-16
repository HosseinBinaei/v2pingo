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
  - Every source is cached in [`cache/`](#subscription-cache-cache): before a run starts you are asked whether to refresh the files from GitHub or reuse the copies downloaded earlier.
  - Automatically rewrites GitHub `/blob/` URLs to `raw.githubusercontent.com`.
  - Decodes raw plain-text links, single/multiline Base64 (standard and URL-safe, padded and unpadded), and mixed subscription payloads.
  - Normalizes and deduplicates duplicate configurations (ignoring remark `#fragments` for uniqueness).
  - Resilient network fetching: failed subscriptions emit warnings without aborting the test batch, and a failed refresh keeps the cached copy of that source.
- **Controlled Concurrency & Timeouts:**
  - Fixed worker pool (default: `20` concurrent workers).
  - Strict per-test context timeouts (default: `15s`) covering handshake, connection setup, and HTTP GET request.
  - Safe resource cleanup (`defer` for box instances, transports, idle connections, and HTTP bodies).
- **Clean & Sanitized Output:**
  - Real-time live status updates without leaking credentials, passwords, UUIDs, or Reality keys.
  - Optional verbose mode (`-v` / `-verbose`) for inspecting failures.
  - Graceful shutdown on `Ctrl+C` (`SIGINT` / `SIGTERM`) that halts workers safely and prints a summary for completed tests.
- **Opt-In Shareable Results:**
  - Once the results file is written, the tool asks whether to upload it to [paste.rs](https://paste.rs) — answering `y` / `yes` uploads it and prints the link, any other answer simply keeps the local file.
  - The question is only asked when a results file with healthy configurations actually exists; empty runs save nothing and are never asked about.
  - If the upload fails, the error is reported and you are asked whether to retry — `y` / `yes` tries again, any other answer gives up and leaves the saved file on disk.
  - Skip the question entirely with `-paste=false`, or point it at another paste service with `-paste-url`.
  - The TUI shows the same questions on its done screen (`y` uploads or retries, any other key skips or gives up).

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

Before anything is fetched the tool asks whether to refresh the subscription files from GitHub or reuse the copies cached in `cache/` by a previous run — see [Subscription Cache](#subscription-cache-cache).

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

### 6. Publishing Results (Opt-In)
Once the results file is saved, `v2pingo` asks before publishing anything:
```text
Saved 42 HTTP 2xx configuration(s) to results\results_20261009_183039.md
Upload the saved results to paste.rs and get a shareable link? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Answer `y`, `Y`, `yes`, `YES` or `Yes` to upload; **anything else** (including just pressing Enter, or running non-interactively) keeps the results local and prints `Skipped paste upload.`

If the upload itself fails, the reason is printed and the tool asks again:
```text
Warning: failed to upload results file: ...
Upload failed. Retry uploading to paste.rs? [y/N]: y
Paste link: https://paste.rs/AbCdE
```
Answering anything other than `y` / `yes` ends the run with the results still saved locally (`Giving up on paste upload; results remain saved locally.`).

Skip the question altogether, or send it to another paste service that follows the same API:
```powershell
.\v2pingo.exe -paste=false
.\v2pingo.exe -paste-url "https://my-paste.example.com/"
```

> The uploaded file contains working proxy share links (credentials included) and the paste link is public, so only answer yes when you intend to share them.

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

## Subscription Cache (`cache/`)

Every subscription source is downloaded once and stored in the `cache/` directory, one file per source URL. Nothing is fetched and no test starts before the cache question is answered, so a run either refreshes the sources from GitHub or reuses the files of an earlier run.

The very first run finds no cache and downloads without asking:

```text
No cached subscription files in cache; fetching fresh copies from the source links.
```

Every later run shows what is available and asks:

```text
Cached subscription files in cache: 13 file(s), newest 2026-10-15 09:12:44.
Fetch fresh copies from GitHub and refresh the cached subscription files? [y/N]:
```

- Answer `y` / `yes`, `Y` or `YES` to download every source again and refresh the cache with the new payloads. Cached copies of sources that are no longer listed in `sub-input.txt` are removed, so after a refresh the directory mirrors the current source list; a source that fails to download keeps its previous file for a later run.
- **Any other answer** — `n`, Enter, or a non-interactive run without an interactive prompt — reuses the cached files and prints `Using the cached subscription files.` Sources that are still missing from the cache are downloaded once and added to it.
- Each cache file is named after its source (`host-path-<hash>.txt`), so a source always maps to the same file and two different sources can never collide.
- The question is skipped when `cache/` is still empty, and `-cache=false` disables the cache entirely: every run downloads fresh copies and nothing is read from or written to disk.
- Store the cache somewhere else with `-cache-dir <path>`. A `cache` directory next to `v2pingo.exe` is also found when the tool is started from another working directory.
- The directory only holds downloaded subscription payloads, it is ignored by Git, and you can delete it at any time to start from scratch.
- In the TUI the question is asked before the form appears, and the form shows where the sources of the run will come from (`sources: cached files in cache` or `sources: fresh download from the subscription URLs`).

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
| `-cache` | `bool` | `true` | Ask whether to reuse the cached subscription files; `false` disables the cache and always downloads fresh copies. |
| `-cache-dir` | `string` | `cache` | Directory that keeps the fetched subscription files. |
| `-v`, `-verbose` | `bool` | `false` | Enable verbose output (prints failed tests). |
| `-paste` | `bool` | `true` | Ask whether to upload the saved results file to the paste service (`false` never asks). |
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

### Paste Upload (Opt-In)
```text
Saved 42 HTTP 2xx configuration(s) to results\results_20261009_183039.md
Upload the saved results to paste.rs and get a shareable link? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Any other answer prints `Skipped paste upload.` and nothing is sent. A failed upload is reported and followed by a retry question: `y` attempts the upload again, any other answer gives up with the results left in the local file. On the TUI's done screen the same questions are answered with a single key (`y` = upload or retry, any other key = skip or give up) and a successful link appears as a `Paste:` line.

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
2. `https://raw.githubusercontent.com/ebrasha/free-v2ray-public-list/refs/heads/main/V2Ray-Config-By-EbraSha.txt`
3. `https://raw.githubusercontent.com/MatinGhanbari/v2ray-configs/main/subscriptions/v2ray/all_sub.txt`
4. `https://raw.githubusercontent.com/0xRadikal/Free-v2ray-Configs/main/all/configs.txt`
5. `https://raw.githubusercontent.com/Au1rxx/free-vpn-subscriptions/main/output/by-country/v2ray-base64-JP.txt`
6. `https://raw.githubusercontent.com/Au1rxx/free-vpn-subscriptions/main/output/by-country/v2ray-base64-NL.txt`
7. `https://raw.githubusercontent.com/Au1rxx/free-vpn-subscriptions/main/output/by-country/v2ray-base64-US.txt`
8. `https://raw.githubusercontent.com/Au1rxx/free-vpn-subscriptions/main/output/by-country/v2ray-base64-DE.txt`
9. `https://raw.githubusercontent.com/Alirewa/V2ray-Configs/main/config.txt`
10. `https://raw.githubusercontent.com/morpheusadam/v2ray-config/main/subs/bundles/best.txt`
11. `https://raw.githubusercontent.com/DukeMehdi/FreeList-V2ray-Configs/refs/heads/main/Configs/All-DukeMehdi-Configs.txt`
12. `https://raw.githubusercontent.com/Mr-Meshky/vify/main/configs/all.txt`
13. `https://raw.githubusercontent.com/HakurouKen/free-node/main/public`

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
 [Source Cache]          ──► Reuse cache/ … or refresh it from GitHub (asked first)
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
 [Paste Publisher]       ──► On "yes": POST results_*.md to paste.rs, print the link
```

---

## License

MIT License.
