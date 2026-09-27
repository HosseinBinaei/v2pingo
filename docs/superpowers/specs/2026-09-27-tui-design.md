# TUI Design — v2pingo (BubbleTea)

Date: 2026-09-27
Status: proposed, awaiting user review
Approach: BubbleTea + Bubbles + LipGloss (option 1, approved)

## 1. Goal

Replace manual CLI flag typing (`-sub`, `-url`, `-c`, `-timeout`) with a
minimal interactive terminal UI: input form + live run + summary, while
keeping the current CLI behavior intact for scripts.

## 2. Architecture and boundaries

- New isolated package: `pkg/tui` with `model.go` (BubbleTea Model/Update/View)
  and `run.go` (entry point `Run(initialConfig)`).
- `cmd/v2pingo/main.go` branches: `-tui=false` or piped output selects the
  existing CLI path unchanged; otherwise launches `tui.Run` using the current
  flag values (`-url`, `-c`, `-timeout`, `-sub`, `-sub-file`) as form defaults.
- Test/fetch/parse logic is untouched. Only result delivery changes: the TUI
  passes its own thread-safe `Collector` (counts + 2xx `successfulLinks` +
  mutex) to `tester.RunPool` instead of `reporter.Reporter.Record`, and each
  result is forwarded with `program.Send(ResultMsg{...})`. The TUI never
  prints directly; everything flows through `Update()`.
- Result saving reuses the existing semantic: only `Success == true` with
  status `200 <= code < 300` and non-empty `RawLink` is stored. Same rule as
  `pkg/reporter/reporter.go` and the range style of
  `pkg/subscription/fetcher.go`.

Files:

- New: `pkg/tui/model.go`, `pkg/tui/run.go`, `pkg/tui/model_test.go`.
- Small change: `cmd/v2pingo/main.go` (add `-tui` flag + branch).
- Deps: `go.mod` adds `charmbracelet/bubbletea`, `charmbracelet/bubbles`,
  `charmbracelet/lipgloss`.
- Untouched: `pkg/tester`, `pkg/reporter`, `pkg/parser`, `pkg/subscription`.

## 3. Screens and data flow

States in one Model: `form` -> `running` -> `done`.

- `form`: four Bubbles `textinput` fields with defaults: subscriptions
  (preloaded lines from `sub-input.txt` plus one extra-sub field), target URL
  (`https://x.com`), workers (`20`), timeout (`15s`). Keys: `Tab` moves,
  `Enter` starts, `Ctrl+C` quits.
- `running`: `progress` bar + `spinner`, counters `tested/total` and
  `success/fail`, plus a `viewport` showing only the last 10 successes
  (`[#n] [proto] host:port - latency (HTTP xxx)`) to stay fast.
  `q`/`Ctrl+C` cancels cleanly.
- `done`: summary (Total/Tested/Success/Failed/Duration), saved-file path,
  `Enter` back to form, `q` quit.

Flow: `Enter` in form emits `StartTestsMsg`; a `tea.Cmd` runs
`FetchSubscriptions -> DeduplicateConfigs -> RunPool` in a separate
goroutine; `onResult` forwards each result as `ResultMsg`; completion emits
`DoneMsg{summary, savedFile}`. Form validation runs before start: workers
must be 1..200, timeout must parse with `time.ParseDuration`, at least one
subscription required.

## 4. Error handling

- Form errors: inline red message under the offending field; `Enter` blocked.
- Fetch errors: same as CLI (warn, continue with remaining subs); zero valid
  configs returns to the form with a "No valid configs" banner.
- Cancel (`Ctrl+C`/`q`): cancels the run context (mirrors the existing
  `signal.NotifyContext` behavior), workers stop, `done` shows a partial
  summary and saves collected 2xx links so far.

## 5. Testing

- Unit (no real terminal): `Update()` state transitions form->running->done;
  table-driven validation tests (reject `0`, `500`, `abc`; accept `20`;
  reject bad durations and empty subs); 2xx collector boundary
  (200/201/204/299 saved; 199/300/404 and `Success=false` not saved).
- Manual: `go run ./cmd/v2pingo -tui` on Windows Terminal;
  ` -tui=false` regression check of the plain CLI.
- `go test ./...` and `go vet ./...` must stay green.

## 6. Non-goals (YAGNI)

No editing of `sub-input.txt` from the TUI, no results history browser, no
per-node detail view, no config-file persistence. Those are follow-upsideas,
not part of this spec.
