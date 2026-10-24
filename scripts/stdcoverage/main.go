// Command stdcoverage prints the std API that no corpus or golden reaches
// (internal/stdcoverage): every top level function and method std/*.xo and
// std/native/prelude.xo declare, marked against the calls the generated
// corpora and the program goldens make, so a new declaration can be given
// a corpus before it lands.
//
//	go run ./scripts/stdcoverage
//	go run ./scripts/stdcoverage -check   # exit 1 when allow.txt is not enough
//
// internal/repotest's TestStdAPICovered runs the same check in the fast set.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/xo-lang/xo/internal/stdcoverage"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	check := flag.Bool("check", false, "exit 1 when an uncovered declaration is not in allow.txt")
	flag.Parse()
	root, err := xotool.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcoverage:", err)
		os.Exit(1)
	}
	entries, total, err := stdcoverage.Uncovered(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcoverage:", err)
		os.Exit(1)
	}
	rules, err := stdcoverage.Allow()
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcoverage:", err)
		os.Exit(1)
	}
	allowed := func(e stdcoverage.Entry) bool {
		for _, r := range rules {
			if r.Match(e) {
				return true
			}
		}
		return false
	}
	byModule := map[string][]stdcoverage.Entry{}
	var open []stdcoverage.Entry
	for _, e := range entries {
		byModule[e.Module] = append(byModule[e.Module], e)
		if !allowed(e) {
			open = append(open, e)
		}
	}
	fmt.Printf("%d declarations, %d uncovered, %d of those in allow.txt\n", total, len(entries), len(entries)-len(open))
	mods := make([]string, 0, len(byModule))
	for m := range byModule {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		fmt.Printf("== %s (%d)\n", m, len(byModule[m]))
		for _, e := range byModule[m] {
			fmt.Printf("  %s\n", e.Symbol)
		}
	}
	if len(open) > 0 {
		fmt.Printf("\nnot in allow.txt (%d):\n", len(open))
		for _, e := range open {
			fmt.Printf("  %s.%s (%s)\n", e.Module, e.Symbol, e.Kind)
		}
		if *check {
			os.Exit(1)
		}
	}
}
