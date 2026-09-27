package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"v2pingo/pkg/runner"
	"v2pingo/pkg/types"
)

// Batching: raw links must be processed in 5k-style partitions with only one
// batch in RAM at a time, successes appended incrementally after each batch.
func TestRunBatches_PartitionsAndAppends(t *testing.T) {
	dir := t.TempDir()
	resultsFile := filepath.Join(dir, "results.md")
	checkpointFile := filepath.Join(dir, "checkpoint.txt")

	// Parseable but unroutable (TEST-NET-1): phase-1 TCP precheck fails fast,
	// so no box is built and the test needs no network.
	rawLinks := []string{
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.1:443?security=none#n1",
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.1:443?security=none#n1",
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.2:443?security=none#n2",
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.3:443?security=none#n3",
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.4:443?security=none#n4",
	}

	var got int
	err := runner.RunBatches(context.Background(), rawLinks, runner.BatchConfig{
		TargetURL:      "http://127.0.0.1:9/",
		Concurrency:    2,
		Timeout:        3 * time.Second,
		BatchSize:      2,
		ResultsFile:    resultsFile,
		CheckpointFile: checkpointFile,
	}, func(res types.TestResult) {
		got++
	})
	if err != nil {
		t.Fatalf("RunBatches returned error: %v", err)
	}
	// 5 raw links, 1 duplicate -> 4 unique configs tested in 2+2+1 batches.
	if got != 4 {
		t.Fatalf("expected 4 results (deduped), got %d", got)
	}
	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		t.Fatalf("expected checkpoint file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "3" {
		t.Fatalf("expected checkpoint=3 completed batches, got %q", string(data))
	}
}

// Resume: a second run with the same checkpoint must skip completed batches
// and must not duplicate appended results.
func TestRunBatches_ResumeSkipsCompleted(t *testing.T) {
	dir := t.TempDir()
	resultsFile := filepath.Join(dir, "results.md")
	checkpointFile := filepath.Join(dir, "checkpoint.txt")

	rawLinks := []string{
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.1:443?security=none#n1",
		"vless://12345678-1234-1234-1234-123456789012@192.0.2.2:443?security=none#n2",
	}

	cfg := runner.BatchConfig{
		TargetURL:      "http://127.0.0.1:9/",
		Concurrency:    2,
		Timeout:        3 * time.Second,
		BatchSize:      2,
		ResultsFile:    resultsFile,
		CheckpointFile: checkpointFile,
	}
	var first int
	if err := runner.RunBatches(context.Background(), rawLinks, cfg, func(res types.TestResult) { first++ }); err != nil {
		t.Fatalf("first run: %v", err)
	}
	var second int
	if err := runner.RunBatches(context.Background(), rawLinks, cfg, func(res types.TestResult) { second++ }); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second != 0 {
		t.Fatalf("expected resumed run to test 0 configs, got %d", second)
	}
}
