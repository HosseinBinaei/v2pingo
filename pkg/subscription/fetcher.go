package subscription

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// DefaultSubscriptionFile is the project-root file that lists the subscription
// URLs to test, one URL per line.
const DefaultSubscriptionFile = "sub-input.txt"

// LoadSubscriptionFile reads subscription URLs from a plain-text file, one URL
// per line. Blank lines and lines starting with '#' are ignored and duplicate
// URLs are removed while their original order is preserved.
func LoadSubscriptionFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read subscription file %s: %w", path, err)
	}

	var urls []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read subscription file %s: %w", path, err)
	}

	return urls, nil
}

// ResolveSubscriptionFile returns the path of the subscription list file. The
// provided path is used when it exists, otherwise the directory that holds the
// running executable is checked so the tool also works when it is started from
// another working directory.
func ResolveSubscriptionFile(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	executable, err := os.Executable()
	if err != nil {
		return path
	}
	candidate := filepath.Join(filepath.Dir(executable), filepath.Base(path))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return path
}

// NormalizeURL trims spaces and rewrites GitHub /blob/ URLs into raw.githubusercontent.com URLs.
func NormalizeURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}

	host := strings.ToLower(u.Host)
	if host == "github.com" || host == "www.github.com" {
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) >= 4 && parts[2] == "blob" {
			newPath := "/" + parts[0] + "/" + parts[1] + "/" + parts[3]
			if len(parts) > 4 {
				newPath += "/" + strings.Join(parts[4:], "/")
			}
			u.Scheme = "https"
			u.Host = "raw.githubusercontent.com"
			u.Path = newPath
			return u.String()
		}
	}

	return trimmed
}

// ExtractProxyLinks parses text line-by-line and returns supported V2Ray/Sing-box share links.
func ExtractProxyLinks(text string) []string {
	var links []string
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if isProxyLink(line) {
			links = append(links, line)
		}
	}
	return links
}

// DecodePayload decodes plain-text or base64-encoded subscription payloads and extracts proxy links.
func DecodePayload(content []byte) []string {
	var results []string
	seen := make(map[string]bool)

	addLinks := func(links []string) {
		for _, l := range links {
			l = strings.TrimSpace(l)
			if l != "" && !seen[l] {
				seen[l] = true
				results = append(results, l)
			}
		}
	}

	// 1. Direct plain text extraction
	directLinks := ExtractProxyLinks(string(content))

	// 2. Try whole payload base64 decode (stripping whitespace)
	cleanContent := stripWhitespace(content)
	var b64Links []string
	if len(cleanContent) > 0 {
		if decoded, ok := tryBase64Decode(cleanContent); ok {
			b64Links = ExtractProxyLinks(string(decoded))
		}
	}

	// If whole-payload base64 decoding yielded proxy links, combine with direct links and return
	if len(b64Links) > 0 {
		addLinks(b64Links)
		addLinks(directLinks)
		return results
	}

	// 3. Otherwise add direct links and try line-by-line base64 decode for mixed/chunked lines
	addLinks(directLinks)

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || isProxyLink(line) {
			continue
		}
		if decoded, ok := tryBase64Decode([]byte(line)); ok {
			lineLinks := ExtractProxyLinks(string(decoded))
			addLinks(lineLinks)
		}
	}

	return results
}

// FetchSubscriptions fetches multiple subscription URLs sequentially and
// extracts their proxy share links. Sources that fail are reported as warnings
// without aborting the run.
//
// dir is the directory that keeps a copy of every downloaded payload so a later
// run can reuse it: with FetchCache the cached payloads are used and only
// sources missing from the cache are downloaded, while with FetchNetwork every
// source is downloaded and stale cache entries are pruned so the directory
// mirrors the current source list. An empty dir disables caching entirely —
// nothing is read from or written to disk.
func FetchSubscriptions(ctx context.Context, urls []string, client *http.Client, dir string, mode FetchMode) []string {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	var allLinks []string
	keep := make(map[string]bool)
	for _, rawURL := range urls {
		select {
		case <-ctx.Done():
			return allLinks
		default:
		}

		normURL := NormalizeURL(rawURL)
		if normURL == "" {
			continue
		}

		if dir != "" {
			keep[CacheFileName(normURL)] = true
			if mode == FetchCache {
				if payload, ok := readCachedSubscription(dir, normURL); ok {
					allLinks = append(allLinks, DecodePayload(payload)...)
					continue
				}
				log.Printf("warn: %s is missing from the cache, downloading it", normURL)
			}
		}

		body, err := fetchPayload(ctx, client, normURL)
		if err != nil {
			log.Printf("warn: %v", err)
			continue
		}
		if dir != "" {
			if err := writeCachedSubscription(dir, normURL, body); err != nil {
				log.Printf("warn: %v", err)
			}
		}
		allLinks = append(allLinks, DecodePayload(body)...)
	}

	if dir != "" && mode == FetchNetwork {
		if err := pruneCache(dir, keep); err != nil {
			log.Printf("warn: %v", err)
		}
	}

	return allLinks
}

// fetchPayload downloads one normalized subscription URL and returns its body.
// A transport error, a non-2xx status and a failed body read are all reported as
// an error so callers can warn and continue with the remaining sources.
func fetchPayload(ctx context.Context, client *http.Client, normURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", normURL, err)
	}
	req.Header.Set("User-Agent", "v2rayN/6.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subscription %s: %w", normURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription %s returned status %d", normURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for %s: %w", normURL, err)
	}

	return body, nil
}

func isProxyLink(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "vless://") ||
		strings.HasPrefix(lower, "vmess://") ||
		strings.HasPrefix(lower, "trojan://") ||
		strings.HasPrefix(lower, "ss://") ||
		strings.HasPrefix(lower, "socks://") ||
		strings.HasPrefix(lower, "socks5://") ||
		strings.HasPrefix(lower, "hysteria2://") ||
		strings.HasPrefix(lower, "hy2://") ||
		strings.HasPrefix(lower, "wireguard://") ||
		strings.HasPrefix(lower, "wg://")
}

func stripWhitespace(b []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, b)
}

func tryBase64Decode(data []byte) ([]byte, bool) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}

	for _, enc := range encodings {
		decoded, err := enc.DecodeString(string(data))
		if err == nil && len(decoded) > 0 {
			return decoded, true
		}
	}
	return nil, false
}
