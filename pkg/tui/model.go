package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// formLabels names the four form fields, in focus order.
var formLabels = []string{
	"Extra subscription URL",
	"Target URL",
	"Workers (1-200)",
	"Timeout",
}

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

// NoticeMsg carries a non-fatal problem from the background run — a
// subscription source that failed to download, a cache file that could not be
// written — onto the running screen. The background goroutine must never print
// such messages itself: a console write behind the UI's back corrupts the
// rendered frame.
type NoticeMsg struct{ Text string }

// StartTestsMsg triggers the background fetch+test goroutine.
type StartTestsMsg struct{ Cfg FormConfig }

// Model is the single BubbleTea model for form, running, and done screens.
type Model struct {
	screen         Screen
	inputs         []textinput.Model
	focus          int
	formErr        string
	subs           []string
	cfg            FormConfig
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
	warning        string
	opts           Options
	start          time.Time
	spinner        spinner.Model
	width          int
	height         int
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
		ti.Width = 48
		if i == 0 {
			ti.Focus()
		}
		m.inputs = append(m.inputs, ti)
	}
	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.Dot
	return m
}

// Update routes messages and keys to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
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
				m.cfg = cfg
				m.screen = ScreenRunning
				m.start = time.Now()
				m.warning = "" // a fresh run starts with a clean warning line
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
	case NoticeMsg:
		m.warning = msg.Text
		return m, nil
	case ResultMsg:
		m.tested++
		if msg.Res.Success {
			m.succeeded++
			if msg.Res.StatusCode >= 200 && msg.Res.StatusCode < 300 && msg.Res.Config.RawLink != "" {
				m.links = append(m.links, msg.Res.Config.RawLink)
				m.recent = append(m.recent, m.resultLine(msg.Res))
				if len(m.recent) > 8 {
					m.recent = m.recent[len(m.recent)-8:]
				}
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

// resultLine renders one live-log row. It deliberately shows only protocol,
// host:port, latency and status — never the share link — so credentials stay out
// of the terminal, matching the CLI reporter's sanitized output.
func (m Model) resultLine(res types.TestResult) string {
	return okStyle.Render("✓ ") +
		valueStyle.Render(fmt.Sprintf("%-10s", res.Config.Protocol)) +
		dimStyle.Render(res.Config.Target()) + "  " +
		softStyle.Render(fmt.Sprintf("%6s", res.Latency.Round(time.Millisecond))) + "  " +
		okStyle.Render(fmt.Sprintf("HTTP %d", res.StatusCode))
}

// View renders the active screen.
func (m Model) View() string {
	switch m.screen {
	case ScreenRunning:
		return m.viewRunning()
	case ScreenDone:
		return m.viewDone()
	default:
		return m.viewForm()
	}
}

// Init starts the cursor blink and the dashboard spinner.
func (m Model) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.spinner.Tick) }

// viewForm renders the configuration screen.
func (m Model) viewForm() string {
	w := m.frameWidth()
	content := innerWidth(w)

	lines := make([]string, 0, len(m.inputs)+2)
	for i := range m.inputs {
		in := m.inputs[i]
		in.Width = content - 27
		if in.Width < 10 {
			in.Width = 10
		}
		cursor := "  "
		if i == m.focus {
			cursor = accentStyle.Render("▸ ")
		}
		lines = append(lines, cursor+labelStyle.Render(fmt.Sprintf("%-24s", formLabels[i]))+in.View())
	}
	if m.formErr != "" {
		lines = append(lines, "", errStyle.Render("✖ "+m.formErr))
	}

	subs := fmt.Sprintf("%d subscription URL(s) preloaded", len(m.subs))
	if len(m.subs) == 0 {
		subs = "no subscription URL preloaded"
	}

	return strings.Join([]string{
		m.header(w, accentStyle.Render("● SETUP"), "configure a new test run"),
		box("Sources", w, 0, dimStyle.Render(clip(m.sourceLine(), content)), hintStyle.Render(clip(subs, content))),
		box("Parameters", w, 0, lines...),
		hintStyle.Render("Tab = next field · Enter = start run · Ctrl+C = quit"),
	}, "\n")
}

// viewRunning renders the live mission-control screen.
func (m Model) viewRunning() string {
	w := m.frameWidth()

	leftW, rightW, stacked := columnWidths(w)
	left := m.statusLines(innerWidth(leftW))
	right := m.progressLines(innerWidth(rightW))

	lines := []string{
		m.header(w, accentStyle.Render("● RUNNING"), "mission control · "+m.cfg.TargetURL),
		joinColumns(leftW, rightW, stacked, "Status", left, "Progress", right),
		m.metricRow(w),
		box("Live results", w, 0, m.logLines()...),
	}
	if m.warning != "" {
		lines = append(lines, warnStyle.Render("⚠ "+clip(m.warning, innerWidth(w)-2)))
	}
	lines = append(lines, hintStyle.Render("Ctrl+C / q = stop and save partial results"))
	return strings.Join(lines, "\n")
}

// statusLines builds the left panel of the running dashboard.
func (m Model) statusLines(contentW int) []string {
	return []string{
		accentStyle.Render("▌ RUNNING ") + m.spinner.View(),
		"",
		accentStyle.Render(fmt.Sprintf("%d / %d", m.tested, m.total)) + "  " + labelStyle.Render("CONFIGS"),
		"",
		kv("TARGET", m.cfg.TargetURL, contentW),
		kv("WORKERS", fmt.Sprintf("%d", m.cfg.Workers), contentW),
		kv("TIMEOUT", m.cfg.Timeout.String(), contentW),
	}
}

// progressLines builds the right panel of the running dashboard.
func (m Model) progressLines(contentW int) []string {
	elapsed := m.elapsed
	if !m.start.IsZero() {
		elapsed = time.Since(m.start)
	}
	pct := 0.0
	if m.total > 0 {
		pct = float64(m.tested) / float64(m.total)
	}
	barW := contentW - 7
	if barW < 8 {
		barW = 8
	}
	return []string{
		progressBar(pct, barW) + "  " + accentStyle.Render(fmt.Sprintf("%3d%%", int(pct*100))),
		"",
		kv("SUCCESS", fmt.Sprintf("%d", m.succeeded), contentW),
		kv("FAILED", fmt.Sprintf("%d", m.failed), contentW),
		kv("ELAPSED", humanDuration(elapsed), contentW),
		kv("SAVED", fmt.Sprintf("%d", len(m.links)), contentW),
	}
}

// viewDone renders the completed-run dashboard.
func (m Model) viewDone() string {
	w := m.frameWidth()

	leftW, rightW, stacked := columnWidths(w)
	left := m.resultLines(innerWidth(leftW))
	right := m.breakdownLines(innerWidth(rightW))

	return strings.Join([]string{
		m.header(w, okStyle.Render("✔ COMPLETE"), "run finished · "+m.cfg.TargetURL),
		joinColumns(leftW, rightW, stacked, "Result", left, "Breakdown", right),
		m.metricRow(w),
		box("Share", w, 0, m.shareLines()...),
		hintStyle.Render(m.footerHint()),
	}, "\n")
}

// resultLines builds the left panel of the done dashboard.
func (m Model) resultLines(contentW int) []string {
	saved := "no results file (interrupted)"
	if m.savedFile != "" {
		saved = m.savedFile
	} else if m.notice != "" {
		saved = "none"
	}
	return []string{
		okStyle.Render("✔ COMPLETE"),
		"",
		okStyle.Render(fmt.Sprintf("%d / %d", m.succeeded, m.tested)) + "  " + labelStyle.Render("HEALTHY"),
		"",
		kv("TARGET", m.cfg.TargetURL, contentW),
		kv("SAVED", saved, contentW),
	}
}

// breakdownLines builds the right panel of the done dashboard.
func (m Model) breakdownLines(contentW int) []string {
	frac := m.successFraction()
	barW := contentW - 7
	if barW < 8 {
		barW = 8
	}
	return []string{
		progressBar(frac, barW) + "  " + okStyle.Render(fmt.Sprintf("%3d%%", int(frac*100))),
		"",
		kv("SUCCESS", fmt.Sprintf("%d", m.succeeded), contentW),
		kv("FAILED", fmt.Sprintf("%d", m.failed), contentW),
		kv("ELAPSED", humanDuration(m.elapsed), contentW),
		kv("LINKS", fmt.Sprintf("%d", len(m.links)), contentW),
	}
}

// metricRow renders the four stat cards shown on the running and done screens.
func (m Model) metricRow(w int) string {
	elapsed := m.elapsed
	if m.screen == ScreenRunning && !m.start.IsZero() {
		elapsed = time.Since(m.start)
	}
	cards := [4]struct {
		label string
		value string
		style lipgloss.Style
	}{
		{"Elapsed", humanDuration(elapsed), accentStyle},
		{"Workers", fmt.Sprintf("%d", m.cfg.Workers), accentStyle},
		{"Success", fmt.Sprintf("%d", m.succeeded), okStyle},
		{"Failed", fmt.Sprintf("%d", m.failed), errStyle},
	}

	if w < twoColumnMin {
		cw := (w - 1) / 2
		row1 := lipgloss.JoinHorizontal(lipgloss.Top,
			metricCard(cards[0].label, cards[0].value, cw, cards[0].style), " ",
			metricCard(cards[1].label, cards[1].value, cw, cards[1].style))
		row2 := lipgloss.JoinHorizontal(lipgloss.Top,
			metricCard(cards[2].label, cards[2].value, cw, cards[2].style), " ",
			metricCard(cards[3].label, cards[3].value, cw, cards[3].style))
		return lipgloss.JoinVertical(lipgloss.Left, row1, row2)
	}
	cw := (w - 3) / 4
	return lipgloss.JoinHorizontal(lipgloss.Top,
		metricCard(cards[0].label, cards[0].value, cw, cards[0].style), " ",
		metricCard(cards[1].label, cards[1].value, cw, cards[1].style), " ",
		metricCard(cards[2].label, cards[2].value, cw, cards[2].style), " ",
		metricCard(cards[3].label, cards[3].value, cw, cards[3].style))
}

// logLines returns the live result rows, or a placeholder before the first one.
func (m Model) logLines() []string {
	if len(m.recent) == 0 {
		return []string{dimStyle.Render("waiting for the first healthy configuration…")}
	}
	out := make([]string, 0, len(m.recent))
	out = append(out, m.recent...)
	return out
}

// shareLines renders the done-screen paste/notice panel. The exact prompt
// strings are preserved so the CLI and TUI stay in sync.
func (m Model) shareLines() []string {
	var lines []string
	switch {
	case m.pasteUploading:
		lines = append(lines, accentStyle.Render("↑ Uploading to paste.rs..."))
	case m.pastePrompt && m.pasteRetry:
		lines = append(lines, errStyle.Render("✖ "+m.pasteErr))
		lines = append(lines, "", warnStyle.Render(paste.RetryPrompt)+dimStyle.Render("(y = retry · any other key = give up · q = quit)"))
	case m.pastePrompt:
		lines = append(lines, valueStyle.Render(paste.Prompt)+dimStyle.Render("(y = upload · any other key = skip · q = quit)"))
	case m.pasteLink != "":
		lines = append(lines, okStyle.Render("✔ paste link: ")+valueStyle.Render(m.pasteLink))
	default:
		lines = append(lines, dimStyle.Render("result links stay on this machine"))
	}
	if m.notice != "" {
		lines = append(lines, "", dimStyle.Render(m.notice))
	}
	return lines
}

// successFraction is the share of tested configs that were healthy (0..1).
func (m Model) successFraction() float64 {
	if m.tested <= 0 {
		return 0
	}
	return float64(m.succeeded) / float64(m.tested)
}

// footerHint advertises the keys that are valid on the done screen.
func (m Model) footerHint() string {
	switch {
	case m.pastePrompt && m.pasteRetry:
		return "y = retry upload · any other key = give up · q = quit"
	case m.pastePrompt:
		return "y = upload · any other key = skip · q = quit"
	default:
		return "Enter = new run · q = quit"
	}
}

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
