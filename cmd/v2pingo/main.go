package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"v2pingo/pkg/paste"
	"v2pingo/pkg/parser"
	"v2pingo/pkg/reporter"
	"v2pingo/pkg/runner"
	"v2pingo/pkg/subscription"
	"v2pingo/pkg/tui"
)

// stringSliceFlag supports repeatable -sub flags and comma-separated values.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			*s = append(*s, trimmed)
		}
	}
	return nil
}

func main() {
	targetURLFlag := flag.String("url", "https://x.com", "Target test URL")
	concurrencyFlag := flag.Int("c", 20, "Concurrency workers")
	timeoutFlag := flag.Duration("timeout", 15*time.Second, "Test timeout duration")
	batchFlag := flag.Int("batch", runner.DefaultBatchSize, "Configs parsed/tested per batch (bounds RAM)")
	defaultSubsFlag := flag.Bool("default-subs", true, "Include the subscription URLs listed in the sub-input file")
	subFileFlag := flag.String("sub-file", subscription.DefaultSubscriptionFile, "Path to the subscription list file (one URL per line)")
	var verboseFlag bool
	flag.BoolVar(&verboseFlag, "v", false, "Enable verbose failure output")
	flag.BoolVar(&verboseFlag, "verbose", false, "Enable verbose failure output")

	var subFlags stringSliceFlag
	flag.Var(&subFlags, "sub", "Custom subscription URL (repeatable or comma-separated)")
	tuiFlag := flag.Bool("tui", true, "Launch interactive terminal UI")
	pasteFlag := flag.Bool("paste", true, "Ask whether to upload the saved results file to paste.rs")
	pasteURLFlag := flag.String("paste-url", paste.DefaultEndpoint, "Paste service endpoint used by -paste (a custom endpoint disables the built-in fallback services)")
	cacheFlag := flag.Bool("cache", true, "Ask whether to reuse the cached subscription files (false disables the cache)")
	cacheDirFlag := flag.String("cache-dir", subscription.DefaultCacheDir, "Directory that keeps the fetched subscription files")
	typesFlag := flag.String("types", "", "Only test these config types (comma-separated: vmess, vless, trojan, ss, socks, hysteria2, wireguard; empty = all)")

	flag.Parse()

	// Every interactive question shares one buffered reader so no buffered line
	// is lost between the cache question and the paste questions.
	reader := bufio.NewReader(os.Stdin)
	cacheDir := resolveCacheDir(*cacheDirFlag, *cacheFlag)
	typeFilter, err := parseTypesFlag(*typesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *tuiFlag && isTerminal() {
		subFilePath := subscription.ResolveSubscriptionFile(*subFileFlag)
		fileSubs, _ := subscription.LoadSubscriptionFile(subFilePath)
		allSubs := append(append([]string{}, fileSubs...), subFlags...)
		if err := runTUI(allSubs, *targetURLFlag, *concurrencyFlag, *timeoutFlag, tui.Options{
			AskPaste:      *pasteFlag,
			PasteEndpoint: *pasteURLFlag,
			FetchMode:     resolveFetchMode(reader, cacheDir),
			CacheDir:      cacheDir,
			Types:         typeFilter,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Assemble subscription URL list
	var subURLs []string
	if *defaultSubsFlag {
		subFilePath := subscription.ResolveSubscriptionFile(*subFileFlag)
		fileSubs, err := subscription.LoadSubscriptionFile(subFilePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		} else {
			fmt.Printf("Loaded %d subscription URL(s) from %s\n", len(fileSubs), subFilePath)
		}
		subURLs = append(subURLs, fileSubs...)
	}
	subURLs = append(subURLs, subFlags...)

	if len(subURLs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no subscription URLs provided (add them to %s or specify -sub)\n", *subFileFlag)
		os.Exit(1)
	}

	// The cache question is asked once, before any source is fetched, so the run
	// either reuses the files of a previous run or refreshes them from GitHub.
	mode := resolveFetchMode(reader, cacheDir)

	fmt.Printf("Fetching subscriptions from %d sources...\n", len(subURLs))
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 60*time.Second)
	rawLinks := subscription.FetchSubscriptions(fetchCtx, subURLs, httpClient, cacheDir, mode)
	fetchCancel()

	fmt.Printf("Extracted %d raw proxy share links.\n", len(rawLinks))

	if len(typeFilter) > 0 {
		kept := parser.FilterByTypes(rawLinks, typeFilter)
		fmt.Printf("Type filter %s: kept %d of %d links.\n", strings.Join(typeFilter, ", "), len(kept), len(rawLinks))
		rawLinks = kept
	}

	if len(rawLinks) == 0 {
		fmt.Fprintln(os.Stderr, "No valid proxy configurations found to test.")
		os.Exit(0)
	}

	rootCtx, rootCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer rootCancel()

	rep := reporter.NewReporter(len(rawLinks), verboseFlag)
	resultsFile := runner.DefaultResultsFile()
	checkpointFile := resultsFile + ".checkpoint"

	startTime := time.Now()
	err = runner.RunBatches(rootCtx, rawLinks, runner.BatchConfig{
		TargetURL:      *targetURLFlag,
		Concurrency:    *concurrencyFlag,
		Timeout:        *timeoutFlag,
		BatchSize:      *batchFlag,
		ResultsFile:    resultsFile,
		CheckpointFile: checkpointFile,
	}, rep.Record)
	duration := time.Since(startTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running batches: %v\n", err)
	}

	rep.PrintSummary(duration)
	fmt.Printf("Healthy links appended to %s (checkpoint: %s)\n", resultsFile, checkpointFile)

	if *pasteFlag {
		promptPasteUpload(reader, resultsFile, *pasteURLFlag)
	}
}

// parseTypesFlag turns the -types value into canonical type names. An empty or
// blank value disables filtering (nil); unknown names fail with the valid
// names listed so the user can correct the flag.
func parseTypesFlag(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(value, ",") {
		name, ok := parser.NormalizeTypeName(part)
		if !ok {
			return nil, fmt.Errorf("unknown config type %q (valid: vmess, vless, trojan, ss/shadowsocks, socks/socks5, hysteria2/hy2, wireguard/wg)", strings.TrimSpace(part))
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out, nil
}

// resolveCacheDir returns the directory used for cached subscription sources, or
// "" when -cache=false disables the cache for this run.
func resolveCacheDir(dir string, enabled bool) string {
	if !enabled {
		return ""
	}
	return subscription.ResolveCacheDir(dir)
}

// resolveFetchMode decides where the subscription sources of this run come from.
// An empty cacheDir (the cache disabled with -cache=false) always downloads the
// sources; otherwise the question is asked only when the cache directory already
// holds at least one source — without a cache a download is the only option — and
// only an explicit y / yes refreshes the sources from GitHub, while Enter and
// every other answer reuses the cached files.
func resolveFetchMode(reader *bufio.Reader, cacheDir string) subscription.FetchMode {
	if cacheDir == "" {
		return subscription.FetchNetwork
	}

	stats := subscription.InspectCache(cacheDir)
	if stats.Files == 0 {
		fmt.Println("No cached subscriptions; downloading fresh copies.")
		return subscription.FetchNetwork
	}

	fmt.Printf("Cache: %d file(s), newest %s.\n", stats.Files, stats.Updated.Format("2006-01-02 15:04:05"))
	if askYesNo(reader, subscription.CachePrompt) {
		return subscription.FetchNetwork
	}
	return subscription.FetchCache
}

// promptPasteUpload asks the user whether to publish the saved results file and
// uploads it only on an affirmative answer (y / yes, any case). Anything else —
// including an empty answer or EOF — keeps the run local and skips the upload,
// so a non-interactive run never blocks or publishes by accident.
//
// A failed upload is reported and then the user is asked whether to try again;
// declining retry (or any non-affirmative answer) ends the process with the
// results still saved locally.
func promptPasteUpload(reader *bufio.Reader, path, endpoint string) {
	if _, err := os.Stat(path); err != nil {
		return // no results file was saved, nothing to publish
	}

	// The reader is shared with every other question so no buffered line is lost
	// between the initial prompt and a retry prompt.
	if !askYesNo(reader, paste.Prompt) {
		fmt.Println("Skipped paste upload.")
		return
	}

	client := paste.NewClient(endpoint)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		link, err := client.UploadFile(ctx, path)
		cancel()

		if err == nil {
			if link == "" {
				fmt.Println("Skipped paste upload.")
				return
			}
			fmt.Printf("Paste link: %s\n", link)
			return
		}

		fmt.Fprintf(os.Stderr, "Warning: failed to upload results file: %v\n", err)
		if !askYesNo(reader, paste.RetryPrompt) {
			fmt.Println("Giving up on paste upload; results remain saved locally.")
			return
		}
	}
}

// askYesNo prints prompt and reads one line, reporting whether the answer means
// yes (y / yes, any case, whitespace-tolerant). EOF and empty answers are no.
func askYesNo(reader *bufio.Reader, prompt string) bool {
	fmt.Print(prompt)
	answer, err := reader.ReadString('\n')
	if err != nil {
		// EOF or a missing trailing newline: keep the next line separate.
		fmt.Println()
	}
	return paste.IsAffirmative(answer)
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
