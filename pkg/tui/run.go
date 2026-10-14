package tui

import (
	"context"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
}

// PasteDoneMsg carries the outcome of an on-demand paste upload.
type PasteDoneMsg struct {
	Link string
	Err  error
}

// bound is the running program, set by Run before prog.Run() so Update can
// start the background test goroutine when StartTestsMsg arrives.
// boundCancel stops the in-flight run (set by startBackgroundRun); the 'q'
// key in Update calls it so the goroutine below can send a partial DoneMsg.
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

// startBackgroundRun executes fetch -> dedupe -> RunPool off the UI thread
// and forwards every result with prog.Send (goroutine-safe in BubbleTea). The
// subscription sources come from the cache directory or from GitHub according
// to opts.FetchMode, which the caller resolved before the UI started.
func startBackgroundRun(prog *tea.Program, cfg FormConfig, opts Options) {
	go func() {
		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		boundCancel = cancel
		client := &http.Client{Timeout: 30 * time.Second}
		fctx, fcancel := context.WithTimeout(ctx, 60*time.Second)
		rawLinks := subscription.FetchSubscriptions(fctx, cfg.Subs, client, opts.CacheDir, opts.FetchMode)
		fcancel()
		resultsFile := runner.DefaultResultsFile()
		checkpointFile := resultsFile + ".checkpoint"
		prog.Send(TotalMsg{Total: len(rawLinks)})
		col := &Collector{}
		if len(rawLinks) == 0 {
			prog.Send(DoneMsg{Notice: "No valid configs", Elapsed: time.Since(start)})
			return
		}
		// Batched run: at most one 5k batch of configs in RAM; healthy links
		// are appended to resultsFile after each batch with resume support.
		_ = runner.RunBatches(ctx, rawLinks, runner.BatchConfig{
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
		tested, okCount, failCount, links := col.Snapshot()
		prog.Send(DoneMsg{Tested: tested, Success: okCount, Failed: failCount, Links: links, SavedFile: resultsFile, Elapsed: time.Since(start)})
	}()
}

// uploadResults returns a command that publishes path to the paste service off
// the UI thread. The user triggers it explicitly from the done screen; a
// missing or empty file resolves to an empty link and no error.
func uploadResults(path, endpoint string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		link, err := paste.NewClient(endpoint).UploadFile(ctx, path)
		return PasteDoneMsg{Link: link, Err: err}
	}
}
