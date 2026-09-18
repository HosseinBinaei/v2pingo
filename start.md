# Quick Start & Usage Guide for v2pingo

`v2pingo` is a fast CLI tool for testing V2Ray-compatible proxy configurations directly in memory.

---

## 1. Build

To build the Windows executable:

```powershell
go build -o v2pingo.exe ./cmd/v2pingo
```

---

## 2. Default Test

Run the simplest command to test using built-in subscriptions, the default target URL (`https://x.com`), default concurrency (`20`), and default timeout (`15s`):

```powershell
.\v2pingo.exe
```

---

## 3. Custom Target

Test connectivity to a specific website or domain:

```powershell
.\v2pingo.exe -url https://gemini.google.com
```

> **Note:** `v2pingo` tests the full end-to-end path:
> **Local machine → Proxy config/server → Target URL**

---

## 4. Custom Concurrency & Timeout

Adjust worker count and request timeout for your network conditions:

```powershell
.\v2pingo.exe -url https://gemini.google.com -c 50 -timeout 5s
```

* **Higher Concurrency (`-c 50` or higher):** Speeds up testing large lists on fast connections.
* **Lower Concurrency (`-c 5` to `-c 10`):** Reduces local CPU/network strain or bandwidth congestion.
* **Shorter Timeout (`-timeout 5s`):** Fast-fails unresponsive proxies to quickly identify low-latency options.
* **Longer Timeout (`-timeout 20s`):** Gives slower or high-latency proxies more time to complete handshakes.

---

## 5. Custom Subscriptions

Disable built-in subscriptions or supply your own custom subscription URLs.

**Single custom subscription (disabling default subscriptions):**
```powershell
.\v2pingo.exe -default-subs=false -sub "https://example.com/sub.txt"
```

**Multiple custom subscriptions:**
```powershell
.\v2pingo.exe -sub "https://site1.com/sub.txt" -sub "https://site2.com/sub.txt"
```

*(You can also pass comma-separated URLs or combine `-sub` with default subscriptions.)*

---

## 6. Verbose Mode

Enable verbose output to view failed proxy tests alongside sanitized error reasons:

```powershell
.\v2pingo.exe -v
```

---

## 7. Results File

Configurations that return an **HTTP 200** status code are automatically saved at the end of execution (or upon pressing Ctrl+C) to a Markdown file:

```text
results_YYYYMMDD_HHMMSS.md
```

The file contains the working proxy configurations saved as original share links, one per line.

---

## 8. Stop / Ctrl+C

Pressing **Ctrl+C** during a test run will:

* Cancel ongoing worker tests gracefully
* Preserve results from all completed tests
* Save configurations that received HTTP 200 to the results file
* Print a partial test summary

---

## 9. Practical Recommended Examples

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

**Test exclusively against custom subscriptions:**
```powershell
.\v2pingo.exe -default-subs=false -sub "https://example.com/sub.txt" -c 30
```

**Verbose debugging:**
```powershell
.\v2pingo.exe -v -timeout 5s
```

---

## 10. Important Notes

* **Internet Access:** `v2pingo` requires an active internet connection to fetch subscription lists and perform target URL checks.
* **Target-Dependent Results:** A proxy may succeed for one target URL (e.g., `https://google.com`) but fail or time out for another.
* **HTTP 200 Verification:** An **HTTP 200** response confirms the proxy successfully connected, proxied the traffic, and received a standard successful HTTP response from the target server.
* **HTTP 403 / 404 / Non-200 Status:** Status codes like 403 or 404 mean the proxy connection itself succeeded, but the target server or cloudflare/CDN returned an error response.
* **Real HTTP Requests:** `v2pingo` does not perform a simple ping to the proxy IP. It executes a real HTTP request routed through the in-process proxy core.
* **Credential Safety:** Do not expose or publicly share result files containing sensitive or private proxy credentials.
