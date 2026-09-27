package tui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/parser"
	"v2pingo/pkg/subscription"
	"v2pingo/pkg/tester"
	"v2pingo/pkg/types"
)

// bound is the running program, set by Run before prog.Run() so Update can
// start the background test goroutine when StartTestsMsg arrives.
// boundCancel stops the in-flight run (set by startBackgroundRun); the 'q'
// key in Update calls it so the goroutine below can send a partial DoneMsg.
var bound *tea.Program

var boundCancel context.CancelFunc

func bindProgram(p *tea.Program) { bound = p }

// Run starts the interactive TUI with CLI flag values as form defaults.
func Run(subs []string, target string, workers int, timeout time.Duration) error {
	m := NewModel(subs, target, workers, timeout)
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
func startBackgroundRun(prog *tea.Program, cfg FormConfig) {
	go func() {
		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		boundCancel = cancel
		client := &http.Client{Timeout: 30 * time.Second}
		fctx, fcancel := context.WithTimeout(ctx, 60*time.Second)
		rawLinks := subscription.FetchSubscriptions(fctx, cfg.Subs, client)
		fcancel()
		configs := parser.DeduplicateConfigs(rawLinks)
		prog.Send(TotalMsg{Total: len(configs)})
		col := &Collector{}
		if len(configs) == 0 {
			prog.Send(DoneMsg{Notice: "No valid configs", Elapsed: time.Since(start)})
			return
		}
		tester.RunPool(ctx, configs, cfg.TargetURL, cfg.Workers, cfg.Timeout, func(res types.TestResult) {
			col.OnResult(res)
			prog.Send(ResultMsg{Res: res})
		})
		tested, okCount, failCount, links := col.Snapshot()
		saved := ""
		if len(links) > 0 {
			_ = os.MkdirAll("results", 0755)
			saved = filepath.Join("results", fmt.Sprintf("results_%s.md", time.Now().Format("20060102_150405")))
			_ = os.WriteFile(saved, []byte(strings.Join(links, "\n")+"\n"), 0644)
		}
		prog.Send(DoneMsg{Tested: tested, Success: okCount, Failed: failCount, Links: links, SavedFile: saved, Elapsed: time.Since(start)})
	}()
}
