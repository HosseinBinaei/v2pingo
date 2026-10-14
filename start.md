# Quick Start & Usage Guide for v2pingo

`v2pingo` is a fast CLI tool for testing V2Ray-compatible proxy configurations directly in memory.

It supports **VLESS**, **VMess**, **Trojan**, **Shadowsocks**, **SOCKS**, **Hysteria2** and **WireGuard** share links.

---

## 1. Build

To build the Windows executable:

```powershell
go build -o v2pingo.exe ./cmd/v2pingo
```

---

## 2. Subscription List File

The subscriptions to test are read from `sub-input.txt` in the project root — one URL per line:

```text
# lines starting with '#' and blank lines are ignored
https://raw.githubusercontent.com/barry-far/V2ray-Config/main/All_Configs_Sub.txt
https://github.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt
```

Add a line to include another subscription, delete a line to drop one. Every remaining line is fetched and tested. Use a different file with:

```powershell
.\v2pingo.exe -sub-file "D:\my-subs.txt"
```

> If `sub-input.txt` is not found in the current working directory, the folder that contains `v2pingo.exe` is checked as a fallback.

---

## 3. Subscription Cache (`cache/`) — asked before every run

The subscription sources are downloaded once and kept in the `cache/` directory, one file per source URL. Before any source is fetched, `v2pingo` asks where the run should take them from:

* **First run** — the cache is empty, so the sources are downloaded and stored without a question:

```text
No cached subscription files in cache; fetching fresh copies from the source links.
```

* **Later runs** — the question is asked before the tests start:

```text
Cached subscription files in cache: 13 file(s), newest 2026-10-15 09:12:44.
Fetch fresh copies from GitHub and refresh the cached subscription files? [y/N]:
```

Answer `y`, `Y`, `yes` or `YES` to download fresh copies from GitHub and refresh the cache with them. **Every other answer** — `n`, Enter, or a run without an interactive prompt — reuses the cached files and prints `Using the cached subscription files.`; a source that is still missing from the cache is downloaded once and added to it.

A refresh replaces the cache: cached copies of sources that were removed from `sub-input.txt` are deleted, so the directory mirrors the current source list. A source that cannot be downloaded keeps its previous cached file so a later run can still use it.

Disable the cache for a run — the question is skipped, every source is downloaded and nothing is written to disk — or keep the cache in another directory:

```powershell
.\v2pingo.exe -cache=false
.\v2pingo.exe -cache-dir "D:\v2pingo-cache"
```

> The cache holds only downloaded subscription payloads; it is ignored by Git and can be deleted at any time. In the TUI the question is asked before the form opens, and the form then shows `sources: cached files in cache` or `sources: fresh download from the subscription URLs`.

---

## 4. Default Test

Run the simplest command to test using the subscription file, the default target URL (`https://x.com`), default concurrency (`20`), and default timeout (`15s`):

```powershell
.\v2pingo.exe
```

---

## 5. Custom Target

Test connectivity to a specific website or domain:

```powershell
.\v2pingo.exe -url https://gemini.google.com
```

> **Note:** `v2pingo` tests the full end-to-end path:
> **Local machine → Proxy config/server → Target URL**

---

## 6. Custom Concurrency & Timeout

Adjust worker count and request timeout for your network conditions:

```powershell
.\v2pingo.exe -url https://gemini.google.com -c 50 -timeout 5s
```

* **Higher Concurrency (`-c 50` or higher):** Speeds up testing large lists on fast connections.
* **Lower Concurrency (`-c 5` to `-c 10`):** Reduces local CPU/network strain or bandwidth congestion.
* **Shorter Timeout (`-timeout 5s`):** Fast-fails unresponsive proxies to quickly identify low-latency options.
* **Longer Timeout (`-timeout 20s`):** Gives slower or high-latency proxies more time to complete handshakes.

---

## 7. Custom Subscriptions

Disable the subscription file and supply your own subscription URLs instead.

**Single custom subscription:**
```powershell
.\v2pingo.exe -default-subs=false -sub "https://example.com/sub.txt"
```

**Multiple custom subscriptions:**
```powershell
.\v2pingo.exe -sub "https://site1.com/sub.txt" -sub "https://site2.com/sub.txt"
```

*(You can also pass comma-separated URLs or combine `-sub` with the subscription file.)*

---

## 8. Verbose Mode

Enable verbose output to view failed proxy tests alongside sanitized error reasons:

```powershell
.\v2pingo.exe -v
```

---

## 9. Results File

Configurations that return an **HTTP 200** status code are automatically saved at the end of execution (or upon pressing Ctrl+C) to a Markdown file:

```text
results/results_YYYYMMDD_HHMMSS.md
```

The file contains the working proxy configurations saved as original share links, one per line.

---

## 10. Shareable Link (paste.rs) — asks first

Once the results file is saved, `v2pingo` **asks** whether you want a shareable link:

```text
Saved 42 HTTP 2xx configuration(s) to results/results_20261009_183039.md
Upload the saved results to paste.rs and get a shareable link? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Answer `y`, `Y`, `yes`, `YES` or `Yes` to upload the file to [paste.rs](https://paste.rs) and get the link. **Any other answer** — `n`, Enter, a random word, or running without an interactive prompt — keeps the results on your machine and prints `Skipped paste upload.`

If the upload fails (network down, paste.rs unreachable, bad response), the failure is reported and you are asked whether to try again:

```text
Warning: failed to upload results file: ...
Upload failed. Retry uploading to paste.rs? [y/N]: y
Paste link: https://paste.rs/AbCdE
```

Answer `y` / `yes` to attempt the upload again; **any other answer** gives up and the process ends with the results still saved locally (`Giving up on paste upload; results remain saved locally.`).

In the TUI the same questions appear on the done screen: press `y` to upload or retry, any other key to skip or give up, and a successful link is then shown as a `Paste:` line.

Details:

* If the run found no HTTP 2xx configuration, no results file is created and the question is never asked.
* Skip the question for a run with `-paste=false`.
* Point the upload at another service that follows the same API with `-paste-url`:

```powershell
.\v2pingo.exe -paste=false
.\v2pingo.exe -paste-url "https://my-paste.example.com/"
```

> **Privacy note:** the uploaded file contains working proxy share links, credentials included, and the paste link is public. Only answer yes when you really want to share them; `-paste=false` never asks.

---

## 11. Stop / Ctrl+C

Pressing **Ctrl+C** during a test run will:

* Cancel ongoing worker tests gracefully
* Preserve results from all completed tests
* Save configurations that received HTTP 200 to the results file
* Ask whether to upload the saved results file to `paste.rs`
* Print a partial test summary

---

## 12. Practical Recommended Examples

**Quick default test:**
```powershell
.\v2pingo.exe
```

**Large batch run with high concurrency:**
```powershell
.\v2pingo.exe -c 50 -timeout 10s
```

**Test proxy access to a specific site:**
```powershell
.\v2pingo.exe -url https://youtube.com -timeout 10s
```

**Test using a custom subscription file:**
```powershell
.\v2pingo.exe -sub-file "D:\my-subs.txt" -c 30
```

**Test exclusively against custom subscriptions:**
```powershell
.\v2pingo.exe -default-subs=false -sub "https://example.com/sub.txt" -c 30
```

**Verbose debugging:**
```powershell
.\v2pingo.exe -v -timeout 5s
```

**Share the results (answer `y` at the prompt) or never ask:**
```powershell
.\v2pingo.exe
.\v2pingo.exe -paste=false
```

**Reuse the cached subscriptions (answer Enter) or always refresh them:**
```powershell
.\v2pingo.exe
.\v2pingo.exe -cache=false
```

---

## 13. Important Notes

* **Internet Access:** `v2pingo` requires an active internet connection to fetch subscription lists and perform target URL checks. Subscription sources are cached in `cache/` and reused whenever the cache question is not answered with `y`/`yes`, so a run from the cache only needs the internet for the target URL checks; `-cache=false` skips the cache entirely and downloads fresh copies every time.
* **Target-Dependent Results:** A proxy may succeed for one target URL (e.g., `https://google.com`) but fail or time out for another.
* **HTTP 200 Verification:** An **HTTP 200** response confirms the proxy successfully connected, proxied the traffic, and received a standard successful HTTP response from the target server.
* **HTTP 403 / 404 / Non-200 Status:** Status codes like 403 or 404 mean the proxy connection itself succeeded, but the target server or cloudflare/CDN returned an error response.
* **Real HTTP Requests:** `v2pingo` does not perform a simple ping to the proxy IP. It executes a real HTTP request routed through the in-process proxy core.
* **Protocol Support:** VLESS, VMess, Trojan, Shadowsocks, SOCKS5, Hysteria2 and WireGuard. WireGuard nodes are tested through a sing-box endpoint, and Hysteria2 nodes always use TLS.
* **Credential Safety:** Result files contain sensitive or private proxy credentials. `v2pingo` only publishes them to `paste.rs` when you answer `y`/`yes` at the prompt, and that link is public — pass `-paste=false` when the results must never leave your machine.
