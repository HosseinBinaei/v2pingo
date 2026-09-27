package main

import (
	"time"

	"v2pingo/pkg/tui"
)

func runTUI(subs []string, target string, workers int, timeout time.Duration) error {
	return tui.Run(subs, target, workers, timeout)
}
