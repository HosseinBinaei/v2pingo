package subscription

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// DefaultSubscriptions contains curated public V2Ray subscription endpoints.
var DefaultSubscriptions = []string{
	"https://raw.githubusercontent.com/Epodonios/v2ray-configs/main/All_Configs_Sub.txt",
	"https://raw.githubusercontent.com/barry-far/V2ray-Config/main/All_Configs_Sub.txt",
	"https://github.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt",
	"https://raw.githubusercontent.com/MatinGhanbari/v2ray-configs/main/subscriptions/v2ray/all_sub.txt",
	"https://raw.githubusercontent.com/0xRadikal/Free-v2ray-Configs/main/all/configs.txt",
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

// FetchSubscriptions fetches multiple subscription URLs concurrently/sequentially and extracts proxy links.
func FetchSubscriptions(ctx context.Context, urls []string, client *http.Client) []string {
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	var allLinks []string
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

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, normURL, nil)
		if err != nil {
			log.Printf("warn: failed to create request for %s: %v", normURL, err)
			continue
		}
		req.Header.Set("User-Agent", "v2rayN/6.0")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("warn: failed to fetch subscription %s: %v", normURL, err)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			log.Printf("warn: subscription %s returned status %d", normURL, resp.StatusCode)
			resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("warn: failed to read response body for %s: %v", normURL, err)
			continue
		}

		extracted := DecodePayload(body)
		allLinks = append(allLinks, extracted...)
	}

	return allLinks
}

func isProxyLink(s string) bool {
	return strings.HasPrefix(s, "vless://") ||
		strings.HasPrefix(s, "vmess://") ||
		strings.HasPrefix(s, "trojan://") ||
		strings.HasPrefix(s, "ss://")
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
