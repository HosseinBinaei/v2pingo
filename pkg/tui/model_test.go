package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/paste"
	"v2pingo/pkg/subscription"
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

// newSavedRunModel returns a done-screen model that just finished a run which
// saved one healthy link, with paste asking enabled.
func newSavedRunModel() Model {
	m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	m.opts = Options{AskPaste: true}
	m.screen = ScreenRunning
	next, _ := m.Update(DoneMsg{Tested: 5, Success: 2, Failed: 3, SavedFile: "results/x.md", Links: []string{"vless://a#1"}})
	return next.(Model)
}

func TestModel_DoneMsgAsksBeforePasting(t *testing.T) {
	got := newSavedRunModel()
	if !got.pastePrompt {
		t.Fatalf("expected the paste question to be pending after a saved run")
	}
	if got.pasteLink != "" {
		t.Fatalf("no upload may happen before the user answers, got %q", got.pasteLink)
	}
	if !strings.Contains(got.View(), paste.Prompt) {
		t.Fatalf("expected the paste question in the done view, got %q", got.View())
	}
}

func TestModel_NonYesAnswerSkipsPasteUpload(t *testing.T) {
	for _, answer := range []string{"n", "N", "x", "1", " "} {
		next, cmd := newSavedRunModel().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(answer)})
		res := next.(Model)
		if res.pastePrompt {
			t.Errorf("answer %q must resolve the prompt", answer)
		}
		if res.pasteUploading || res.pasteLink != "" {
			t.Errorf("answer %q must not start an upload (uploading=%v link=%q)", answer, res.pasteUploading, res.pasteLink)
		}
		if cmd != nil {
			t.Errorf("answer %q must not return an upload command", answer)
		}
	}
}

func TestModel_YesAnswerStartsPasteUpload(t *testing.T) {
	for _, answer := range []string{"y", "Y"} {
		got := newSavedRunModel()
		next, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(answer)})
		res := next.(Model)
		if res.pastePrompt {
			t.Errorf("answer %q must resolve the prompt", answer)
		}
		if !res.pasteUploading {
			t.Errorf("answer %q must start the upload", answer)
		}
		if cmd == nil {
			t.Errorf("answer %q must return an upload command", answer)
		}
		if !strings.Contains(res.View(), "Uploading") {
			t.Errorf("expected an uploading state in the view, got %q", res.View())
		}
	}
}

func TestModel_PasteDoneMsgShowsLink(t *testing.T) {
	got := newSavedRunModel()
	pressed, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	next, _ := pressed.(Model).Update(PasteDoneMsg{Link: "https://paste.rs/abc"})
	res := next.(Model)
	if res.pasteUploading {
		t.Fatalf("upload must be finished after PasteDoneMsg")
	}
	if res.pasteLink != "https://paste.rs/abc" {
		t.Fatalf("expected the paste link to be stored, got %q", res.pasteLink)
	}
	if !strings.Contains(res.View(), "https://paste.rs/abc") {
		t.Fatalf("expected paste link in done view, got %q", res.View())
	}
}

func TestModel_PasteDoneMsgFailureShowsNotice(t *testing.T) {
	got := newSavedRunModel()
	next, _ := got.Update(PasteDoneMsg{Err: fmt.Errorf("boom")})
	res := next.(Model)
	if res.pasteLink != "" {
		t.Fatalf("a failed upload must not produce a link, got %q", res.pasteLink)
	}
	if !strings.Contains(res.View(), "boom") {
		t.Fatalf("expected the failure reason in the view, got %q", res.View())
	}
}

// pressPasteYes answers the pending paste question with y.
func pressPasteYes(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	return next.(Model)
}

func TestModel_FailedUploadAsksToRetry(t *testing.T) {
	got := pressPasteYes(t, newSavedRunModel())
	next, _ := got.Update(PasteDoneMsg{Err: fmt.Errorf("boom")})
	res := next.(Model)
	if !res.pastePrompt || !res.pasteRetry {
		t.Fatalf("expected a retry question after a failed upload, got prompt=%v retry=%v", res.pastePrompt, res.pasteRetry)
	}
	if res.pasteLink != "" {
		t.Fatalf("a failed upload must not produce a link, got %q", res.pasteLink)
	}
	view := res.View()
	if !strings.Contains(view, "boom") || !strings.Contains(view, paste.RetryPrompt) {
		t.Fatalf("expected the failure reason and retry question in the view, got %q", view)
	}
}

func TestModel_RetryYesUploadsAgain(t *testing.T) {
	got := pressPasteYes(t, newSavedRunModel())
	failed, _ := got.Update(PasteDoneMsg{Err: fmt.Errorf("boom")})
	next, cmd := failed.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	res := next.(Model)
	if res.pastePrompt || res.pasteRetry {
		t.Fatalf("retrying must resolve the question, got prompt=%v retry=%v", res.pastePrompt, res.pasteRetry)
	}
	if !res.pasteUploading {
		t.Fatal("expected retry to start a fresh upload")
	}
	if cmd == nil {
		t.Fatal("expected an upload command for the retry")
	}
}

func TestModel_RetryDeclinedGivesUp(t *testing.T) {
	for _, answer := range []string{"n", "x", " "} {
		got := pressPasteYes(t, newSavedRunModel())
		failed, _ := got.Update(PasteDoneMsg{Err: fmt.Errorf("boom")})
		next, cmd := failed.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(answer)})
		res := next.(Model)
		if res.pastePrompt || res.pasteRetry || res.pasteUploading {
			t.Errorf("answer %q must give up on the upload, got prompt=%v retry=%v uploading=%v", answer, res.pastePrompt, res.pasteRetry, res.pasteUploading)
		}
		if cmd != nil {
			t.Errorf("answer %q must not return an upload command", answer)
		}
		if !strings.Contains(res.notice, "boom") {
			t.Errorf("answer %q must report the failure, got notice %q", answer, res.notice)
		}
	}
}

func TestModel_SuccessfulRetryShowsLink(t *testing.T) {
	got := pressPasteYes(t, newSavedRunModel())
	failed, _ := got.Update(PasteDoneMsg{Err: fmt.Errorf("boom")})
	retried, _ := failed.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	next, _ := retried.(Model).Update(PasteDoneMsg{Link: "https://paste.rs/retry"})
	res := next.(Model)
	if res.pasteLink != "https://paste.rs/retry" {
		t.Fatalf("expected the retry link to be stored, got %q", res.pasteLink)
	}
	if res.pasteRetry || res.pasteErr != "" {
		t.Fatalf("a successful retry must clear the retry state, got retry=%v err=%q", res.pasteRetry, res.pasteErr)
	}
	if !strings.Contains(res.View(), "https://paste.rs/retry") {
		t.Fatalf("expected the retry link in the view, got %q", res.View())
	}
}

func TestModel_NoPasteQuestionWithoutSavedResults(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		msg  DoneMsg
	}{
		{"asking disabled", Options{AskPaste: false}, DoneMsg{SavedFile: "results/x.md", Links: []string{"vless://a#1"}}},
		{"no healthy links", Options{AskPaste: true}, DoneMsg{SavedFile: "results/x.md"}},
		{"interrupted partial", Options{AskPaste: true}, DoneMsg{Links: []string{"vless://a#1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
			m.opts = tc.opts
			m.screen = ScreenRunning
			next, _ := m.Update(tc.msg)
			if got := next.(Model); got.pastePrompt {
				t.Fatalf("expected no paste question for case %q", tc.name)
			}
		})
	}
}

func TestModel_QOnDoneWhilePromptPendingQuits(t *testing.T) {
	got := newSavedRunModel()
	_, cmd := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected q to quit even while the paste question is pending")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("expected tea.QuitMsg from q while the paste question is pending")
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

func TestModel_FormShowsWhereSourcesComeFrom(t *testing.T) {
	fresh := NewModel([]string{"https://a.com/s"}, "https://x.com", 20, 15*time.Second)
	if view := fresh.View(); !strings.Contains(view, "fresh download") {
		t.Fatalf("expected the default form to announce a fresh download, got %q", view)
	}

	cached := fresh
	cached.opts = Options{FetchMode: subscription.FetchCache, CacheDir: "my-cache"}
	if view := cached.View(); !strings.Contains(view, "cached files in my-cache") {
		t.Fatalf("expected the form to announce the cached sources, got %q", view)
	}

	unnamed := fresh
	unnamed.opts = Options{FetchMode: subscription.FetchCache}
	if view := unnamed.View(); !strings.Contains(view, "sources: cached subscription files") {
		t.Fatalf("expected the cached-sources line without a directory, got %q", view)
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
