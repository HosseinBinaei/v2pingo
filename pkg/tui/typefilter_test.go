package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/parser"
)

// formToTypeFilter presses Enter on a valid form and returns the loading
// type-filter model plus the emitted FetchSourcesMsg.
func formToTypeFilter(t *testing.T) (Model, FormConfig) {
	t.Helper()
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.screen != ScreenTypeFilter || !got.typeLoading {
		t.Fatalf("Enter on the form must open the type filter in its loading state, got screen=%v loading=%v", got.screen, got.typeLoading)
	}
	if cmd == nil {
		t.Fatal("Enter on the form must emit the fetch trigger")
	}
	msg, ok := cmd().(FetchSourcesMsg)
	if !ok {
		t.Fatalf("expected FetchSourcesMsg, got %T", cmd())
	}
	return got, msg.Cfg
}

// loadedTypeFilter returns a type-filter model populated as the fetch
// goroutine would: vmess(2), vless(1), socks(3), all checked.
func loadedTypeFilter(t *testing.T) Model {
	t.Helper()
	m, _ := formToTypeFilter(t)
	next, _ := m.Update(TypesFoundMsg{
		Counts: []parser.TypeCount{
			{Type: "vmess", Count: 2},
			{Type: "vless", Count: 1},
			{Type: "socks", Count: 3},
		},
		Links: []string{"vmess://a", "vmess://b", "vless://c", "1.1.1.1:1080", "1.1.1.2:1080", "socks5://d"},
	})
	got := next.(Model)
	if got.typeLoading {
		t.Fatal("TypesFoundMsg must end the loading state")
	}
	return got
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestTypeFilter_ShowsCountsAndChecksAll(t *testing.T) {
	m := loadedTypeFilter(t)
	if len(m.typeChecked) != 3 {
		t.Fatalf("expected one checked flag per type, got %d", len(m.typeChecked))
	}
	for i, c := range m.typeChecked {
		if !c {
			t.Fatalf("type %d must start checked", i)
		}
	}
	view := m.View()
	for _, want := range []string{"vmess", "vless", "socks", "[x]", "TYPE FILTER"} {
		if !strings.Contains(view, want) {
			t.Errorf("type-filter view must show %q, got:\n%s", want, view)
		}
	}
}

func TestTypeFilter_KeysNavigateAndToggle(t *testing.T) {
	m := loadedTypeFilter(t)
	down, _ := m.Update(key("j"))
	if got := down.(Model).typeCursor; got != 1 {
		t.Fatalf("j must move the cursor down, got %d", got)
	}
	up, _ := down.(Model).Update(key("up"))
	if got := up.(Model).typeCursor; got != 0 {
		t.Fatalf("up must move the cursor up, got %d", got)
	}
	toggled, _ := up.(Model).Update(key(" "))
	if got := toggled.(Model).typeChecked[0]; got {
		t.Fatal("space must uncheck the cursor type")
	}

	// From all checked (m), a unchecks everything; from none, a checks all.
	none, _ := m.Update(key("a"))
	for i, c := range none.(Model).typeChecked {
		if c {
			t.Fatalf("a with all checked must uncheck all, index %d still checked", i)
		}
	}
	all, _ := none.(Model).Update(key("a"))
	for i, c := range all.(Model).typeChecked {
		if !c {
			t.Fatalf("a with none checked must check all, index %d unchecked", i)
		}
	}
	// From a partial selection (vmess unchecked above), a checks everything.
	partial, _ := toggled.(Model).Update(key("a"))
	for i, c := range partial.(Model).typeChecked {
		if !c {
			t.Fatalf("a with a partial selection must check all, index %d unchecked", i)
		}
	}
}

func TestTypeFilter_EnterWithNothingCheckedShowsError(t *testing.T) {
	m := loadedTypeFilter(t)
	none, _ := m.Update(key("a"))
	next, cmd := none.(Model).Update(key("enter"))
	got := next.(Model)
	if got.screen != ScreenTypeFilter || cmd != nil {
		t.Fatalf("Enter with zero types must stay on the filter screen and emit nothing, got screen=%v cmd=%v", got.screen, cmd != nil)
	}
	if got.typeErr == "" || !strings.Contains(got.View(), got.typeErr) {
		t.Fatalf("Enter with zero types must show an inline error, got %q", got.typeErr)
	}
}

func TestTypeFilter_EnterStartsTestsWithSelection(t *testing.T) {
	m := loadedTypeFilter(t)
	// Uncheck vmess (cursor 0), keep vless + socks.
	one, _ := m.Update(key(" "))
	next, cmd := one.(Model).Update(key("enter"))
	got := next.(Model)
	if got.screen != ScreenRunning {
		t.Fatalf("Enter must start the run, got screen=%v", got.screen)
	}
	if cmd == nil {
		t.Fatal("Enter must emit the test trigger")
	}
	msg, ok := cmd().(StartTestsMsg)
	if !ok {
		t.Fatalf("expected StartTestsMsg, got %T", cmd())
	}
	if len(msg.Types) != 2 || msg.Types[0] != "vless" || msg.Types[1] != "socks" {
		t.Fatalf("selection must carry the checked types in order, got %v", msg.Types)
	}
	if msg.Cfg.TargetURL != "https://cloudflare.com" {
		t.Fatalf("StartTestsMsg must carry the form config, got %+v", msg.Cfg)
	}
}

func TestTypeFilter_LoadingIgnoresKeysButCtrlCQuits(t *testing.T) {
	m, _ := formToTypeFilter(t)
	ignored, cmd := m.Update(key("enter"))
	if got := ignored.(Model); got.screen != ScreenTypeFilter || cmd != nil {
		t.Fatal("Enter while loading must be ignored")
	}
	_, cmd = m.Update(key("ctrl+c"))
	if cmd == nil {
		t.Fatal("Ctrl+C while loading must quit")
	}
}

func TestTypeFilter_DoneMsgFromFilterScreenReachesDone(t *testing.T) {
	m, _ := formToTypeFilter(t)
	next, _ := m.Update(DoneMsg{Notice: "No valid configs"})
	if got := next.(Model); got.screen != ScreenDone {
		t.Fatalf("expected the done screen, got %v", got.screen)
	}
}

func TestTypeFilter_TotalMsgSwitchesToRunningScreen(t *testing.T) {
	// The -types flag path never shows the selector: the tests start straight
	// from the fetch and the first TotalMsg must flip the screen to running.
	m, _ := formToTypeFilter(t)
	next, _ := m.Update(TotalMsg{Total: 4})
	got := next.(Model)
	if got.screen != ScreenRunning || got.total != 4 {
		t.Fatalf("TotalMsg on the filter screen must switch to running, got screen=%v total=%d", got.screen, got.total)
	}
	// Idempotent for the normal path: a second TotalMsg must not reset state.
	next, _ = got.Update(TotalMsg{Total: 4})
	if got := next.(Model); got.screen != ScreenRunning {
		t.Fatalf("TotalMsg on the running screen must keep it, got %v", got.screen)
	}
}

func TestTypeFilter_SelectedTypesHelper(t *testing.T) {
	m := loadedTypeFilter(t)
	if got := m.selectedTypes(); len(got) != 3 {
		t.Fatalf("all checked must select all types, got %v", got)
	}
	one, _ := m.Update(key(" "))
	if got := one.(Model).selectedTypes(); len(got) != 2 || got[0] != "vless" {
		t.Fatalf("unchecking vmess must leave vless+socks, got %v", got)
	}
}

func TestHumanCount(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 3201: "3,201", 1234567: "1,234,567"}
	for in, want := range cases {
		if got := humanCount(in); got != want {
			t.Errorf("humanCount(%d) = %q, want %q", in, got, want)
		}
	}
}
