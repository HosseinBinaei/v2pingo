package subscription

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "GitHub blob URL standard",
			input:    "https://github.com/ebrasha/free-v2ray-public-list/blob/main/V2Ray-Config-By-EbraSha-All-Type.txt",
			expected: "https://raw.githubusercontent.com/ebrasha/free-v2ray-public-list/main/V2Ray-Config-By-EbraSha-All-Type.txt",
		},
		{
			name:     "GitHub blob URL with whitespace",
			input:    "  https://github.com/user/repo/blob/master/sub.txt \n\t",
			expected: "https://raw.githubusercontent.com/user/repo/master/sub.txt",
		},
		{
			name:     "GitHub blob URL with http prefix",
			input:    "http://github.com/user/repo/blob/v1/sub.txt",
			expected: "https://raw.githubusercontent.com/user/repo/v1/sub.txt",
		},
		{
			name:     "GitHub raw URL untouched",
			input:    "https://raw.githubusercontent.com/user/repo/main/sub.txt",
			expected: "https://raw.githubusercontent.com/user/repo/main/sub.txt",
		},
		{
			name:     "Regular non-GitHub URL",
			input:    "https://example.com/sub/list.txt",
			expected: "https://example.com/sub/list.txt",
		},
		{
			name:     "Empty URL",
			input:    "   ",
			expected: "",
		},
		{
			name:     "Invalid parseable URL",
			input:    "https://example.com/a b/c",
			expected: "https://example.com/a b/c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLoadSubscriptionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub-input.txt")
	content := strings.Join([]string{
		"# v2pingo subscription list",
		"",
		"https://example.com/one.txt",
		"   ",
		"https://example.com/two.txt",
		"  https://example.com/three.txt  ",
		"https://example.com/one.txt",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp subscription file: %v", err)
	}

	urls, err := LoadSubscriptionFile(path)
	if err != nil {
		t.Fatalf("LoadSubscriptionFile returned error: %v", err)
	}

	expected := []string{
		"https://example.com/one.txt",
		"https://example.com/two.txt",
		"https://example.com/three.txt",
	}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d urls, got %d (%v)", len(expected), len(urls), urls)
	}
	for i, want := range expected {
		if urls[i] != want {
			t.Errorf("urls[%d] = %q, want %q", i, urls[i], want)
		}
	}
}

func TestLoadSubscriptionFileMissing(t *testing.T) {
	if _, err := LoadSubscriptionFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("expected an error for a missing subscription file")
	}
}

func TestResolveSubscriptionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub-input.txt")
	if err := os.WriteFile(path, []byte("https://example.com/one.txt\n"), 0644); err != nil {
		t.Fatalf("failed to write temp subscription file: %v", err)
	}

	if got := ResolveSubscriptionFile(path); got != path {
		t.Errorf("ResolveSubscriptionFile(existing) = %q, want %q", got, path)
	}

	missing := filepath.Join(dir, "nested", "missing.txt")
	if got := ResolveSubscriptionFile(missing); got != missing {
		t.Errorf("ResolveSubscriptionFile(absolute missing) = %q, want %q", got, missing)
	}
}

func TestExtractProxyLinks(t *testing.T) {
	input := `
# Comment line
vless://uuid-1@1.1.1.1:443?security=tls#Node1
vmess://eyJ2IjoiMiIsInBzIjoiTm9kZTIifQ==
invalid-link://something
trojan://pass@2.2.2.2:443#Node3
ss://YWVzLTEyOC1nY206cGFzc0AxLjEuMS4xOjQ0Mw==#Node4

   vless://uuid-2@3.3.3.3:443?type=ws#Node5   
`
	links := ExtractProxyLinks(input)
	expected := []string{
		"vless://uuid-1@1.1.1.1:443?security=tls#Node1",
		"vmess://eyJ2IjoiMiIsInBzIjoiTm9kZTIifQ==",
		"trojan://pass@2.2.2.2:443#Node3",
		"ss://YWVzLTEyOC1nY206cGFzc0AxLjEuMS4xOjQ0Mw==#Node4",
		"vless://uuid-2@3.3.3.3:443?type=ws#Node5",
	}

	if len(links) != len(expected) {
		t.Fatalf("ExtractProxyLinks returned %d links, want %d: %v", len(links), len(expected), links)
	}

	for i, exp := range expected {
		if links[i] != exp {
			t.Errorf("links[%d] = %q, want %q", i, links[i], exp)
		}
	}
}

func TestExtractProxyLinksNewProtocols(t *testing.T) {
	input := `
# Comment line
socks://dXNlcjpwYXNz@1.1.1.1:1080#SOCKS5Node
socks5://user:pass@2.2.2.2:1080#SOCKS5Plain
hysteria2://secret@3.3.3.3:443?obfs=salamander&obfs-password=obfs#HY2Node
hy2://secret@4.4.4.4:443#HY2Alias
wireguard://privatekey@5.5.5.5:51820?publickey=publickey&address=10.0.0.2/32#WGNode
wg://privatekey@6.6.6.6:2408?publickey=publickey&address=10.0.0.2/32#WGAlias
`

	links := ExtractProxyLinks(input)
	expected := []string{
		"socks://dXNlcjpwYXNz@1.1.1.1:1080#SOCKS5Node",
		"socks5://user:pass@2.2.2.2:1080#SOCKS5Plain",
		"hysteria2://secret@3.3.3.3:443?obfs=salamander&obfs-password=obfs#HY2Node",
		"hy2://secret@4.4.4.4:443#HY2Alias",
		"wireguard://privatekey@5.5.5.5:51820?publickey=publickey&address=10.0.0.2/32#WGNode",
		"wg://privatekey@6.6.6.6:2408?publickey=publickey&address=10.0.0.2/32#WGAlias",
	}

	if len(links) != len(expected) {
		t.Fatalf("ExtractProxyLinks returned %d links, want %d: %v", len(links), len(expected), links)
	}
	for i, exp := range expected {
		if links[i] != exp {
			t.Errorf("links[%d] = %q, want %q", i, links[i], exp)
		}
	}
}

// TestExtractProxyLinksBareAddresses pins that bare "ip:port" lines are
// extracted (later classified as SOCKS5) while look-alikes that are not plain
// IP addresses keep being ignored.
func TestExtractProxyLinksBareAddresses(t *testing.T) {
	input := `
# Plain SOCKS5 address list
1.2.3.4:1080
5.6.7.8:1080
# Not bare addresses
example.com:1080
1.2.3.4
1.2.3.4:0
1.2.3.4:70000
256.1.1.1:1080
# Mixed with scheme links
ss://YWVzLTEyOC1nY206cGFzc0AxLjEuMS4xOjQ0Mw==#Node4
socks5://user:pass@9.9.9.9:1080#Explicit
[2001:db8::1]:1080
`
	links := ExtractProxyLinks(input)
	expected := []string{
		"1.2.3.4:1080",
		"5.6.7.8:1080",
		"ss://YWVzLTEyOC1nY206cGFzc0AxLjEuMS4xOjQ0Mw==#Node4",
		"socks5://user:pass@9.9.9.9:1080#Explicit",
		"[2001:db8::1]:1080",
	}

	if len(links) != len(expected) {
		t.Fatalf("ExtractProxyLinks returned %d links, want %d: %v", len(links), len(expected), links)
	}
	for i, exp := range expected {
		if links[i] != exp {
			t.Errorf("links[%d] = %q, want %q", i, links[i], exp)
		}
	}
}

func TestDecodePayload(t *testing.T) {
	sampleLinks := []string{
		"vless://uuid-1@1.1.1.1:443?security=tls#Node1",
		"vmess://eyJ2IjoiMiIsInBzIjoiTm9kZTIifQ==",
		"trojan://pass@2.2.2.2:443#Node3",
		"ss://YWVzLTEyOC1nY206cGFzc0AxLjEuMS4xOjQ0Mw==#Node4",
	}
	rawText := strings.Join(sampleLinks, "\n")

	t.Run("Plain text payload", func(t *testing.T) {
		got := DecodePayload([]byte(rawText))
		if len(got) != len(sampleLinks) {
			t.Fatalf("DecodePayload returned %d links, want %d", len(got), len(sampleLinks))
		}
		for i, exp := range sampleLinks {
			if got[i] != exp {
				t.Errorf("got[%d] = %q, want %q", i, got[i], exp)
			}
		}
	})

	t.Run("Base64 StdEncoding single block", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte(rawText))
		got := DecodePayload([]byte(encoded))
		if len(got) != len(sampleLinks) {
			t.Fatalf("DecodePayload returned %d links, want %d", len(got), len(sampleLinks))
		}
	})

	t.Run("Base64 RawStdEncoding (unpadded)", func(t *testing.T) {
		encoded := base64.RawStdEncoding.EncodeToString([]byte(rawText))
		got := DecodePayload([]byte(encoded))
		if len(got) != len(sampleLinks) {
			t.Fatalf("DecodePayload returned %d links, want %d", len(got), len(sampleLinks))
		}
	})

	t.Run("Base64 URLEncoding", func(t *testing.T) {
		encoded := base64.URLEncoding.EncodeToString([]byte(rawText))
		got := DecodePayload([]byte(encoded))
		if len(got) != len(sampleLinks) {
			t.Fatalf("DecodePayload returned %d links, want %d", len(got), len(sampleLinks))
		}
	})

	t.Run("Base64 RawURLEncoding", func(t *testing.T) {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(rawText))
		got := DecodePayload([]byte(encoded))
		if len(got) != len(sampleLinks) {
			t.Fatalf("DecodePayload returned %d links, want %d", len(got), len(sampleLinks))
		}
	})

	t.Run("Base64 with newlines interspersed", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte(rawText))
		// insert newlines every 20 chars
		var chunked string
		for i := 0; i < len(encoded); i += 20 {
			end := i + 20
			if end > len(encoded) {
				end = len(encoded)
			}
			chunked += encoded[i:end] + "\r\n"
		}
		got := DecodePayload([]byte(chunked))
		if len(got) != len(sampleLinks) {
			t.Fatalf("DecodePayload returned %d links, want %d", len(got), len(sampleLinks))
		}
	})

	t.Run("Mixed format payload (plain + base64 lines)", func(t *testing.T) {
		b64part := base64.StdEncoding.EncodeToString([]byte("vless://b64@5.5.5.5:443#B64Node"))
		mixed := "trojan://pass@6.6.6.6:443#PlainNode\n" + b64part + "\n# Some comment\n"
		got := DecodePayload([]byte(mixed))
		if len(got) != 2 {
			t.Fatalf("DecodePayload returned %d links, want 2: %v", len(got), got)
		}
		if got[0] != "trojan://pass@6.6.6.6:443#PlainNode" {
			t.Errorf("got[0] = %q", got[0])
		}
		if got[1] != "vless://b64@5.5.5.5:443#B64Node" {
			t.Errorf("got[1] = %q", got[1])
		}
	})

	t.Run("Empty or garbage payload", func(t *testing.T) {
		got := DecodePayload([]byte("random binary garbage \x00\x01\x02 \r\n not a config"))
		if len(got) != 0 {
			t.Errorf("expected 0 links for garbage, got %d", len(got))
		}
	})

	t.Run("Plain text payload with bare addresses", func(t *testing.T) {
		got := DecodePayload([]byte("1.2.3.4:1080\nexample.com:1080\n5.6.7.8:1080\n"))
		want := []string{"1.2.3.4:1080", "5.6.7.8:1080"}
		if len(got) != len(want) {
			t.Fatalf("DecodePayload returned %d links, want %d: %v", len(got), len(want), got)
		}
		for i, exp := range want {
			if got[i] != exp {
				t.Errorf("got[%d] = %q, want %q", i, got[i], exp)
			}
		}
	})

	t.Run("Whole-payload base64 of bare addresses", func(t *testing.T) {
		encoded := base64.StdEncoding.EncodeToString([]byte("1.2.3.4:1080\n5.6.7.8:1080\n"))
		got := DecodePayload([]byte(encoded))
		if len(got) != 2 {
			t.Fatalf("DecodePayload returned %d links, want 2: %v", len(got), got)
		}
		if got[0] != "1.2.3.4:1080" || got[1] != "5.6.7.8:1080" {
			t.Errorf("bare address links mismatch: %v", got)
		}
	})

	t.Run("Bare address line next to a base64 line", func(t *testing.T) {
		b64part := base64.StdEncoding.EncodeToString([]byte("vless://b64@5.5.5.5:443#B64Node"))
		mixed := "1.2.3.4:1080\n" + b64part + "\n"
		got := DecodePayload([]byte(mixed))
		if len(got) != 2 {
			t.Fatalf("DecodePayload returned %d links, want 2: %v", len(got), got)
		}
		if got[0] != "1.2.3.4:1080" {
			t.Errorf("got[0] = %q, want the bare address", got[0])
		}
		if got[1] != "vless://b64@5.5.5.5:443#B64Node" {
			t.Errorf("got[1] = %q, want the decoded share link", got[1])
		}
	})
}

func TestFetchSubscriptions(t *testing.T) {
	userAgentReceived := ""
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgentReceived = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("vless://user1@1.1.1.1:443#Server1\nvmess://eyJ2IjoiMiJ9#Server1VMess\n"))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		encoded := base64.StdEncoding.EncodeToString([]byte("trojan://pass@2.2.2.2:443#Server2\n"))
		w.Write([]byte(encoded))
	}))
	defer ts2.Close()

	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer ts404.Close()

	ts500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer ts500.Close()

	t.Run("Fetch multiple URLs with resilience to failures", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		urls := []string{
			ts1.URL,
			ts404.URL,
			"http://127.0.0.1:0/invalid-port", // network error
			ts2.URL,
			ts500.URL,
		}

		links := FetchSubscriptions(ctx, urls, nil, "", FetchNetwork)
		expectedCount := 3 // 2 from ts1 + 1 from ts2
		if len(links) != expectedCount {
			t.Fatalf("FetchSubscriptions returned %d links, want %d: %v", len(links), expectedCount, links)
		}

		if !strings.Contains(userAgentReceived, "v2ray") && !strings.Contains(userAgentReceived, "v2rayN") {
			t.Errorf("User-Agent header was %q, expected v2rayN user agent", userAgentReceived)
		}
	})

	t.Run("Context cancellation handling", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		links := FetchSubscriptions(ctx, []string{ts1.URL}, nil, "", FetchNetwork)
		if len(links) != 0 {
			t.Errorf("expected 0 links on canceled context, got %d", len(links))
		}
	})
}

func TestFetchSubscriptionsWithWarnRoutesWarningsToCallback(t *testing.T) {
	tsOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("vless://user1@1.1.1.1:443#Server1\n"))
	}))
	defer tsOK.Close()

	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer ts404.Close()

	var warnings []string
	links := FetchSubscriptionsWithWarn(context.Background(), []string{tsOK.URL, ts404.URL}, nil, "", FetchNetwork, func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	if len(links) != 1 {
		t.Fatalf("expected 1 link from the healthy source, got %v", links)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], ts404.URL) {
		t.Fatalf("expected exactly the failing source to reach the callback, got %v", warnings)
	}

	// A nil sink must not panic and must still return the healthy links.
	if links := FetchSubscriptionsWithWarn(context.Background(), []string{tsOK.URL, ts404.URL}, nil, "", FetchNetwork, nil); len(links) != 1 {
		t.Fatalf("expected the nil warn sink to keep working, got links %v", links)
	}
}

// TestFetchSubscriptionsKeepsWarnPrefixInLog pins the CLI behavior: the
// standard-logger variant prefixes every warning with "warn:".
func TestFetchSubscriptionsKeepsWarnPrefixInLog(t *testing.T) {
	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer ts404.Close()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	FetchSubscriptions(context.Background(), []string{ts404.URL}, nil, "", FetchNetwork)
	if out := buf.String(); !strings.Contains(out, "warn: ") || !strings.Contains(out, ts404.URL) {
		t.Fatalf("expected a 'warn:' line naming the source in the log, got %q", out)
	}
}
