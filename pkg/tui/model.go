package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

// DoneMsg ends the run with final counters and the saved-file path.
type DoneMsg struct {
	Tested    int
	Success   int
	Failed    int
	Links     []string
	SavedFile string
	Elapsed   time.Duration
}

// StartTestsMsg triggers the background fetch+test goroutine.
type StartTestsMsg struct{ Cfg FormConfig }

// Model is the single BubbleTea model for form, running, and done screens.
type Model struct {
	screen    Screen
	inputs    []textinput.Model
	focus     int
	formErr   string
	subs      []string
	total     int
	tested    int
	succeeded int
	failed    int
	links     []string
	recent    []string
	savedFile string
	elapsed   time.Duration
	start     time.Time
	spinner   spinner.Model
	viewport  viewport.Model
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
		switch msg.Type {
		case tea.KeyCtrlC:
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
		return m, cmd
	case StartTestsMsg:
		if bound != nil {
			startBackgroundRun(bound, msg.Cfg)
		}
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
		return m, nil
	}
	return m, nil
}

// View renders the active screen. Full styling is a follow-up; this keeps
// the state machine testable and the screens readable.
func (m Model) View() string {
	switch m.screen {
	case ScreenRunning:
		return fmt.Sprintf("Testing %d/%d  ok=%d fail=%d\n%s", m.tested, m.total, m.succeeded, m.failed, m.viewport.View())
	case ScreenDone:
		return fmt.Sprintf("Done: tested=%d success=%d failed=%d in %s\nSaved: %s\n(Enter=form q=quit)", m.tested, m.succeeded, m.failed, m.elapsed.Round(time.Second), m.savedFile)
	default:
		var b strings.Builder
		b.WriteString("v2pingo — Tab moves, Enter starts, Ctrl+C quits\n")
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
