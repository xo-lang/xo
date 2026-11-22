// goldenpass DIR: folds DIR's golden run files into DIR/pass.json
// (internal/xotool goldenpass.go, decision 0737 step 2) and prints one
// line: how many goldens passed, were skipped (their binary identical to
// the last passing run's) and failed, and the run time the skips saved.
package main

import (
	"fmt"
	"os"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: goldenpass DIR")
		os.Exit(2)
	}
	s, err := xotool.MergeGoldenPass(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "goldenpass: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("golden pass: %d passed, %d skipped (identical to the last passing run, %d s of runs saved), %d failed\n", s.Passed, s.Skipped, s.SavedMS/1000, s.Failed)
}
