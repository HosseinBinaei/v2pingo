package tui

import (
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
