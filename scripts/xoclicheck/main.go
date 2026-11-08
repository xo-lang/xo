// Command xoclicheck reports whether $XO_CLI is stale for this checkout,
// the check internal/xotool makes when a Go test starts (StaleCLI), so a
// wrapper can make it before a job waits for a heavy.sh slot: a stale
// XO_CLI used to queue, take a slot, and fail every test in under a
// second (2026-11-07: a control run of TestGoldenRunIslands waited in the
// gate queue, then failed all 20 subtests with "is stale").
//
//	go run ./scripts/xoclicheck        # exit 0: unset, current, or XO_CLI_STALE_OK=1; 1: stale
//	go run ./scripts/xoclicheck PATH   # the same for the xo at PATH (heavy.sh: a job that runs a checkout's bin/xo)
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	f := strings.Fields(os.Getenv("XO_CLI"))
	what := "XO_CLI"
	if len(os.Args) > 1 {
		f = os.Args[1:2]
		what = "the xo"
	}
	if len(f) == 0 || os.Getenv("XO_CLI_STALE_OK") == "1" {
		return
	}
	if _, err := os.Stat(f[0]); err != nil {
		fmt.Fprintf(os.Stderr, "xoclicheck: %s=%s: %v\n", what, f[0], err)
		os.Exit(1)
	}
	root, err := xotool.Root()
	if err != nil {
		fmt.Fprintf(os.Stderr, "xoclicheck: %v\n", err)
		os.Exit(2)
	}
	if err := xotool.StaleCLI(root, f[0]); err != nil {
		fmt.Fprintf(os.Stderr, "xoclicheck: %v\n", err)
		os.Exit(1)
	}
}
