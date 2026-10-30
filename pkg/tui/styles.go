package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Mission-control palette: warm orange accents with green for healthy results
// and red for failures, tuned to read on both dark and light terminals.
var (
	accentColor = lipgloss.Color("208") // orange
	softColor   = lipgloss.Color("179") // muted orange
	okColor     = lipgloss.Color("42")  // green
	errColor    = lipgloss.Color("203") // red
	warnColor   = lipgloss.Color("214") // amber
	labelColor  = lipgloss.Color("244") // dim label
	valueColor  = lipgloss.Color("252") // light value
	borderColor = lipgloss.Color("238") // panel border
	titleColor  = lipgloss.Color("245") // panel title
)

var (
	titleStyle  = lipgloss.NewStyle().Foreground(titleColor).Bold(true)
	labelStyle  = lipgloss.NewStyle().Foreground(labelColor).Bold(true)
	valueStyle  = lipgloss.NewStyle().Foreground(valueColor).Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	softStyle   = lipgloss.NewStyle().Foreground(softColor)
	okStyle     = lipgloss.NewStyle().Foreground(okColor)
	errStyle    = lipgloss.NewStyle().Foreground(errColor)
	warnStyle   = lipgloss.NewStyle().Foreground(warnColor)
	dimStyle    = lipgloss.NewStyle().Foreground(labelColor)
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

// twoColumnMin is the frame width below which the dashboard stacks its columns
// vertically instead of placing them side by side.
const twoColumnMin = 72

// frameWidth returns the layout width, falling back to a sane default before
// the first window-size event and clamping extreme terminal sizes.
func (m Model) frameWidth() int {
	w := m.width
	switch {
	case w <= 0:
		w = 80
	case w > 120:
		w = 120
	case w < 40:
		w = 40
	}
	return w
}

// innerWidth is the usable content width inside a panel of the given total width
// (border 2 + horizontal padding 2).
func innerWidth(total int) int {
	if total-4 < 4 {
		return 4
	}
	return total - 4
}

// box draws a rounded panel of the given total width (border included). An
// optional height pads the panel so side-by-side columns line up; a title is
// rendered as the first line. Note: lipgloss Width includes padding, so the
// style width is total-2 (border adds the remaining 2).
func box(title string, width, height int, lines ...string) string {
	if width < 12 {
		width = 12
	}
	content := make([]string, 0, len(lines)+2)
	if title != "" {
		content = append(content, titleStyle.Render(strings.ToUpper(title)))
	}
	if title != "" && len(lines) > 0 {
		content = append(content, "")
	}
	content = append(content, lines...)

	body := lipgloss.JoinVertical(lipgloss.Left, content...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(width - 2)
	if height > 0 {
		style = style.Height(height)
	}
	return style.Render(body)
}

// header renders the dashboard top bar: brand on the left, status on the right.
func (m Model) header(width int, right, sub string) string {
	inner := innerWidth(width)
	brand := titleStyle.Render(":: V2PINGO")
	gap := inner - lipgloss.Width(brand) - lipgloss.Width(right)
	line := brand
	if gap >= 1 {
		line += strings.Repeat(" ", gap) + right
	} else {
		line += " " + right
	}
	if sub != "" {
		line += "\n" + dimStyle.Render(clip(sub, inner))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1).
		Width(width - 2).
		Render(line)
}

// kv renders an aligned "LABEL   value" row clipped to the content width.
func kv(label, value string, contentW int) string {
	pad := 9 // 8-wide label column + one space
	avail := contentW - pad
	if avail < 4 {
		avail = 4
	}
	return labelStyle.Render(fmt.Sprintf("%-8s", label)) + " " + valueStyle.Render(clip(value, avail))
}

// clip truncates a plain (uncolored) string to a display width, adding an
// ellipsis when it had to cut. It must only be used on text without ANSI codes.
func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	limit := width - 1
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > limit {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

// progressBar renders a filled/empty bar for a 0..1 fraction.
func progressBar(fraction float64, width int) string {
	if width < 4 {
		width = 4
	}
	filled := int(fraction*float64(width) + 0.5)
	switch {
	case filled > width:
		filled = width
	case filled < 0:
		filled = 0
	}
	return okStyle.Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", width-filled))
}

// humanCount formats a count with thousands separators (3201 -> "3,201").
func humanCount(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// humanDuration formats a duration compactly for the metric cards.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, mins)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %02ds", mins, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// metricCard renders one small labelled stat card.
func metricCard(label, value string, width int, style lipgloss.Style) string {
	return box(label, width, 0, style.Render(clip(value, innerWidth(width)-2)+" "))
}

// columnWidths returns the panel widths for the two-column layout; stacked is
// true when the frame is too narrow and both panels should span the full width.
func columnWidths(width int) (leftW, rightW int, stacked bool) {
	if width < twoColumnMin {
		return width, width, true
	}
	leftW = width * 42 / 100
	if leftW < 30 {
		leftW = 30
	}
	rightW = width - leftW - 1
	if rightW < 30 {
		rightW = 30
		leftW = width - rightW - 1
	}
	return leftW, rightW, false
}

// joinColumns places the two panels side by side (equal height) or stacks them
// when the frame is too narrow.
func joinColumns(leftW, rightW int, stacked bool, leftTitle string, left []string, rightTitle string, right []string) string {
	if stacked {
		return lipgloss.JoinVertical(lipgloss.Left,
			box(leftTitle, leftW, 0, left...),
			box(rightTitle, rightW, 0, right...),
		)
	}
	h := len(left)
	if len(right) > h {
		h = len(right)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(leftTitle, leftW, h, left...),
		" ",
		box(rightTitle, rightW, h, right...),
	)
}
