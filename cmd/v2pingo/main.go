package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"v2pingo/pkg/parser"
	"v2pingo/pkg/reporter"
	"v2pingo/pkg/subscription"
	"v2pingo/pkg/tester"
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
	defaultSubsFlag := flag.Bool("default-subs", true, "Include the subscription URLs listed in the sub-input file")
	subFileFlag := flag.String("sub-file", subscription.DefaultSubscriptionFile, "Path to the subscription list file (one URL per line)")
	var verboseFlag bool
	flag.BoolVar(&verboseFlag, "v", false, "Enable verbose failure output")
	flag.BoolVar(&verboseFlag, "verbose", false, "Enable verbose failure output")

	var subFlags stringSliceFlag
	flag.Var(&subFlags, "sub", "Custom subscription URL (repeatable or comma-separated)")
	tuiFlag := flag.Bool("tui", true, "Launch interactive terminal UI")

	flag.Parse()

	if *tuiFlag && isTerminal() {
		subFilePath := subscription.ResolveSubscriptionFile(*subFileFlag)
		fileSubs, _ := subscription.LoadSubscriptionFile(subFilePath)
		allSubs := append(append([]string{}, fileSubs...), subFlags...)
		if err := runTUI(allSubs, *targetURLFlag, *concurrencyFlag, *timeoutFlag); err != nil {
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

	fmt.Printf("Fetching subscriptions from %d sources...\n", len(subURLs))
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 60*time.Second)
	rawLinks := subscription.FetchSubscriptions(fetchCtx, subURLs, httpClient)
	fetchCancel()

	fmt.Printf("Extracted %d raw proxy share links.\n", len(rawLinks))

	configs := parser.DeduplicateConfigs(rawLinks)
	fmt.Printf("Total unique configs to test: %d\n", len(configs))

	if len(configs) == 0 {
		fmt.Fprintln(os.Stderr, "No valid proxy configurations found to test.")
		os.Exit(0)
	}

	rootCtx, rootCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer rootCancel()

	rep := reporter.NewReporter(len(configs), verboseFlag)

	startTime := time.Now()
	tester.RunPool(rootCtx, configs, *targetURLFlag, *concurrencyFlag, *timeoutFlag, rep.Record)
	duration := time.Since(startTime)

	rep.PrintSummary(duration)
	if _, err := rep.SaveResults(); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving results: %v\n", err)
	}
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
