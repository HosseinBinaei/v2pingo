# v2pingo

**English** · [فارسی](README.fa.md)

A fast, lightweight and robust CLI tool that tests V2Ray-compatible proxy configurations
extracted from subscriptions, using the **sing-box core in-process**.

Written in Go, it tests every configuration directly in memory — without binding any local
TCP listening port — and measures the true end-to-end HTTP round-trip latency with strict
concurrency and timeout controls.

---

## Table of contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation & Build](#installation--build)
  - [Windows](#windows)
  - [Linux](#linux)
  - [macOS](#macos)
  - [Cross-compiling (build for any OS)](#cross-compiling-build-for-any-os)
  - [Build from source (any OS)](#build-from-source-any-os)
- [Quick Start & Usage](#quick-start--usage)
- [How a test works (two-phase)](#how-a-test-works-two-phase)
- [Subscription List File (`sub-input.txt`)](#subscription-list-file-sub-inputtxt)
- [Subscription Cache (`cache/`)](#subscription-cache-cache)
- [CLI Options](#cli-options)
- [Choosing Workers & Timeout (Tuning)](#choosing-workers--timeout-tuning)
- [Terminal UI (TUI)](#terminal-ui-tui)
- [Batch Size, Batched Runs & Resume](#batch-size-batched-runs--resume)
- [Output Examples](#output-examples)
- [Running Tests](#running-tests)
- [Architecture Overview](#architecture-overview)
- [Privacy & Security Notes](#privacy--security-notes)
- [Contributing](#contributing)
- [Disclaimer](#disclaimer)
- [License](#license)

---

## Features

- **In-process sing-box core:** direct in-memory dialing (`DialContext`) with **zero local
  proxy listening ports** (`127.0.0.1:XXXX`), so there is no local port exhaustion and no
  firewall interference.
- **Two-phase testing:** a cheap raw **TCP precheck** eliminates dead nodes first, and only
  reachable nodes pay the full end-to-end HTTP cost.
- **Multiple protocol support:**
  - **VLESS:** TCP, WebSocket, gRPC, TLS, Reality (`pbk`, `sid`, `fp`), and Vision flow
    (`xtls-rprx-vision`).
  - **VMess:** standard v2rayN JSON format, TCP, WebSocket, gRPC and TLS.
  - **Trojan:** TLS, WebSocket and gRPC transports.
  - **Shadowsocks:** standard SIP002 (plain / base64 userinfo) and legacy base64 URI formats.
  - **SOCKS:** `socks://` and `socks5://` links with plain, base64 and legacy
    fully-encoded userinfo. Bare `ip:port` entries (no scheme, IPv4 or bracketed
    IPv6) carry no protocol information and are treated as unauthenticated
    SOCKS5 proxies.
  - **Hysteria2:** `hysteria2://` and `hy2://` links with URI parameters (`sni`, `insecure`,
    `obfs`/`obfs-password`, `pinSHA256`, `up`/`down`) and port hopping (`mport`, multi-port
    authority).
  - **WireGuard:** `wireguard://` and `wg://` links (private key, `publickey`, `presharedkey`,
    `address`, `mtu`, `reserved`). WireGuard is tested through a sing-box **endpoint**, the
    only form sing-box still supports since the legacy outbound was removed.
- **Flexible subscription handling:**
  - Subscription URLs are read from [`sub-input.txt`](sub-input.txt) in the project root, one
    URL per line, so the list can be edited without rebuilding.
  - Every source is cached in [`cache/`](#subscription-cache-cache); before a run starts you
    are asked whether to refresh the files from the network or reuse the copies downloaded
    earlier.
  - Automatically rewrites GitHub `/blob/` URLs to `raw.githubusercontent.com`.
  - Decodes raw plain-text links, single/multiline Base64 (standard and URL-safe, padded and
    unpadded) and mixed subscription payloads.
  - Normalizes and deduplicates configurations (ignoring remark `#fragments` for uniqueness).
  - Resilient fetching: a failed source emits a warning without aborting the run, and a failed
    refresh keeps the cached copy of that source.
- **Controlled concurrency & timeouts:**
  - Fixed worker pool (default: `20` concurrent workers).
  - Strict per-test context timeouts (default: `15s`) covering handshake, connection setup and
    the HTTP GET request.
  - Safe resource cleanup (`defer` for box instances, transports, idle connections and HTTP
    bodies).
- **Clean, sanitized output:**
  - Real-time live status updates that never leak credentials, passwords, UUIDs or Reality keys.
  - Optional verbose mode (`-v` / `-verbose`) for inspecting failures.
  - Graceful shutdown on `Ctrl+C` (`SIGINT` / `SIGTERM`) that halts workers safely and prints a
    summary for completed tests.
- **No elevated privileges required:** the tool only builds outbound dialers — it never creates
  a TUN interface or touches system routing — so it runs as a normal user with no admin/root.
- **Re-importable results:** saved entries are the original share links, so the results file can
  be fed straight back into a v2rayN / sing-box client.
- **Hardened fetching:** subscription downloads send a `v2rayN/6.0` User-Agent, which avoids some
  host-based blocks.
- **Bounded response read:** each test reads only the first ~4 KB of the response body, so the
  reported latency reflects the handshake plus time-to-first-bytes rather than a full page
  download.
- **Opt-in shareable results:**
  - Once the results file is written, the tool asks whether to upload it to
    [paste.rs](https://paste.rs) — answering `y` / `yes` uploads it and prints the link, any
    other answer simply keeps the local file.
  - Automatic fallback: if paste.rs fails (HTTP error, timeout, DNS/TLS or any connectivity
    problem), the same file is tried against [paste.c-net.org](https://paste.c-net.org/) and
    then [dpaste.org](https://dpaste.org/), and the first successful link is printed.
  - The question is asked only when a results file with healthy configurations actually exists;
    empty runs save nothing and are never asked about.
  - If the upload fails, the error is reported and you are asked whether to retry.
  - Skip the question entirely with `-paste=false`, or point it at another paste service with
    `-paste-url` (a custom endpoint is used alone, without the built-in fallbacks).
  - The TUI shows the same questions on its done screen.

---

## Requirements

- **[Go 1.27+](https://go.dev/dl/)** installed (`go.mod` requires `go 1.27.0`).
- Internet access to fetch subscription lists and to reach the target test URL.
- Any of Windows, Linux or macOS. Windows is the primary tested platform, but the code is plain
  Go and builds and runs on Linux and macOS too.

---

## Installation & Build

Prebuilt binaries are **not** committed to the repository (`*.exe` is git-ignored). Build the
executable locally with the commands below.

> Replace the clone URL with your own fork if you are working on a copy.

### Windows

1. Install Go from [go.dev/dl](https://go.dev/dl/) and open a new **PowerShell** window
   (`go version` should print `go1.27` or newer).
2. Clone and build:

```powershell
git clone https://github.com/HosseinBinaei/v2pingo.git
cd v2pingo
go build -o v2pingo.exe ./cmd/v2pingo
```

3. Run it:

```powershell
.\v2pingo.exe
```

> If PowerShell blocks the build or run, allow local scripts for the current session with
> `Set-ExecutionPolicy -Scope Process Bypass`.

### Linux

1. Install Go (either the distro package or the official tarball):

```bash
# Debian / Ubuntu
sudo apt update && sudo apt install -y golang-go

# Fedora / RHEL
sudo dnf install -y golang
```

If your distro ships an older Go than 1.27, install the official tarball instead:

```bash
curl -LO https://go.dev/dl/go1.27.0.linux-amd64.tar.gz   # use linux-arm64 on ARM
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.27.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

2. Clone and build:

```bash
git clone https://github.com/HosseinBinaei/v2pingo.git
cd v2pingo
go build -o v2pingo ./cmd/v2pingo
```

3. Run it:

```bash
./v2pingo
```

### macOS

1. Install Go with [Homebrew](https://brew.sh) (or the official installer from
   [go.dev/dl](https://go.dev/dl/)):

```bash
brew install go
```

2. Clone and build:

```bash
git clone https://github.com/HosseinBinaei/v2pingo.git
cd v2pingo
go build -o v2pingo ./cmd/v2pingo
```

3. Run it:

```bash
./v2pingo
```

> On Apple Silicon (M-series) the build is native `arm64` automatically; Intel Macs build
> `amd64`.

### Cross-compiling (build for any OS)

Go cross-compiles with no extra toolchain, so from any single machine you can produce binaries
for every target. Set `GOOS` (operating system) and `GOARCH` (CPU architecture):

| Target | `GOOS` | `GOARCH` | Output |
|---|---|---|---|
| Windows 64-bit | `windows` | `amd64` | `v2pingo.exe` |
| Windows ARM64 | `windows` | `arm64` | `v2pingo.exe` |
| Linux x86-64 | `linux` | `amd64` | `v2pingo` |
| Linux ARM64 (Raspberry Pi / ARM server) | `linux` | `arm64` | `v2pingo` |
| macOS Apple Silicon | `darwin` | `arm64` | `v2pingo` |
| macOS Intel | `darwin` | `amd64` | `v2pingo` |

Bash / macOS / Linux:

```bash
# Windows executable from Linux/macOS
GOOS=windows GOARCH=amd64 go build -o v2pingo.exe ./cmd/v2pingo

# Linux ARM64 (e.g. Raspberry Pi / ARM server)
GOOS=linux GOARCH=arm64 go build -o v2pingo-arm64 ./cmd/v2pingo

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o v2pingo-darwin-arm64 ./cmd/v2pingo
```

PowerShell (Windows):

```powershell
# Linux executable from Windows
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o v2pingo ./cmd/v2pingo

# Build every desktop target at once
$env:CGO_ENABLED="0"
foreach ($t in @(@("windows","amd64"),@("linux","amd64"),@("linux","arm64"),@("darwin","arm64"))) {
  $env:GOOS=$t[0]; $env:GOARCH=$t[1]
  $ext = if ($t[0] -eq "windows") { ".exe" } else { "" }
  go build -o "v2pingo-$($t[0])-$($t[1])$ext" ./cmd/v2pingo
}
```

### Build from source (any OS)

Use `go install` to place the binary in your `GOBIN` (`$HOME/go/bin` by default):

```bash
go install ./cmd/v2pingo
```

---

## Quick Start & Usage

Run the tool from the directory that contains `sub-input.txt`, so the subscription list and the
`cache/` and `results/` folders are created next to it.

### 1. Default run

Reads the subscription URLs listed in `sub-input.txt`, extracts all supported configurations,
deduplicates them, and tests against `https://cloudflare.com` with 20 workers:

```bash
# Linux / macOS
./v2pingo

# Windows
.\v2pingo.exe
```

Before anything is fetched the tool asks whether to refresh the subscription files or reuse the
copies cached in `cache/` — see [Subscription Cache](#subscription-cache-cache).

### 2. Custom target URL & concurrency

Test against Google with 30 concurrent workers and a 10-second timeout:

```bash
./v2pingo -url https://www.google.com/generate_204 -c 30 -timeout 10s
```

### 3. Custom subscriptions

Disable the subscription file and test only your own subscription URLs:

```bash
./v2pingo -default-subs=false -sub "https://example.com/mysub.txt"
```

Pass multiple custom subscriptions either by repeating `-sub` or using comma-separated values:

```bash
./v2pingo -sub "https://site1.com/sub.txt" -sub "https://site2.com/sub.txt"
./v2pingo -sub "https://site1.com/sub.txt,https://site2.com/sub.txt"
```

### 4. Use another subscription file

Point the tool at a subscription list stored somewhere else:

```bash
./v2pingo -sub-file "/path/to/my-subs.txt"
```

### 5. Verbose logging

Show failed tests in addition to successful ones:

```bash
./v2pingo -v
```

### 6. Publishing results (opt-in)

Once the results file is saved, `v2pingo` asks before publishing anything:

```text
Saved 42 HTTP 2xx configuration(s) to results/results_20261009_183039.md
Upload the saved results to paste.rs and get a shareable link? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Answer `y`, `Y`, `yes`, `YES` or `Yes` to upload; **anything else** (including pressing Enter,
or running non-interactively) keeps the results local and prints `Skipped paste upload.`

If the upload itself fails, the reason is printed and the tool asks again:

```text
Warning: failed to upload results file: ...
Upload failed. Retry uploading to paste.rs? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Answering anything other than `y` / `yes` ends the run with the results still saved locally
(`Giving up on paste upload; results remain saved locally.`).

#### Automatic fallback services

The upload does not depend on a single service. paste.rs is tried first; if it fails with any
HTTP error, timeout, DNS/TLS problem or connectivity issue, the same file is automatically
tried against the next service, in order:

1. [paste.rs](https://paste.rs/) — raw POST body, the link comes back in the response.
2. [paste.c-net.org](https://paste.c-net.org/) — same raw POST body style.
3. [dpaste.org](https://dpaste.org/) — form API (`content` field, plain text).

The printed `Paste link:` may therefore come from any of the three. If every service fails,
the error message lists each endpoint that was tried, and you can retry or give up as usual.

Skip the question altogether, or send it to another paste service that follows the same API
(a custom `-paste-url` endpoint is used alone, without the built-in fallbacks):

```bash
./v2pingo -paste=false
./v2pingo -paste-url "https://my-paste.example.com/"
```

> The uploaded file contains working proxy share links (credentials included) and the paste
> link is public, so only answer yes when you intend to share them.

---

## How a test works (two-phase)

Each configuration is tested in two phases to avoid paying the full sing-box setup cost for
every dead node:

1. **Phase 1 — TCP precheck.** A cheap raw TCP dial to the proxy's own `host:port` (capped at
   2 seconds, or the test timeout, whichever is smaller). Nodes that cannot even accept a TCP
   connection fail fast here, before any sing-box instance or HTTP request is created.
   UDP-based transports (**WireGuard**, **Hysteria2**) never complete a TCP handshake and are
   skipped straight to phase 2.
2. **Phase 2 — end-to-end HTTP.** An ephemeral in-process sing-box `Box` is created with a
   single outbound (or WireGuard endpoint), an `http.Client` is routed through that
   outbound's `DialContext`, and a real `GET` request is sent to the target URL. The measured
   latency is the true HTTP round-trip time.

A configuration is only saved when it returns an HTTP **2xx** status code.

---

## Subscription List File (`sub-input.txt`)

The list of subscriptions to test lives in a plain-text file in the project root. One URL per
line:

```text
# comments and blank lines are ignored
https://raw.githubusercontent.com/barry-far/V2ray-Config/main/All_Configs_Sub.txt
https://raw.githubusercontent.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt
```

Add or remove lines to change which subscriptions are fetched — every remaining line is
downloaded and tested. If the file does not exist next to the working directory, the directory
of the `v2pingo` executable is checked as a fallback, and `-sub-file <path>` can point anywhere.

---

## Subscription Cache (`cache/`)

Every subscription source is downloaded once and stored in the `cache/` directory, one file per
source URL. Nothing is fetched and no test starts before the cache question is answered, so a
run either refreshes the sources or reuses the files of an earlier run.

The very first run finds no cache and downloads without asking:

```text
No cached subscriptions; downloading fresh copies.
```

Every later run shows what is available and asks:

```text
Cache: 13 file(s), newest 2026-10-15 09:12:44.
Refresh subscriptions from GitHub? [y/N]:
```

- Answer `y` / `yes` to download every source again and refresh the cache with the new payloads.
  Cached copies of sources that are no longer listed in `sub-input.txt` are removed, so after a
  refresh the directory mirrors the current source list; a source that fails to download keeps
  its previous file for a later run.
- **Any other answer** — `n`, Enter, or a non-interactive run without an interactive prompt —
  reuses the cached files. Sources that are still missing from the cache are downloaded once
  and added to it.
- Each cache file is named after its source (`host-path-<hash>.txt`), so a source always maps to
  the same file and two different sources can never collide.
- The question is skipped when `cache/` is still empty, and `-cache=false` disables the cache
  entirely: every run downloads fresh copies and nothing is read from or written to disk.
- Store the cache somewhere else with `-cache-dir <path>`. A `cache` directory next to the
  executable is also found when the tool is started from another working directory.
- The directory only holds downloaded subscription payloads, it is ignored by Git, and you can
  delete it at any time to start from scratch.
- In the TUI the question is asked before the form appears, and the form shows where the sources
  of the run will come from (`sources: cached files in cache` or
  `sources: fresh download from the subscription URLs`).

---

## CLI Options

| Flag | Type | Default | Description |
|---|---|---|---|
| `-url` | `string` | `https://cloudflare.com` | Target test URL for end-to-end HTTP GET verification. |
| `-c` | `int` | `20` | Concurrency level (number of worker goroutines). |
| `-timeout` | `duration` | `15s` | Timeout for each proxy configuration test. |
| `-batch` | `int` | `5000` | Configs parsed/tested per batch (bounds RAM; results are appended incrementally). |
| `-tui` | `bool` | `true` | Launch interactive terminal UI when stdout is a TTY; use `-tui=false` to force plain CLI mode. |
| `-default-subs` | `bool` | `true` | Include the subscription URLs listed in the subscription file. |
| `-sub-file` | `string` | `sub-input.txt` | Path to the subscription list file (one URL per line). |
| `-sub` | `string` | *(empty)* | Custom subscription URL(s) (repeatable or comma-separated). |
| `-cache` | `bool` | `true` | Ask whether to reuse the cached subscription files; `false` disables the cache and always downloads fresh copies. |
| `-cache-dir` | `string` | `cache` | Directory that keeps the fetched subscription files. |
| `-types` | `string` | *(empty)* | Comma-separated config types to test (`vmess, vless, trojan, ss, socks, hysteria2, wireguard`; aliases `ss/socks5/hy2/wg` accepted). Empty tests everything and asks on the TUI type-filter screen; a value skips that screen and filters right after the fetch. |
| `-v`, `-verbose` | `bool` | `false` | Enable verbose output (prints failed tests). |
| `-paste` | `bool` | `true` | Ask whether to upload the saved results file to the paste service (`false` never asks). |
| `-paste-url` | `string` | `https://paste.rs/` | Paste service endpoint used when `-paste` is enabled. The default gets automatic fallbacks (paste.c-net.org, then dpaste.org); a custom endpoint is used alone. |

---

## Choosing Workers & Timeout (Tuning)

`-c` sets how many configurations are tested at once and `-timeout` how long a single test may
take. Bigger is not better — both values trade accuracy for speed.

> **Important — do not push concurrency too high.** Although `-c` accepts up to 200 (and the
> default is `20`), testing **more than ~10 configurations concurrently rarely improves results
> and usually makes them worse**. Most ISPs/operators — and many home or office routers —
> rate-limit or throttle a burst of simultaneous TLS/QUIC handshakes to many different servers,
> so extra connections start timing out and configurations that are actually fine get recorded
> as failures. **Keep `-c` at or below about 10 for trustworthy results.** Only raise it toward
> the default `20` on a fast, unthrottled connection where you care more about speed than
> precision (e.g. a second pass over the already-filtered healthy nodes).

> **Tune for your own operator & system.** The best worker count and timeout are not the same
> on every network — your operator's throttling behavior, your machine's CPU/RAM, and your
> local network conditions all matter. **Test on your own setup before relying on the numbers
> below:** start from a conservative value (e.g. `-c 5`), run a small batch, and raise `-c`
> gradually while watching whether successes drop or false failures increase. Too many workers
> can sometimes produce *worse* results (more timeouts, more false failures), so the best value
> for you is the highest concurrency that still yields stable, accurate results.

---

## Terminal UI (TUI)

By default `v2pingo` starts an interactive [Bubble Tea](https://github.com/charmbracelet/bubbletea)
UI when stdout is a terminal (`-tui=true`):

- The cache question is asked first in the terminal, then a form opens showing where the sources
  come from (`sources: cached files in cache` or `sources: fresh download from the subscription URLs`).
- Fill in target URL, concurrency (1–200), timeout, subscriptions, then run. Live progress, the
  summary, the results-file path and the optional paste upload are all shown on the done screen
  (`y` = upload/retry, any other key = skip/give up).
- After Enter, the sources are fetched and a **Type Filter** screen lists every config type
  actually present with its count (all pre-checked). Toggle types with `space`, flip all with
  `a`, and press `Enter` to test only the selected types — unselected types are never parsed
  or tested. Passing `-types` on the command line skips this screen and applies the filter
  directly.

Force non-interactive CLI mode (CI, scripts, piping) with:

```bash
./v2pingo -tui=false
./v2pingo -tui=false -paste=false -cache=false -c 20
```

---

## Batch Size, Batched Runs & Resume

### What is a batch?

A **batch** is a fixed-size slice of the extracted configurations. `v2pingo` never loads a whole
subscription into memory at once: it parses at most one batch of configs, tests them, appends
that batch's healthy links to the results file, records a checkpoint, frees the batch, and moves
on. The batch size defaults to **`5000`** and is controlled by `-batch`.

Change it with `-batch <N>`:

```bash
./v2pingo -batch 1000        # smaller batches, lower peak RAM
./v2pingo -batch 50000       # larger batches, fewer writes, more RAM
```

- **Smaller batches** (`-batch 500`–`1000`): lower peak memory and more frequent incremental
  saves, at the cost of more disk writes and checkpoint updates.
- **Larger batches** (`-batch 20000`+): fewer saves and slightly faster overall, but a higher
  memory ceiling — on a very large list this can reach hundreds of MB.
- Batch size affects **RAM and save/checkpoint granularity only** — it does not change how each
  individual test behaves or how fast a single test runs.
- The batch size is a CLI flag; in the TUI the default (`5000`) is always used. Use `-tui=false`
  with `-batch` when you want to change it.

### How batching, resume and cleanup work

- Only one batch of parsed configs lives in memory at a time.
- Each batch's healthy links are appended incrementally to
  `results/results_YYYYMMDD_HHMMSS.md`.
- A sidecar checkpoint file (`results_...md.checkpoint`) records completed batches so an
  interrupted run (`Ctrl+C`) can resume without re-testing finished batches.
- `cache/`, `results/` and checkpoint files are git-ignored and safe to delete at any time.

---

## Output Examples

### Live testing output

```text
[✓ SUCCESS] [#14] [vless] 104.21.58.12:443 - 182ms (HTTP 200)
[✓ SUCCESS] [#28] [trojan] sg.proxy-node.net:443 - 245ms (HTTP 200)
[✓ SUCCESS] [#35] [vmess] us-east.server.org:80 - 310ms (HTTP 204)
```

With `-v` enabled, failures are displayed with sanitized reasons:

```text
[✗ FAILED] [#15] [vless] 198.51.100.2:443 - precheck: context deadline exceeded
```

### Final summary

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

### Paste upload (opt-in)

```text
Saved 42 HTTP 2xx configuration(s) to results/results_20261009_183039.md
Upload the saved results to paste.rs and get a shareable link? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Any other answer prints `Skipped paste upload.` and nothing is sent. A failed upload is reported
and followed by a retry question. Uploads fall back automatically from paste.rs to
paste.c-net.org and then dpaste.org when the primary service fails. On the TUI's done screen
the same questions are answered with a single key and a successful link appears as a `Paste:`
line.

---

## Running Tests

Run the full Go unit and integration test suite:

```bash
go test ./...
```

Parser/subscription/reporter/paste unit tests run in a few seconds. The `cmd/v2pingo`
integration tests build a temporary binary and spin up local mock servers (including a minimal
SOCKS5 server), so give them a generous timeout on first run:

```bash
go test -timeout 120s ./cmd/v2pingo/ -v
go vet ./...
```

---

## Architecture Overview

```text
sub-input.txt (one URL per line) + custom -sub URLs
       │
       ▼
 [Source Cache]          ──► Reuse cache/ … or refresh it (asked first)
       │
       ▼
 [Fetcher & Normalizer]  ──► Plain / Base64 decode, GitHub blob rewrite
       │
       ▼
 [Parser & Deduplicator] ──► Parse VLESS / VMess / Trojan / SS / SOCKS / Hysteria2 / WireGuard
       │                     & build sing-box outbound (or endpoint) options
       ▼
 [Two-Phase Worker Pool] ──► Phase 1: raw TCP precheck (fail fast, skip for UDP protocols)
       │                  ──► Phase 2: ephemeral in-process Box + HTTP over DialContext
       │                     (zero open listening ports)
       ▼
 [Reporter & Sanitizer]  ──► Live progress & final summary
       │
       ▼
 [Paste Publisher]       ──► On "yes": POST results_*.md to paste.rs
       │                     (fallbacks: paste.c-net.org, dpaste.org), print the link
```

---

## Privacy & Security Notes

- Test only subscriptions you trust. Subscription payloads are third-party proxy lists.
- `results/*.md` files contain working proxy share links **with credentials included**. They are
  git-ignored and stay local unless you explicitly answer `y` to the paste.rs upload prompt.
- The paste link is public. Only upload when you intend to share those credentials.
- Logs never print passwords, UUIDs or Reality keys; live output shows only `host:port`,
  protocol and latency.
- Use `-paste=false` in scripts or on shared machines to guarantee nothing leaves the machine.

---

## Contributing

Issues and pull requests are welcome:

1. Fork the repo and create a feature branch.
2. Run `go vet ./...` and `go test -timeout 120s ./...` (integration tests need the longer
   timeout).
3. Keep `sub-input.txt` edits minimal and explain why a source was added or removed.
4. Do not commit `v2pingo.exe`, `cache/`, `results/`, `type-of-results/` or local subscription
   payloads — they are git-ignored.

---

## Disclaimer

This tool is for network measurement and debugging of proxies you are allowed to use. The
shipped `sub-input.txt` points to public third-party lists of unknown provenance and
availability; there is no guarantee any of them works, is stable or is safe. Users are
responsible for complying with local laws and the terms of the networks they test.

---

## License

MIT License — see [LICENSE](LICENSE).
