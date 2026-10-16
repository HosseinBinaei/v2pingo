package reporter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"v2pingo/pkg/types"
)

type Reporter struct {
	mu                   sync.Mutex
	totalConfigs         int
	verbose              bool
	testedCount          int
	successCount         int
	failCount            int
	lastReportedProgress int
	progressActive       bool
	successfulLinks      []string
	out                  io.Writer
}

func NewReporter(totalConfigs int, verbose bool) *Reporter {
	return &Reporter{
		totalConfigs: totalConfigs,
		verbose:      verbose,
		out:          os.Stdout,
	}
}

// IsSavable reports whether a result qualifies for the results file,
// mirroring the architecture rule: successful HTTP 2xx with a raw link.
func IsSavable(res types.TestResult) bool {
	return res.Success && res.StatusCode >= 200 && res.StatusCode < 300 && res.Config.RawLink != ""
}

// AppendLinks appends raw proxy links to the results file, creating the
// parent directory on demand. It is the incremental-write primitive behind
// batched runs: each completed batch flushes only its own healthy links.
func AppendLinks(filename string, links []string) error {
	if len(links) == 0 {
		return nil
	}
	if dir := filepath.Dir(filename); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create results directory: %w", err)
		}
	}
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open results file %s: %w", filename, err)
	}
	defer f.Close()
	for _, link := range links {
		if _, err := f.WriteString(link + "\n"); err != nil {
			return fmt.Errorf("failed to append results to file %s: %w", filename, err)
		}
	}
	return nil
}
func (r *Reporter) clearProgressLine() {
	if r.progressActive && r.out != nil {
		fmt.Fprint(r.out, "\r\033[K")
		r.progressActive = false
	}
}

func (r *Reporter) Record(res types.TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.testedCount++
	if res.Success {
		r.successCount++
		if IsSavable(res) {
			r.successfulLinks = append(r.successfulLinks, res.Config.RawLink)
		}
		if r.out != nil {
			r.clearProgressLine()
			fmt.Fprintf(r.out, "[✓ SUCCESS] [#%d] [%s] %s - %v (HTTP %d)\n",
				res.Config.Index,
				res.Config.Protocol,
				res.Config.Target(),
				res.Latency,
				res.StatusCode,
			)
		}
	} else {
		r.failCount++
		if r.verbose && r.out != nil {
			errReason := "unknown"
			if res.Error != nil {
				errReason = res.Error.Error()
			}
			r.clearProgressLine()
			fmt.Fprintf(r.out, "[✗ FAILED] [#%d] [%s] %s - %s\n",
				res.Config.Index,
				res.Config.Protocol,
				res.Config.Target(),
				errReason,
			)
		}
	}

	if r.out != nil && (r.testedCount%100 == 0 || r.testedCount == r.totalConfigs) {
		r.printProgress()
	}
}

func (r *Reporter) printProgress() {
	if r.lastReportedProgress == r.testedCount && r.progressActive {
		return
	}
	r.lastReportedProgress = r.testedCount
	var pct float64
	if r.totalConfigs > 0 {
		pct = (float64(r.testedCount) / float64(r.totalConfigs)) * 100.0
	}
	fmt.Fprintf(r.out, "\rProgress: %.1f%% (%d/%d tested)", pct, r.testedCount, r.totalConfigs)
	r.progressActive = true
}

func (r *Reporter) SaveResults() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	r.clearProgressLine()

	// Ensure results directory exists
	resultsDir := "results"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create results directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(resultsDir, fmt.Sprintf("results_%s.md", timestamp))

	var sb strings.Builder
	for _, link := range r.successfulLinks {
		sb.WriteString(link)
		sb.WriteString("\n")
	}

	err := os.WriteFile(filename, []byte(sb.String()), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to save results to file %s: %w", filename, err)
	}

	fmt.Fprintf(out, "Saved %d HTTP 2xx configuration(s) to %s\n", len(r.successfulLinks), filename)
	return filename, nil
}

func (r *Reporter) PrintSummary(duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if r.testedCount != r.lastReportedProgress && r.testedCount > 0 {
		r.printProgress()
	}
	if r.progressActive {
		fmt.Fprintln(out)
		r.progressActive = false
	}

	// Format duration cleanly
	durStr := formatDuration(duration)

	fmt.Fprintln(out, "==================================================")
	fmt.Fprintln(out, "TEST SUMMARY")
	fmt.Fprintln(out, "==================================================")
	fmt.Fprintf(out, "Total configs: %d\n", r.totalConfigs)
	fmt.Fprintf(out, "Tested:        %d\n", r.testedCount)
	fmt.Fprintf(out, "Success:       %d\n", r.successCount)
	fmt.Fprintf(out, "Failed:        %d\n", r.failCount)
	fmt.Fprintf(out, "Duration:      %s\n", durStr)
	fmt.Fprintln(out, "==================================================")
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
}
