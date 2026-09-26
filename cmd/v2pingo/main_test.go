package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary execution failed: %v\nOutput: %s", err, string(out))
	}

	outStr := string(out)
	if !strings.Contains(outStr, "Fetching subscriptions from 1 sources...") {
		t.Errorf("output missing fetching line: %s", outStr)
	}
	if !strings.Contains(outStr, "Total unique configs to test: 1") {
		t.Errorf("output missing unique configs line: %s", outStr)
	}
	if !strings.Contains(outStr, "TEST SUMMARY") {
		t.Errorf("output missing TEST SUMMARY: %s", outStr)
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

	cmd := exec.Command(binPath, "-sub-file", subFile, "-c", "2", "-timeout", "1s")
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
	if !strings.Contains(outStr, "Total unique configs to test: 1") {
		t.Errorf("output missing unique configs line: %s", outStr)
	}
	if !strings.Contains(outStr, "TEST SUMMARY") {
		t.Errorf("output missing TEST SUMMARY: %s", outStr)
	}
}
