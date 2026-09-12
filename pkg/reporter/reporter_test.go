package reporter

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"v2pingo/pkg/types"
)

func TestReporter_SuccessAndFailureCounting(t *testing.T) {
	var buf bytes.Buffer
	r := &Reporter{
		totalConfigs: 2,
		verbose:      true,
		out:          &buf,
	}

	r.Record(types.TestResult{
		Config: types.ProxyConfig{
			Index:    1,
			Protocol: "vless",
			Host:     "example.com",
			Port:     443,
		},
		Success:    true,
		StatusCode: 200,
		Latency:    238 * time.Millisecond,
	})

	r.Record(types.TestResult{
		Config: types.ProxyConfig{
			Index:    2,
			Protocol: "vmess",
			Host:     "test.net",
			Port:     443,
		},
		Success: false,
		Error:   fmt.Errorf("timeout"),
	})

	if r.testedCount != 2 {
		t.Errorf("expected testedCount=2, got %d", r.testedCount)
	}
	if r.successCount != 1 {
		t.Errorf("expected successCount=1, got %d", r.successCount)
	}
	if r.failCount != 1 {
		t.Errorf("expected failCount=1, got %d", r.failCount)
	}

	output := buf.String()
	if !strings.Contains(output, "[✓ SUCCESS] [#1] [vless] example.com:443 - 238ms (HTTP 200)") {
		t.Errorf("unexpected success output: %q", output)
	}
	if !strings.Contains(output, "[✗ FAILED] [#2] [vmess] test.net:443 - timeout") {
		t.Errorf("unexpected failure output: %q", output)
	}
}

func TestReporter_NonVerboseOmitsFailures(t *testing.T) {
	var buf bytes.Buffer
	r := &Reporter{
		totalConfigs: 2,
		verbose:      false,
		out:          &buf,
	}

	r.Record(types.TestResult{
		Config: types.ProxyConfig{
			Index:    1,
			Protocol: "vless",
			Host:     "example.com",
			Port:     443,
		},
		Success:    true,
		StatusCode: 200,
		Latency:    100 * time.Millisecond,
	})

	r.Record(types.TestResult{
		Config: types.ProxyConfig{
			Index:    2,
			Protocol: "vmess",
			Host:     "test.net",
			Port:     443,
		},
		Success: false,
		Error:   fmt.Errorf("connection refused"),
	})

	output := buf.String()
	if !strings.Contains(output, "[✓ SUCCESS]") {
		t.Errorf("expected success to be printed even in non-verbose mode")
	}
	if strings.Contains(output, "[✗ FAILED]") {
		t.Errorf("expected failure NOT to be printed in non-verbose mode")
	}
}

func TestReporter_PrintSummary(t *testing.T) {
	var buf bytes.Buffer
	r := &Reporter{
		totalConfigs: 2500,
		verbose:      false,
		out:          &buf,
		testedCount:  2500,
		successCount: 180,
		failCount:    2320,
	}

	r.PrintSummary(8*time.Minute + 12*time.Second)

	output := buf.String()
	expectedSummary := `==================================================
TEST SUMMARY
==================================================
Total configs: 2500
Tested:        2500
Success:       180
Failed:        2320
Duration:      8m 12s
==================================================`

	if !strings.Contains(output, expectedSummary) {
		t.Errorf("summary output mismatch.\nGot:\n%s\nExpected to contain:\n%s", output, expectedSummary)
	}
}

func TestReporter_SaveResults(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(2, false)
	r.out = &buf

	r.Record(types.TestResult{
		Config: types.ProxyConfig{
			Index:    1,
			Protocol: "vless",
			Host:     "1.1.1.1",
			Port:     443,
			RawLink:  "vless://user@1.1.1.1:443#node1",
		},
		Success:    true,
		StatusCode: 200,
		Latency:    100 * time.Millisecond,
	})

	r.Record(types.TestResult{
		Config: types.ProxyConfig{
			Index:    2,
			Protocol: "vmess",
			Host:     "2.2.2.2",
			Port:     443,
			RawLink:  "vmess://encoded_config_string",
		},
		Success: false,
		Error:   fmt.Errorf("timeout"),
	})

	filename, err := r.SaveResults()
	if err != nil {
		t.Fatalf("unexpected error saving results: %v", err)
	}
	defer os.Remove(filename)

	if !strings.HasPrefix(filename, "results_") || !strings.HasSuffix(filename, ".md") {
		t.Errorf("unexpected filename pattern: %s", filename)
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	expectedContent := "vless://user@1.1.1.1:443#node1\n"
	if string(content) != expectedContent {
		t.Errorf("file content mismatch.\nGot:\n%q\nExpected:\n%q", string(content), expectedContent)
	}

	if !strings.Contains(buf.String(), filename) {
		t.Errorf("expected stdout to mention saved filename, got: %q", buf.String())
	}
}
