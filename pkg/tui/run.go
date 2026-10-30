package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/parser"
	"v2pingo/pkg/paste"
	"v2pingo/pkg/runner"
	"v2pingo/pkg/subscription"
	"v2pingo/pkg/types"
)

// Options holds run settings that are not part of the interactive form.
type Options struct {
	// AskPaste offers the "upload to paste.rs?" question on the done screen.
	// The upload only happens when the user answers yes, so a run can never
	// publish results without an explicit confirmation.
	AskPaste bool
	// PasteEndpoint overrides the paste service URL; empty means paste.rs.
	PasteEndpoint string
	// FetchMode decides whether the subscription sources are downloaded afresh
	// or reused from the cache directory.
	FetchMode subscription.FetchMode
	// CacheDir is the directory that keeps the fetched subscription sources; an
	// empty directory disables caching, so nothing is read from or written to
	// disk and every source is downloaded.
	CacheDir string
	// Types restricts the run to these canonical config types (from -types).
	// Empty means every type is tested and the TUI asks on the type-filter
	// screen; when set, that screen is skipped and the filter is applied
	// right after the fetch.
	Types []string
}

// PasteDoneMsg carries the outcome of an on-demand paste upload.
type PasteDoneMsg struct {
	Link string
	Err  error
}

// bound is the running program, set by Run before prog.Run() so Update can
// start the background goroutines when FetchSourcesMsg or StartTestsMsg
// arrives.
// boundCancel stops the in-flight run (set synchronously by startTests); the
// 'q' key and Ctrl+C in Update call it so the goroutine winds down and sends
// the single final DoneMsg, which moves the UI to the done screen and decides
// the paste question.
var bound *tea.Program

var boundCancel context.CancelFunc

func bindProgram(p *tea.Program) { bound = p }

// Run starts the interactive TUI with CLI flag values as form defaults.
func Run(subs []string, target string, workers int, timeout time.Duration, opts Options) error {
	m := NewModel(subs, target, workers, timeout)
	m.opts = opts
	prog := tea.NewProgram(m)
	bindProgram(prog)
	defer func() {
		bound = nil
		boundCancel = nil
	}()
	_, err := prog.Run()
	return err
}

// startFetch downloads (or reads cached) subscription sources off the UI
// thread and forwards warnings as NoticeMsg. It then either hands the links to
// the type-filter screen via TypesFoundMsg, or — when -types already decided
// the filter — calls startTests directly so the selector screen is skipped.
func startFetch(prog *tea.Program, cfg FormConfig, opts Options) {
	go func() {
		start := time.Now()
		client := &http.Client{Timeout: 30 * time.Second}
		fctx, fcancel := context.WithTimeout(context.Background(), 60*time.Second)
		// Fetch warnings must never reach the console directly: while the
		// dashboard is on screen a stray write corrupts the rendered frame, so
		// they are forwarded into the BubbleTea loop and shown by the UI itself.
		rawLinks := subscription.FetchSubscriptionsWithWarn(fctx, cfg.Subs, client, opts.CacheDir, opts.FetchMode, func(format string, args ...any) {
			prog.Send(NoticeMsg{Text: fmt.Sprintf(format, args...)})
		})
		fcancel()
		if len(rawLinks) == 0 {
			prog.Send(DoneMsg{Notice: "No valid configs", Elapsed: time.Since(start)})
			return
		}
		if len(opts.Types) > 0 {
			prog.Send(NoticeMsg{Text: "type filter: " + strings.Join(opts.Types, ", ")})
			startTests(prog, cfg, opts, rawLinks, opts.Types)
			return
		}
		prog.Send(TypesFoundMsg{Counts: parser.CountByType(rawLinks), Links: rawLinks})
	}()
}

// startTests filters the raw links down to types, then runs the batched test
// pool off the UI thread exactly as before: at most one 5k batch in RAM,
// healthy links appended to the results file batch by batch, a single final
// DoneMsg. The run context is created here — synchronously before the
// goroutine starts — so boundCancel is stored before any TotalMsg can flip the
// screen to running and make q reach the cancel func; the selector-Enter path
// calls this from the Update loop itself, same as the old single-goroutine
// design.
func startTests(prog *tea.Program, cfg FormConfig, opts Options, rawLinks []string, typesFilter []string) {
	ctx, cancel := context.WithCancel(context.Background())
	boundCancel = cancel
	go func() {
		start := time.Now()
		defer cancel()
		filtered := parser.FilterByTypes(rawLinks, typesFilter)
		if len(filtered) == 0 {
			prog.Send(DoneMsg{Notice: "no configs of the selected types", Elapsed: time.Since(start)})
			return
		}
		resultsFile := runner.DefaultResultsFile()
		checkpointFile := resultsFile + ".checkpoint"
		prog.Send(TotalMsg{Total: len(filtered)})
		col := &Collector{}
		runErr := runner.RunBatches(ctx, filtered, runner.BatchConfig{
			TargetURL:      cfg.TargetURL,
			Concurrency:    cfg.Workers,
			Timeout:        cfg.Timeout,
			BatchSize:      runner.DefaultBatchSize,
			ResultsFile:    resultsFile,
			CheckpointFile: checkpointFile,
		}, func(res types.TestResult) {
			col.OnResult(res)
			prog.Send(ResultMsg{Res: res})
		})
		// A cancelled run ends with the user's partial DoneMsg; any other batch
		// error is surfaced on the done screen instead of being dropped.
		notice := ""
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			notice = "run stopped: " + runErr.Error()
		}
		tested, okCount, failCount, links := col.Snapshot()
		prog.Send(DoneMsg{Tested: tested, Success: okCount, Failed: failCount, Links: links, SavedFile: resultsFile, Elapsed: time.Since(start), Notice: notice})
	}()
}

func uploadResults(path, endpoint string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		link, err := paste.NewClient(endpoint).UploadFile(ctx, path)
		return PasteDoneMsg{Link: link, Err: err}
	}
}
