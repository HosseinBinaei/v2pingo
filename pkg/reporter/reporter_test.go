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

		if !strings.Contains(filename, "results") || !strings.HasSuffix(filename, ".md") {
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

func TestReporter_ProgressIndicator(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(2500, false)
	r.out = &buf

	// Record 99 results
	for i := 1; i <= 99; i++ {
		r.Record(types.TestResult{
			Config:  types.ProxyConfig{Index: i, Protocol: "vless", Host: "127.0.0.1", Port: 80},
			Success: false,
		})
	}

	if strings.Contains(buf.String(), "Progress:") {
		t.Errorf("expected no progress output before 100 tests, got: %q", buf.String())
	}

	// 100th result should trigger progress update
	r.Record(types.TestResult{
		Config:  types.ProxyConfig{Index: 100, Protocol: "vless", Host: "127.0.0.1", Port: 80},
		Success: false,
	})

	if !strings.Contains(buf.String(), "Progress: 4.0% (100/2500 tested)") {
		t.Errorf("expected 4.0%% progress output at 100 tests, got: %q", buf.String())
	}

	buf.Reset()

	// Create reporter with 150 total configs and record 150 tests to check end batch update
	r2 := NewReporter(150, false)
	r2.out = &buf
	for i := 1; i <= 150; i++ {
		r2.Record(types.TestResult{
			Config:  types.ProxyConfig{Index: i, Protocol: "vless", Host: "127.0.0.1", Port: 80},
			Success: false,
		})
	}

	outStr := buf.String()
	if !strings.Contains(outStr, "Progress: 100.0% (150/150 tested)") {
		t.Errorf("expected 100.0%% progress output at end, got: %q", outStr)
	}
}

func TestReporter_ProgressClearingBeforeLog(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(2500, false)
	r.out = &buf

	// Record 100 results to trigger progress update
	for i := 1; i <= 100; i++ {
		r.Record(types.TestResult{
			Config:  types.ProxyConfig{Index: i, Protocol: "vless", Host: "127.0.0.1", Port: 80},
			Success: false,
		})
	}

	progressStr := "Progress: 4.0% (100/2500 tested)"
	if !strings.Contains(buf.String(), progressStr) {
		t.Fatalf("expected progress string in buffer, got %q", buf.String())
	}

  // 101st result is a success log
	r.Record(types.TestResult{
		Config:     types.ProxyConfig{Index: 101, Protocol: "vless", Host: "1.1.1.1", Port: 443},
		Success:    true,
		StatusCode: 200,
		Latency:    50 * time.Millisecond,
	})

	outStr := buf.String()
	// Verify clear line escape code \r\033[K is printed right before the success log
	expectedLog := "\r\033[K[✓ SUCCESS] [#101] [vless] 1.1.1.1:443 - 50ms (HTTP 200)\n"
	if !strings.Contains(outStr, expectedLog) {
		t.Errorf("expected clear line code before log message.\nGot:\n%q\nExpected to contain:\n%q", outStr, expectedLog)
	}
}

func TestReporter_SaveResults_AcceptsAll2xx(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(10, false)
	r.out = &buf

	cases := []struct {
		index      int
		statusCode int
		success    bool
		rawLink    string
		wantSaved  bool
	}{
		{1, 200, true, "vless://user@1.1.1.1:443#node200", true},
		{2, 201, true, "vless://user@1.1.1.2:443#node201", true},
		{3, 204, true, "vless://user@1.1.1.3:443#node204", true},
		{4, 206, true, "vless://user@1.1.1.4:443#node206", true},
		{5, 299, true, "vless://user@1.1.1.5:443#node299", true},
		{6, 199, true, "vless://user@1.1.1.6:443#node199", false},
		{7, 300, true, "vless://user@1.1.1.7:443#node300", false},
		{8, 301, true, "vless://user@1.1.1.8:443#node301", false},
		{9, 404, true, "vless://user@1.1.1.9:443#node404", false},
		{10, 201, false, "vless://user@1.1.1.10:443#node201-fail", false},
	}

	for _, tc := range cases {
		r.Record(types.TestResult{
			Config: types.ProxyConfig{
				Index:    tc.index,
				Protocol: "vless",
				Host:     "1.1.1.1",
				Port:     443,
				RawLink:  tc.rawLink,
			},
			Success:    tc.success,
			StatusCode: tc.statusCode,
			Latency:    10 * time.Millisecond,
		})
	}

	filename, err := r.SaveResults()
	if err != nil {
		t.Fatalf("unexpected error saving results: %v", err)
	}
	defer os.Remove(filename)

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	got := string(content)
	for _, tc := range cases {
		contains := strings.Contains(got, tc.rawLink)
		if tc.wantSaved && !contains {
			t.Errorf("expected status %d link %q to be saved, file content:\n%q", tc.statusCode, tc.rawLink, got)
		}
		if !tc.wantSaved && contains {
			t.Errorf("expected status %d link %q NOT to be saved, file content:\n%q", tc.statusCode, tc.rawLink, got)
		}
	}
}
