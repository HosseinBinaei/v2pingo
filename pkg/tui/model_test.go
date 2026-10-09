package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/types"
)

func TestModel_ResultMsgUpdatesCounters(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(ResultMsg{Res: types.TestResult{
		Config:     types.ProxyConfig{RawLink: "vless://a#1"},
		Success:    true,
		StatusCode: 201,
	}})
	got := next.(Model)
	if got.tested != 1 || got.succeeded != 1 {
		t.Fatalf("expected 1 tested / 1 success, got %+v", got)
	}
	if len(got.links) != 1 {
		t.Fatalf("expected 201 link kept, got %+v", got.links)
	}
}

func TestModel_Non2xxNotSavedButCounted(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(ResultMsg{Res: types.TestResult{
		Config:     types.ProxyConfig{RawLink: "vless://a#x"},
		Success:    true,
		StatusCode: 404,
	}})
	got := next.(Model)
	if got.tested != 1 || got.succeeded != 1 || len(got.links) != 0 {
		t.Fatalf("404 must count as success but save nothing, got %+v", got)
	}
}

func TestModel_DoneMsgSwitchesToDoneScreen(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(DoneMsg{Tested: 5, Success: 2, Failed: 3, SavedFile: "results/x.md"})
	if next.(Model).screen != ScreenDone {
		t.Fatalf("expected ScreenDone after DoneMsg")
	}
}

func TestModel_QuitKeyQuits(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected a quit command on Ctrl+C")
	}
}

func TestModel_TotalMsgSetsTotalAndSurvivesResults(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(TotalMsg{Total: 5})
	if got := next.(Model); got.total != 5 {
		t.Fatalf("expected total=5 after TotalMsg, got %+v", got)
	}
	next2, _ := next.(Model).Update(ResultMsg{Res: types.TestResult{
		Config:     types.ProxyConfig{RawLink: "vless://a#1"},
		Success:    true,
		StatusCode: 200,
	}})
	if got := next2.(Model); got.total != 5 || got.tested != 1 {
		t.Fatalf("ResultMsg must keep total=5 and count tested=1, got %+v", got)
	}
}

func TestModel_QCancelsRunWithoutQuitting(t *testing.T) {
	cancelled := false
	boundCancel = func() { cancelled = true }
	defer func() { boundCancel = nil }()

	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	m.tested, m.succeeded, m.failed = 3, 2, 1
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if got := next.(Model); got.screen != ScreenRunning {
		t.Fatalf("q must leave the model on ScreenRunning (DoneMsg arrives via cmd), got %+v", got.screen)
	}
	if !cancelled {
		t.Fatalf("expected q to invoke the stored run cancel func")
	}
	if cmd == nil {
		t.Fatalf("expected a cmd emitting the partial DoneMsg")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); isQuit {
		t.Fatalf("q must not quit the program")
	}
	done, ok := msg.(DoneMsg)
	if !ok {
		t.Fatalf("expected DoneMsg from q cmd, got %T", msg)
	}
	if done.Tested != 3 || done.Success != 2 || done.Failed != 1 {
		t.Fatalf("partial DoneMsg must carry current counters, got %+v", done)
	}
}

func TestModel_DoneMsgShowsPasteLink(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(DoneMsg{Tested: 5, Success: 2, Failed: 3, SavedFile: "results/x.md", PasteLink: "https://paste.rs/abc"})
	got := next.(Model)
	if got.pasteLink != "https://paste.rs/abc" {
		t.Fatalf("expected paste link to be stored, got %q", got.pasteLink)
	}
	if !strings.Contains(got.View(), "https://paste.rs/abc") {
		t.Fatalf("expected paste link in done view, got %q", got.View())
	}
}

func TestModel_PartialDoneMsgShowsNotice(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(DoneMsg{Notice: "No valid configs"})
	got := next.(Model)
	if got.screen != ScreenDone {
		t.Fatalf("expected ScreenDone after partial DoneMsg")
	}
	if !strings.Contains(got.View(), "No valid configs") {
		t.Fatalf("expected notice in done view, got %q", got.View())
	}
}

func TestModel_CtrlCOnRunningCancelsInsteadOfQuitting(t *testing.T) {
	cancelled := false
	boundCancel = func() { cancelled = true }
	defer func() { boundCancel = nil }()

	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	m.tested, m.succeeded, m.failed = 3, 2, 1
	m.links = []string{"vless://a#1"}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected a cmd from Ctrl+C on the running screen")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); isQuit {
		t.Fatalf("Ctrl+C on the running screen must cancel (so partial results get saved), not quit the program")
	}
	done, ok := msg.(DoneMsg)
	if !ok {
		t.Fatalf("expected partial DoneMsg from Ctrl+C, got %T", msg)
	}
	if done.Tested != 3 || done.Success != 2 || done.Failed != 1 || len(done.Links) != 1 {
		t.Fatalf("partial DoneMsg must carry current counters and links, got %+v", done)
	}
	if !cancelled {
		t.Fatalf("expected Ctrl+C to invoke the stored run cancel func")
	}
}

func TestModel_QOnDoneQuits(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.screen = ScreenDone
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected a quit command from q on the done screen (the view advertises q=quit)")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected tea.QuitMsg from q on the done screen")
	}
}
