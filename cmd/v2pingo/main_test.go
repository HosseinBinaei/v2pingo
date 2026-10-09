package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// goExecutable returns the Go tool used to build the integration test binary.
func goExecutable() string {
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	return `C:\Program Files\Go\bin\go.exe`
}

func TestStringSliceFlag(t *testing.T) {
	var s stringSliceFlag
	if s.String() != "" {
		t.Errorf("expected empty string, got %s", s.String())
	}

	err := s.Set("http://sub1.com, http://sub2.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(s) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(s))
	}
	if s[0] != "http://sub1.com" || s[1] != "http://sub2.com" {
		t.Errorf("unexpected elements: %v", s)
	}

	err = s.Set("http://sub3.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(s) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(s))
	}
	if s.String() != "http://sub1.com,http://sub2.com,http://sub3.com" {
		t.Errorf("unexpected string representation: %s", s.String())
	}
}

func TestCLIIntegration(t *testing.T) {
	// Create mock server for subscription endpoint
	mockSubContent := "vless://00000000-0000-0000-0000-000000000000@127.0.0.1:1080?type=tcp#TestVLESS\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockSubContent))
	}))
	defer ts.Close()

	// Build temporary binary
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "v2pingo_test.exe")

	buildCmd := exec.Command(goExecutable(), "build", "-o", binPath, ".")
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test binary: %v\nOutput: %s", err, string(buildOut))
	}

	// Run binary with flags
	cmd := exec.Command(binPath,
		"-default-subs=false",
		"-sub", ts.URL,
		"-c", "2",
		"-timeout", "1s",
		"-v",
		// Keep the integration test offline: no paste.rs upload attempt.
		"-paste=false",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}

	outStr := string(out)
	if !strings.Contains(outStr, "Fetching subscriptions from 1 sources...") {
		t.Errorf("output missing fetching line: %s", outStr)
	}
	if !strings.Contains(outStr, "Extracted 1 raw proxy share links.") {
		t.Errorf("output missing extracted links line: %s", outStr)
	}
	if !strings.Contains(outStr, "TEST SUMMARY") {
		t.Errorf("output missing TEST SUMMARY: %s", outStr)
	}
	if !strings.Contains(outStr, "Healthy links appended to") {
		t.Errorf("output missing appended results line: %s", outStr)
	}
}

// serveSOCKS5 accepts CONNECT requests on ln and forwards them to the requested
// address. It is a minimal no-auth SOCKS5 server that gives the paste
// integration test a genuinely working proxy without touching the network.
func serveSOCKS5(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			br := bufio.NewReader(c)
			header := make([]byte, 2)
			if _, err := io.ReadFull(br, header); err != nil || header[0] != 5 {
				return
			}
			methods := make([]byte, int(header[1]))
			if _, err := io.ReadFull(br, methods); err != nil {
				return
			}
			if _, err := c.Write([]byte{5, 0}); err != nil { // no authentication required
				return
			}

			req := make([]byte, 4)
			if _, err := io.ReadFull(br, req); err != nil || req[0] != 5 || req[1] != 1 {
				return
			}
			var host string
			switch req[3] {
			case 1:
				b := make([]byte, 4)
				if _, err := io.ReadFull(br, b); err != nil {
					return
				}
				host = net.IP(b).String()
			case 3:
				n := make([]byte, 1)
				if _, err := io.ReadFull(br, n); err != nil {
					return
				}
				b := make([]byte, int(n[0]))
				if _, err := io.ReadFull(br, b); err != nil {
					return
				}
				host = string(b)
			case 4:
				b := make([]byte, 16)
				if _, err := io.ReadFull(br, b); err != nil {
					return
				}
				host = net.IP(b).String()
			default:
				return
			}
			portBytes := make([]byte, 2)
			if _, err := io.ReadFull(br, portBytes); err != nil {
				return
			}
			port := int(portBytes[0])<<8 | int(portBytes[1])

			upstream, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 5*time.Second)
			if err != nil {
				_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0}) // connection refused
				return
			}
			defer upstream.Close()
			if _, err := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
				return
			}
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(upstream, br); done <- struct{}{} }()
			go func() { _, _ = io.Copy(c, upstream); done <- struct{}{} }()
			<-done
		}(conn)
	}
}

// TestCLIIntegrationPastesResults exercises the full requested flow against a
// mock paste service: once a healthy config is saved to the results file, the
// program asks whether to publish it, an affirmative answer uploads the file,
// and the returned link is printed.
func TestCLIIntegrationPastesResults(t *testing.T) {
	var mu sync.Mutex
	var pastedBody string
	var pasteHits int
	pasteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		pastedBody = string(body)
		pasteHits++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, "https://paste.rs/xyz789")
	}))
	defer pasteSrv.Close()

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer targetSrv.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start the local SOCKS5 proxy: %v", err)
	}
	defer ln.Close()
	go serveSOCKS5(ln)

	proxyLink := "socks5://" + ln.Addr().String() + "#local"
	subSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, proxyLink)
	}))
	defer subSrv.Close()

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "v2pingo_paste_test.exe")
	buildCmd := exec.Command(goExecutable(), "build", "-o", binPath, ".")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v\nOutput: %s", err, string(buildOut))
	}

	cmd := exec.Command(binPath,
		"-default-subs=false",
		"-sub", subSrv.URL,
		"-url", targetSrv.URL,
		"-c", "1",
		"-timeout", "10s",
		"-paste-url", pasteSrv.URL,
	)
	// Isolate the results/ directory so the test does not pollute the repo.
	cmd.Dir = tmpDir
	// Answer the paste confirmation prompt with an explicit yes.
	cmd.Stdin = strings.NewReader("y\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}
	outStr := string(out)

	if !strings.Contains(outStr, "Upload the saved results to paste.rs") {
		t.Errorf("expected the paste confirmation prompt, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "Paste link: https://paste.rs/xyz789") {
		t.Errorf("expected the paste link to be printed, got:\n%s", outStr)
	}
	mu.Lock()
	defer mu.Unlock()
	if pasteHits != 1 {
		t.Errorf("expected exactly one paste upload, got %d", pasteHits)
	}
	if !strings.Contains(pastedBody, proxyLink) {
		t.Errorf("expected the uploaded body to contain the healthy config %q, got: %q", proxyLink, pastedBody)
	}
}

// TestCLIIntegrationPasteDeclined verifies that any answer other than y/yes
// keeps the results local: no upload, no link.
func TestCLIIntegrationPasteDeclined(t *testing.T) {
	var mu sync.Mutex
	pasteHits := 0
	pasteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pasteHits++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, "https://paste.rs/should-not-happen")
	}))
	defer pasteSrv.Close()

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer targetSrv.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start the local SOCKS5 proxy: %v", err)
	}
	defer ln.Close()
	go serveSOCKS5(ln)

	subSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "socks5://"+ln.Addr().String()+"#local")
	}))
	defer subSrv.Close()

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "v2pingo_paste_decline_test.exe")
	buildCmd := exec.Command(goExecutable(), "build", "-o", binPath, ".")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v\nOutput: %s", err, string(buildOut))
	}

	cmd := exec.Command(binPath,
		"-default-subs=false",
		"-sub", subSrv.URL,
		"-url", targetSrv.URL,
		"-c", "1",
		"-timeout", "10s",
		"-paste-url", pasteSrv.URL,
	)
	cmd.Dir = tmpDir
	cmd.Stdin = strings.NewReader("n\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}
	outStr := string(out)

	if strings.Contains(outStr, "Paste link:") {
		t.Errorf("declined run must not print a paste link, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "Skipped paste upload.") {
		t.Errorf("expected a skipped-upload notice, got:\n%s", outStr)
	}
	mu.Lock()
	defer mu.Unlock()
	if pasteHits != 0 {
		t.Errorf("declined run must not upload anything, got %d upload(s)", pasteHits)
	}
}

// localProxyEnv starts a loopback SOCKS5 proxy plus a subscription server that
// offers it, so a CLI test can run the whole flow without touching the network.
// It returns the subscription URL and the target URL the tested node must reach.
func localProxyEnv(t *testing.T) (subURL, targetURL string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start the local SOCKS5 proxy: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go serveSOCKS5(ln)

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(targetSrv.Close)

	subSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "socks5://"+ln.Addr().String()+"#local")
	}))
	t.Cleanup(subSrv.Close)

	return subSrv.URL, targetSrv.URL
}

// pasteTestBinary builds the CLI once for a paste integration test and returns
// its path plus the isolated working directory the run must use.
func pasteTestBinary(t *testing.T, name string) string {
	t.Helper()
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, name)
	buildCmd := exec.Command(goExecutable(), "build", "-o", binPath, ".")
	if buildOut, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v\nOutput: %s", err, string(buildOut))
	}
	return binPath
}

// TestCLIIntegrationPasteRetryAfterFailure verifies that a failed upload is
// reported and, when the user answers yes to the retry question, the upload is
// attempted again and the link is printed.
func TestCLIIntegrationPasteRetryAfterFailure(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	pasteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		attempt := hits
		mu.Unlock()
		if attempt == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "boom")
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, "https://paste.rs/retry123")
	}))
	defer pasteSrv.Close()

	subURL, targetURL := localProxyEnv(t)
	binPath := pasteTestBinary(t, "v2pingo_paste_retry_test.exe")

	cmd := exec.Command(binPath,
		"-default-subs=false",
		"-sub", subURL,
		"-url", targetURL,
		"-c", "1",
		"-timeout", "10s",
		"-paste-url", pasteSrv.URL,
	)
	cmd.Dir = filepath.Dir(binPath)
	// yes to the initial prompt, then yes to the retry prompt.
	cmd.Stdin = strings.NewReader("y\ny\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}
	outStr := string(out)

	if !strings.Contains(outStr, "failed to upload results file") {
		t.Errorf("expected the first upload failure to be reported, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "Retry uploading to paste.rs") {
		t.Errorf("expected the retry question after the failure, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "Paste link: https://paste.rs/retry123") {
		t.Errorf("expected the link from the successful retry, got:\n%s", outStr)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Errorf("expected exactly two upload attempts (one failed, one retried), got %d", hits)
	}
}

// TestCLIIntegrationPasteRetryDeclined verifies that declining the retry
// question ends the run: the failure is reported, no further attempt is made,
// and no link is printed.
func TestCLIIntegrationPasteRetryDeclined(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	pasteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "boom")
	}))
	defer pasteSrv.Close()

	subURL, targetURL := localProxyEnv(t)
	binPath := pasteTestBinary(t, "v2pingo_paste_retry_decline_test.exe")

	cmd := exec.Command(binPath,
		"-default-subs=false",
		"-sub", subURL,
		"-url", targetURL,
		"-c", "1",
		"-timeout", "10s",
		"-paste-url", pasteSrv.URL,
	)
	cmd.Dir = filepath.Dir(binPath)
	// yes to the initial prompt, no to the retry question.
	cmd.Stdin = strings.NewReader("y\nn\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}
	outStr := string(out)

	if !strings.Contains(outStr, "Retry uploading to paste.rs") {
		t.Errorf("expected the retry question after the failure, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "Giving up on paste upload") {
		t.Errorf("expected the give-up notice, got:\n%s", outStr)
	}
	if strings.Contains(outStr, "Paste link:") {
		t.Errorf("declined retry must not print a paste link, got:\n%s", outStr)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("declined retry must not upload again, got %d attempt(s)", hits)
	}
}

func TestCLIIntegrationSubscriptionFile(t *testing.T) {
	// Create mock server for subscription endpoint
	mockSubContent := "vless://00000000-0000-0000-0000-000000000000@127.0.0.1:1080?type=tcp#FileVLESS\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockSubContent))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	subFile := filepath.Join(tmpDir, "sub-input.txt")
	// Comments, blank lines and duplicate URLs must not add extra subscriptions.
	content := "# subscriptions from file\n\n" + ts.URL + "\n" + ts.URL + "\n"
	if err := os.WriteFile(subFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write subscription file: %v", err)
	}

	binPath := filepath.Join(tmpDir, "v2pingo_subfile_test.exe")
	buildCmd := exec.Command(goExecutable(), "build", "-o", binPath, ".")
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test binary: %v\nOutput: %s", err, string(buildOut))
	}

	cmd := exec.Command(binPath, "-sub-file", subFile, "-c", "2", "-timeout", "1s", "-paste=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}

	outStr := string(out)
	if !strings.Contains(outStr, "Loaded 1 subscription URL(s) from "+subFile) {
		t.Errorf("output missing loaded subscription file line: %s", outStr)
	}
	if !strings.Contains(outStr, "Fetching subscriptions from 1 sources...") {
		t.Errorf("output missing fetching line: %s", outStr)
	}
	if !strings.Contains(outStr, "Extracted 1 raw proxy share links.") {
		t.Errorf("output missing extracted links line: %s", outStr)
	}
	if !strings.Contains(outStr, "TEST SUMMARY") {
		t.Errorf("output missing TEST SUMMARY: %s", outStr)
	}
}
