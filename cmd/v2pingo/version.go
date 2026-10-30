package main

import (
	"context"
	"strings"

	"v2pingo/pkg/update"
)

// version is the release version of the binary. Release builds inject it at
// compile time with -ldflags "-X main.version=<tag>"; a plain `go build`
// leaves it at "dev", which also disables the update check.
var version = "dev"

// noticeProbe is the release probe behind startUpdateCheck, kept as a variable
// so tests can stub the network call.
var noticeProbe = update.Notice

// startUpdateCheck probes GitHub in the background for a newer release and
// returns a channel that carries at most one single-line notice (empty when
// the run is current or the probe fails). The channel is buffered, so the
// probe never blocks on a reader, and a nil channel means the check is
// disabled — dev builds have no release version to compare against.
func startUpdateCheck(current string) <-chan string {
	if current = strings.TrimSpace(current); current == "" || current == "dev" {
		return nil
	}
	ch := make(chan string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), update.Timeout)
		defer cancel()
		ch <- noticeProbe(ctx, nil, current, "")
	}()
	return ch
}
