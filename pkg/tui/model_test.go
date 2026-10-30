package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"v2pingo/pkg/parser"
	"v2pingo/pkg/paste"
	"v2pingo/pkg/subscription"
	"v2pingo/pkg/types"
)

func TestModel_ResultMsgUpdatesCounters(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, _ := m.Update(DoneMsg{Tested: 5, Success: 2, Failed: 3, SavedFile: "results/x.md"})
	if next.(Model).screen != ScreenDone {
		t.Fatalf("expected ScreenDone after DoneMsg")
	}
}

func TestModel_QuitKeyQuits(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected a quit command on Ctrl+C")
	}
}

func TestModel_TotalMsgSetsTotalAndSurvivesResults(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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

func TestModel_NoticeMsgShowsWarningOnRunningScreen(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	m.cfg = FormConfig{Subs: []string{"https://a.com/s"}, TargetURL: "https://cloudflare.com", Workers: 20, Timeout: 15 * time.Second}
	warning := "failed to fetch subscription https://a.com/s: boom"
	next, _ := m.Update(NoticeMsg{Text: warning})
	got := next.(Model)
	if got.warning != warning {
		t.Fatalf("expected NoticeMsg to store the warning, got %q", got.warning)
	}
	view := got.View()
	if !strings.Contains(view, "boom") {
		t.Fatalf("expected the warning line in the running view, got %q", view)
	}
}

func TestModel_NewRunClearsStaleWarning(t *testing.T) {
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	warned, _ := m.Update(NoticeMsg{Text: "stale warning"})
	if got := warned.(Model); got.warning == "" {
		t.Fatal("expected the warning to be stored first")
	}
	// Form → type filter → confirmed → running; the warning must start clean.
	done, _ := warned.(Model).Update(DoneMsg{Tested: 1, Success: 1})
	backToForm, _ := done.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	filtering, _ := backToForm.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	populated, _ := filtering.(Model).Update(TypesFoundMsg{
		Counts: []parser.TypeCount{{Type: "socks", Count: 1}},
		Links:  []string{"1.2.3.4:1080"},
	})
	running, _ := populated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := running.(Model)
	if got.screen != ScreenRunning {
		t.Fatalf("expected the confirmed filter to start a run, got screen %v", got.screen)
	}
	if got.warning != "" {
		t.Fatalf("a new run must start with a clean warning line, got %q", got.warning)
	}
}

func TestModel_QCancelsRunWithoutQuitting(t *testing.T) {
	cancelled := false
	boundCancel = func() { cancelled = true }
	defer func() { boundCancel = nil }()

	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	m.tested, m.succeeded, m.failed = 3, 2, 1
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := next.(Model)
	if got.screen != ScreenRunning {
		t.Fatalf("q must keep the running screen until the goroutine's final DoneMsg, got %+v", got.screen)
	}
	if !got.stopping {
		t.Fatalf("q must put the model into the stopping state")
	}
	if !cancelled {
		t.Fatalf("expected q to invoke the stored run cancel func")
	}
	// No fabricated DoneMsg and no quit: exactly one DoneMsg ever exists — the
	// final one from the background goroutine, which alone decides the paste
	// question. A fabricated partial DoneMsg here used to race with the final
	// one and could suppress the paste prompt entirely.
	if cmd != nil {
		t.Fatalf("q must not emit a message; the final DoneMsg comes from the goroutine")
	}
	if view := got.View(); !strings.Contains(view, "STOPPING") {
		t.Fatalf("expected the stopping indicator in the running view, got %q", view)
	}
}

// newSavedRunModel returns a done-screen model that just finished a run which
// saved one healthy link, with paste asking enabled.
func newSavedRunModel() Model {
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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
			m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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

	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	m.tested, m.succeeded, m.failed = 3, 2, 1
	m.links = []string{"vless://a#1"}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := next.(Model)
	if got.screen != ScreenRunning || !got.stopping {
		t.Fatalf("Ctrl+C on the running screen must enter the stopping state, got screen=%v stopping=%v", got.screen, got.stopping)
	}
	if !cancelled {
		t.Fatalf("expected Ctrl+C to invoke the stored run cancel func")
	}
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Fatalf("Ctrl+C on the running screen must cancel (so partial results get saved), not quit the program")
		}
		t.Fatalf("Ctrl+C must not emit a message; the final DoneMsg comes from the goroutine, got %T", msg)
	}
}

func TestModel_FormShowsWhereSourcesComeFrom(t *testing.T) {
	fresh := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
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
	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenDone
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected a quit command from q on the done screen (the view advertises q=quit)")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatalf("expected tea.QuitMsg from q on the done screen")
	}
}

// interruptedRunModel drives a model to the running screen with healthy results
// recorded, presses interruptKey, and returns the stopping model. This mirrors
// what the real program does after q / Ctrl+C: cancel the context, show the
// stopping state, and wait for the goroutine's single final DoneMsg.
func interruptedRunModel(t *testing.T, interruptKey tea.KeyType, runes []rune) Model {
	t.Helper()
	cancelled := false
	boundCancel = func() { cancelled = true }
	t.Cleanup(func() { boundCancel = nil })

	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.opts = Options{AskPaste: true}
	m.screen = ScreenRunning
	m.cfg = FormConfig{TargetURL: "https://cloudflare.com"}
	m.tested, m.succeeded, m.failed = 3, 2, 1
	m.links = []string{"vless://a#1", "ss://b#2"}

	var key tea.KeyMsg
	if runes != nil {
		key = tea.KeyMsg{Type: interruptKey, Runes: runes}
	} else {
		key = tea.KeyMsg{Type: interruptKey}
	}
	next, cmd := m.Update(key)
	got := next.(Model)
	if got.screen != ScreenRunning || !got.stopping {
		t.Fatalf("interrupt must enter the stopping state on the running screen, got screen=%v stopping=%v", got.screen, got.stopping)
	}
	if !cancelled {
		t.Fatalf("expected the interrupt to invoke the run cancel func")
	}
	if cmd != nil {
		t.Fatalf("the interrupt must not emit a message; the final DoneMsg comes from the goroutine")
	}
	return got
}

// TestModel_InterruptThenDoneMsgOffersPasteUpload is the core regression for
// the bug where an interrupt could end the program without ever showing the
// paste question: after q / Ctrl+C the single final DoneMsg (the only one the
// model accepts) must land on the done screen WITH the paste prompt whenever
// healthy links were saved, and both answers must work from there.
func TestModel_InterruptThenDoneMsgOffersPasteUpload(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyType
		hits []rune
	}{
		{"q key", tea.KeyRunes, []rune{'q'}},
		{"ctrl+c", tea.KeyCtrlC, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stopping := interruptedRunModel(t, tt.key, tt.hits)

			// The background goroutine finishes saving the partial results and
			// sends its final DoneMsg — the only DoneMsg in the system.
			next, _ := stopping.Update(DoneMsg{
				Tested: 3, Success: 2, Failed: 1,
				SavedFile: "results/x.md",
				Links:     []string{"vless://a#1", "ss://b#2"},
				Elapsed:   time.Second,
			})
			done := next.(Model)
			if done.screen != ScreenDone {
				t.Fatalf("expected the done screen after the final DoneMsg")
			}
			if done.stopping {
				t.Fatalf("the final DoneMsg must clear the stopping state")
			}
			if !done.pastePrompt {
				t.Fatalf("the paste question must be offered after an interrupted run that saved healthy links")
			}
			if view := done.View(); !strings.Contains(view, paste.Prompt) {
				t.Fatalf("expected the paste question in the done view, got %q", view)
			}

			// Choosing yes starts the upload of the saved file.
			next, cmd := done.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			yes := next.(Model)
			if !yes.pasteUploading || cmd == nil {
				t.Fatalf("y must start the upload, got uploading=%v cmd=%v", yes.pasteUploading, cmd != nil)
			}

			// Choosing no keeps the results local.
			next, cmd = done.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
			no := next.(Model)
			if no.pastePrompt || no.pasteUploading || cmd != nil {
				t.Fatalf("n must skip the upload, got prompt=%v uploading=%v cmd=%v", no.pastePrompt, no.pasteUploading, cmd != nil)
			}
			if !strings.Contains(no.notice, "skipped") {
				t.Fatalf("expected the skipped notice after declining, got %q", no.notice)
			}
		})
	}
}

// TestModel_InterruptDuringFetchShowsNoPrompt covers the earliest interruption
// stage: the fetch phase, where no results file can exist. The final DoneMsg
// carries no SavedFile, so no paste question may appear — but the done screen
// must still be reached, and quitting from it stays possible.
func TestModel_InterruptDuringFetchShowsNoPrompt(t *testing.T) {
	stopping := interruptedRunModel(t, tea.KeyRunes, []rune{'q'})

	next, _ := stopping.Update(DoneMsg{Notice: "No valid configs", Elapsed: time.Second})
	done := next.(Model)
	if done.screen != ScreenDone {
		t.Fatalf("expected the done screen after the fetch-phase DoneMsg")
	}
	if done.pastePrompt {
		t.Fatalf("no paste question may be offered when nothing was saved")
	}
	if !strings.Contains(done.View(), "No valid configs") {
		t.Fatalf("expected the notice in the done view, got %q", done.View())
	}
	_, cmd := done.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quitting from the prompt-less done screen must still work")
	}
}

// TestModel_InterruptBeforeRunStartNeverStartsTheRun covers the narrow window
// between Enter and the StartTestsMsg handling: an interrupt there must stop
// the run from ever launching, and the done screen still becomes reachable via
// the empty DoneMsg the StartTestsMsg handler emits.
func TestModel_InterruptBeforeRunStartNeverStartsTheRun(t *testing.T) {
	started := false
	savedBound := bound
	bound = nil // no background runner wired in this test
	defer func() { bound = savedBound }()

	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.opts = Options{AskPaste: true}
	m.screen = ScreenRunning
	m.cfg = FormConfig{TargetURL: "https://cloudflare.com"}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	stopping := next.(Model)
	if !stopping.stopping {
		t.Fatalf("expected the stopping state after the early interrupt")
	}

	// The queued StartTestsMsg arrives after the interrupt.
	next, cmd = stopping.Update(StartTestsMsg{Cfg: FormConfig{TargetURL: "https://cloudflare.com"}})
	if cmd == nil {
		t.Fatal("expected a cmd emitting the empty DoneMsg for the never-started run")
	}
	msg := cmd()
	done, ok := msg.(DoneMsg)
	if !ok {
		t.Fatalf("expected a DoneMsg from the aborted start, got %T", msg)
	}
	if done.SavedFile != "" || done.Tested != 0 {
		t.Fatalf("a never-started run must produce an empty DoneMsg, got %+v", done)
	}
	if done.Notice != "run never started" {
		t.Fatalf("expected the run-never-started notice, got %q", done.Notice)
	}
	final, _ := stopping.Update(done)
	got := final.(Model)
	if got.screen != ScreenDone || got.stopping {
		t.Fatalf("expected the done screen without the stopping state, got screen=%v stopping=%v", got.screen, got.stopping)
	}
	if got.pastePrompt {
		t.Fatalf("no paste question may be offered when the run never started")
	}
	_ = started
}

// TestModel_RepeatedInterruptsAreIgnored pins that mashing q / Ctrl+C while
// the run is winding down neither quits the program nor re-cancels anything.
func TestModel_RepeatedInterruptsAreIgnored(t *testing.T) {
	cancelCalls := 0
	boundCancel = func() { cancelCalls++ }
	defer func() { boundCancel = nil }()

	m := NewModel([]string{"https://a.com/s"}, "https://cloudflare.com", 20, 15*time.Second)
	m.screen = ScreenRunning
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	stopping := next.(Model)
	next, cmd = stopping.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := next.(Model)
	if !got.stopping || got.screen != ScreenRunning {
		t.Fatalf("repeated interrupts must keep the stopping state, got screen=%v stopping=%v", got.screen, got.stopping)
	}
	if cmd != nil {
		t.Fatalf("repeated interrupts must not emit a message")
	}
	if cancelCalls != 1 {
		t.Fatalf("the cancel func must be invoked exactly once, got %d", cancelCalls)
	}
}
