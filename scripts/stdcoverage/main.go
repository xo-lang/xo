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
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/xo-lang/xo/internal/stdcoverage"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	check := flag.Bool("check", false, "exit 1 when an uncovered declaration is not in allow.txt")
	edgesDir := flag.String("edges", "", "a directory of `xo dev edges` output: report the resolution's coverage and its difference from the name scan")
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
	if *edgesDir != "" {
		resolved(os.Stdout, root, *edgesDir, entries, total)
		return
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

// resolved prints the coverage the checker's resolution gives, and the
// difference from the name scan: the declarations the scan calls covered
// that no edge reaches (what it over-claims), and those the scan calls
// uncovered that an edge does reach (what it under-claims). The two lists
// are what a decision to replace the scan with edges has to answer for.
func resolved(w io.Writer, root, dir string, scanned []stdcoverage.Entry, total int) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.tsv"))
	if err != nil || len(paths) == 0 {
		fmt.Fprintf(w, "stdcoverage: no .tsv edge files under %s\n", dir)
		os.Exit(1)
	}
	edges, err := stdcoverage.ReadEdges(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcoverage:", err)
		os.Exit(1)
	}
	got, _, impls, err := stdcoverage.UncoveredResolved(root, edges)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcoverage:", err)
		os.Exit(1)
	}
	key := func(e stdcoverage.Entry) string { return e.Module + "." + e.Symbol + " (" + e.Kind + ")" }
	scannedSet := map[string]bool{} // declarations the name scan calls uncovered
	for _, e := range scanned {
		scannedSet[key(e)] = true
	}
	gotSet := map[string]bool{} // declarations the edges call uncovered
	for _, e := range got {
		gotSet[key(e)] = true
	}
	var over, under []string
	for k := range scannedSet {
		if !gotSet[k] {
			under = append(under, k)
		}
	}
	for k := range gotSet {
		if !scannedSet[k] {
			over = append(over, k)
		}
	}
	sort.Strings(over)
	sort.Strings(under)
	nimpl := 0
	for _, k := range over {
		if impls[k] {
			nimpl++
		}
	}
	fmt.Fprintf(w, "%d declarations, %d uncovered by the name scan, %d uncovered by %d edges\n", total, len(scanned), len(got), len(edges))
	fmt.Fprintf(w, "the scan covered, no edge reaches (%d, %d of them methods whose name a covered interface method has):\n", len(over), nimpl)
	for _, k := range over {
		mark := ""
		if impls[k] {
			mark = "  [impl]"
		}
		fmt.Fprintf(w, "  %s%s\n", k, mark)
	}
	fmt.Fprintf(w, "the scan did not cover, an edge reaches (%d):\n", len(under))
	for _, k := range under {
		fmt.Fprintf(w, "  %s\n", k)
	}
}
