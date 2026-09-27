package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"v2pingo/pkg/parser"
	"v2pingo/pkg/reporter"
	"v2pingo/pkg/tester"
	"v2pingo/pkg/types"
)

// DefaultBatchSize bounds how many proxy configs are ever materialized in RAM
// at once. Raw subscription strings stay sliced in windows; only one batch of
// parsed ProxyConfig values exists at a time.
const DefaultBatchSize = 5000

// BatchConfig tunes a batched run. ResultsFile receives healthy links batch by
// batch (append mode); CheckpointFile stores the completed-batch count so an
// interrupted run resumes where it left off instead of starting over.
type BatchConfig struct {
	TargetURL      string
	Concurrency    int
	Timeout        time.Duration
	BatchSize      int
	ResultsFile    string
	CheckpointFile string
}

// DefaultResultsFile mirrors the reporter naming scheme for batched runs.
func DefaultResultsFile() string {
	return filepath.Join("results", fmt.Sprintf("results_%s.md", time.Now().Format("20060102_150405")))
}

func normalizeBatchSize(n int) int {
	if n <= 0 {
		return DefaultBatchSize
	}
	return n
}

func loadCheckpoint(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func saveCheckpoint(path string, completed int) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(strconv.Itoa(completed)), 0644)
}

// RunBatches processes rawLinks in fixed-size partitions: parse at most one
// batch of configs into RAM, test it with the two-phase pool (cheap TCP
// precheck inside tester.RunPool, then end-to-end HTTP), append that batch's
// healthy links to ResultsFile, record the checkpoint, and move to the next
// batch. A rerun with the same CheckpointFile skips already completed batches
// without duplicating appended results.
func RunBatches(ctx context.Context, rawLinks []string, cfg BatchConfig, onResult func(types.TestResult)) error {
	batchSize := normalizeBatchSize(cfg.BatchSize)
	resultsFile := cfg.ResultsFile
	if resultsFile == "" {
		resultsFile = DefaultResultsFile()
	}

	totalBatches := (len(rawLinks) + batchSize - 1) / batchSize
	completed := loadCheckpoint(cfg.CheckpointFile)
	if completed >= totalBatches {
		return nil
	}

	seen := make(map[string]bool)
	nextIndex := 1

	// Replay dedupe state for skipped batches without materializing them,
	// so global duplicate suppression and indices stay consistent on resume.
	for b := 0; b < completed; b++ {
		end := (b + 1) * batchSize
		if end > len(rawLinks) {
			end = len(rawLinks)
		}
		_, nextIndex = parser.DeduplicateAppend(rawLinks[b*batchSize:end], seen, nextIndex)
	}

	for b := completed; b < totalBatches; b++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := (b + 1) * batchSize
		if end > len(rawLinks) {
			end = len(rawLinks)
		}
		configs, next := parser.DeduplicateAppend(rawLinks[b*batchSize:end], seen, nextIndex)
		nextIndex = next

		if len(configs) > 0 {
			var batchLinks []string
			tester.RunPool(ctx, configs, cfg.TargetURL, cfg.Concurrency, cfg.Timeout, func(res types.TestResult) {
				if reporter.IsSavable(res) {
					batchLinks = append(batchLinks, res.Config.RawLink)
				}
				if onResult != nil {
					onResult(res)
				}
			})
			configs = nil
			if err := reporter.AppendLinks(resultsFile, batchLinks); err != nil {
				return err
			}
		}

		if err := saveCheckpoint(cfg.CheckpointFile, b+1); err != nil {
			return err
		}
	}
	return nil
}
