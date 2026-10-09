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
	// Paste uploads the saved results file to the paste service when the run
	// finishes, and surfaces the link on the done screen.
	Paste bool
	// PasteEndpoint overrides the paste service URL; empty means paste.rs.
	PasteEndpoint string
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
// and forwards every result with prog.Send (goroutine-safe in BubbleTea).
func startBackgroundRun(prog *tea.Program, cfg FormConfig, opts Options) {
	go func() {
		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		boundCancel = cancel
		client := &http.Client{Timeout: 30 * time.Second}
		fctx, fcancel := context.WithTimeout(ctx, 60*time.Second)
		rawLinks := subscription.FetchSubscriptions(fctx, cfg.Subs, client)
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
		pasteLink, pasteErr := publishResults(opts, resultsFile)
		notice := ""
		if pasteErr != nil {
			notice = "paste upload failed: " + pasteErr.Error()
		}
		prog.Send(DoneMsg{Tested: tested, Success: okCount, Failed: failCount, Links: links, SavedFile: resultsFile, PasteLink: pasteLink, Elapsed: time.Since(start), Notice: notice})
	}()
}

// publishResults uploads the saved results file when publishing is enabled and
// returns the paste link. A missing or empty results file yields an empty link
// and no error, so an unproductive run is not reported as an upload failure.
func publishResults(opts Options, resultsFile string) (string, error) {
	if !opts.Paste {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return paste.NewClient(opts.PasteEndpoint).UploadFile(ctx, resultsFile)
}
