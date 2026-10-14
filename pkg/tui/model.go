package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/paste"
	"v2pingo/pkg/subscription"
	"v2pingo/pkg/types"
)

// Screen is the visible TUI page.
type Screen int

const (
	ScreenForm Screen = iota
	ScreenRunning
	ScreenDone
)

// ResultMsg carries one finished proxy test into the BubbleTea loop.
type ResultMsg struct{ Res types.TestResult }

// DoneMsg ends the run with final counters and the saved-file path; the done
// screen uses them to decide whether to offer the paste upload question.
type DoneMsg struct {
	Tested    int
	Success   int
	Failed    int
	Links     []string
	SavedFile string
	Elapsed   time.Duration
	Notice    string
}

// TotalMsg carries the deduped config count so the running view can show
// "Testing x/total" while results stream in.
type TotalMsg struct{ Total int }

// StartTestsMsg triggers the background fetch+test goroutine.
type StartTestsMsg struct{ Cfg FormConfig }

// Model is the single BubbleTea model for form, running, and done screens.
type Model struct {
	screen         Screen
	inputs         []textinput.Model
	focus          int
	formErr        string
	subs           []string
	total          int
	tested         int
	succeeded      int
	failed         int
	links          []string
	recent         []string
	savedFile      string
	pasteLink      string
	pastePrompt    bool
	pasteRetry     bool
	pasteErr       string
	pasteUploading bool
	elapsed        time.Duration
	notice         string
	opts           Options
	start          time.Time
	spinner        spinner.Model
	viewport       viewport.Model
}

// NewModel builds the form screen. subs are the preloaded subscription lines
// (from sub-input.txt); target/workers/timeout are the CLI flag defaults.
func NewModel(subs []string, target string, workers int, timeout time.Duration) Model {
	m := Model{screen: ScreenForm, subs: subs}
	fields := []struct {
		placeholder string
		value       string
	}{
		{"extra sub URL (optional)", ""},
		{"target URL", target},
		{"workers 1-200", fmt.Sprintf("%d", workers)},
		{"timeout, e.g. 15s", timeout.String()},
	}
	for i, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.placeholder
		ti.SetValue(f.value)
		if i == 0 {
			ti.Focus()
		}
		m.inputs = append(m.inputs, ti)
	}
	m.spinner = spinner.New()
	m.viewport = viewport.New(60, 10)
	return m
}

// Update routes messages and keys to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// The done screen may be waiting for a y/N answer; resolve it first so
		// ordinary keys never fall through to navigation while it is pending.
		if m.screen == ScreenDone && m.pastePrompt {
			return m.resolvePastePrompt(msg)
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.screen == ScreenRunning {
				return m.cancelRun()
			}
			return m, tea.Quit
		case tea.KeyTab:
			if m.screen == ScreenForm {
				m.inputs[m.focus].Blur()
				m.focus = (m.focus + 1) % len(m.inputs)
				m.inputs[m.focus].Focus()
			}
			return m, nil
		case tea.KeyEnter:
			if m.screen == ScreenForm {
				cfg, errMsg := ValidateForm(m.subs, m.inputs[0].Value(), m.inputs[1].Value(), m.inputs[2].Value(), m.inputs[3].Value())
				if errMsg != "" {
					m.formErr = errMsg
					return m, nil
				}
				m.formErr = ""
				m.screen = ScreenRunning
				m.start = time.Now()
				return m, func() tea.Msg { return StartTestsMsg{Cfg: cfg} }
			}
			if m.screen == ScreenDone {
				m.screen = ScreenForm
				return m, nil
			}
			return m, nil
		}
		var cmd tea.Cmd
		if m.screen == ScreenForm {
			m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		}
		if msg.String() == "q" && m.screen == ScreenRunning {
			return m.cancelRun()
		}
		if msg.String() == "q" && m.screen == ScreenDone {
			return m, tea.Quit
		}
		return m, cmd
	case StartTestsMsg:
		if bound != nil {
			startBackgroundRun(bound, msg.Cfg, m.opts)
		}
		return m, nil
	case TotalMsg:
		m.total = msg.Total
		return m, nil
	case ResultMsg:
		m.tested++
		if msg.Res.Success {
			m.succeeded++
			if msg.Res.StatusCode >= 200 && msg.Res.StatusCode < 300 && msg.Res.Config.RawLink != "" {
				m.links = append(m.links, msg.Res.Config.RawLink)
				line := fmt.Sprintf("[ok] %s", strings.TrimSpace(msg.Res.Config.RawLink))
				m.recent = append(m.recent, line)
				if len(m.recent) > 10 {
					m.recent = m.recent[len(m.recent)-10:]
				}
				m.viewport.SetContent(strings.Join(m.recent, "\n"))
			}
			return m, nil
		}
		m.failed++
		return m, nil
	case DoneMsg:
		m.screen = ScreenDone
		m.tested, m.succeeded, m.failed = msg.Tested, msg.Success, msg.Failed
		m.links, m.savedFile, m.elapsed = msg.Links, msg.SavedFile, msg.Elapsed
		m.notice = msg.Notice
		m.pasteLink = ""
		m.pasteErr = ""
		m.pasteRetry = false
		m.pasteUploading = false
		// Only ask when the run actually wrote a results file with healthy
		// links; an interrupted partial DoneMsg carries no saved file.
		m.pastePrompt = m.opts.AskPaste && msg.SavedFile != "" && len(msg.Links) > 0
		return m, nil
	case PasteDoneMsg:
		m.pasteUploading = false
		if msg.Err != nil {
			// The upload failed: surface the reason and ask whether to try
			// again instead of silently dropping the shareable link.
			m.pasteErr = msg.Err.Error()
			m.pasteRetry = true
			m.pastePrompt = true
			return m, nil
		}
		m.pasteErr = ""
		m.pasteRetry = false
		m.pasteLink = msg.Link
		if msg.Link == "" {
			m.notice = "nothing to upload"
		}
		return m, nil
	}
	return m, nil
}

// resolvePastePrompt answers the done-screen question. Only an explicit yes
// uploads the results file; every other key keeps the run local, and q/Ctrl+C
// still quit the program. The same state machine answers the retry question
// shown after a failed upload: y tries again, anything else gives up.
func (m Model) resolvePastePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	yes := strings.EqualFold(msg.String(), "y")
	retrying := m.pasteRetry
	m.pastePrompt = false
	m.pasteRetry = false
	if !yes {
		if retrying {
			m.notice = "paste upload failed: " + m.pasteErr
		} else {
			m.notice = "paste upload skipped"
		}
		return m, nil
	}
	m.pasteUploading = true
	m.notice = ""
	return m, uploadResults(m.savedFile, m.opts.PasteEndpoint)
}

// cancelRun stops the in-flight run and emits a partial DoneMsg carrying the
// counters and links collected so far. The process stays alive so the
// background goroutine in run.go can finish and save the partial 2xx links
// to the results file — the same guarantee the CLI has on Ctrl+C.
func (m Model) cancelRun() (tea.Model, tea.Cmd) {
	if boundCancel != nil {
		boundCancel()
	}
	partial := DoneMsg{
		Tested:  m.tested,
		Success: m.succeeded,
		Failed:  m.failed,
		Links:   m.links,
		Elapsed: time.Since(m.start),
	}
	return m, func() tea.Msg { return partial }
}

// View renders the active screen. Full styling is a follow-up; this keeps
// the state machine testable and the screens readable.
func (m Model) View() string {
	switch m.screen {
	case ScreenRunning:
		return fmt.Sprintf("Testing %d/%d  ok=%d fail=%d  (Ctrl+C/q=cancel+save)\n%s", m.tested, m.total, m.succeeded, m.failed, m.viewport.View())
	case ScreenDone:
		var b strings.Builder
		fmt.Fprintf(&b, "Done: tested=%d success=%d failed=%d in %s\n", m.tested, m.succeeded, m.failed, m.elapsed.Round(time.Second))
		fmt.Fprintf(&b, "Saved: %s\n", m.savedFile)
		switch {
		case m.pasteUploading:
			b.WriteString("Uploading to paste.rs...\n")
		case m.pastePrompt && m.pasteRetry:
			b.WriteString("Paste upload failed: " + m.pasteErr + "\n")
			b.WriteString(paste.RetryPrompt + "\n")
		case m.pastePrompt:
			b.WriteString(paste.Prompt + "\n")
		case m.pasteLink != "":
			b.WriteString("Paste: " + m.pasteLink + "\n")
		}
		if m.notice != "" {
			b.WriteString(m.notice + "\n")
		}
		switch {
		case m.pastePrompt && m.pasteRetry:
			b.WriteString("(y = retry, any other key = give up, q = quit)")
		case m.pastePrompt:
			b.WriteString("(y = upload, any other key = skip, q = quit)")
		default:
			b.WriteString("(Enter=form q=quit)")
		}
		return b.String()
	default:
		var b strings.Builder
		b.WriteString("v2pingo — Tab moves, Enter starts, Ctrl+C quits\n")
		b.WriteString(m.sourceLine() + "\n")
		for _, in := range m.inputs {
			b.WriteString(in.View() + "\n")
		}
		if m.formErr != "" {
			b.WriteString("error: " + m.formErr + "\n")
		}
		return b.String()
	}
}

// Init starts the cursor blink for text inputs.
func (m Model) Init() tea.Cmd { return textinput.Blink }

// sourceLine tells the form where the subscription sources of the next run come
// from, so the question answered before the UI started stays visible.
func (m Model) sourceLine() string {
	if m.opts.FetchMode != subscription.FetchCache {
		return "sources: fresh download from the subscription URLs"
	}
	if m.opts.CacheDir == "" {
		return "sources: cached subscription files"
	}
	return "sources: cached files in " + m.opts.CacheDir
}
