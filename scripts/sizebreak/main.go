// Command sizebreak reports where a built Xo binary's code goes, by module.
//
// It reads a Mach-O or ELF executable the way the code-size reports of
// 2026-10-24 did: the size of a text symbol is the gap to the next symbol's
// address, the gaps are checked against the text section's size, and the
// symbols are grouped by the `_xo_f_<module>_2e<name>` mangling of
// compiler/. The Map runtime's three families (xo.map, xo.mlog, xo.maph) are
// broken down by the element type each is instantiated for, because that is
// where a Map-heavy program's bytes go.
//
// The numbers a size claim quotes are this tool's: the text section size,
// the group table, and the top symbols. -reach adds a lower bound on dead
// code: the symbols unreachable from the entry points over direct branch
// edges and the adrp/add pairs that materialize a function address (an
// indirect call through a loaded register is not followed, so a symbol
// listed as unreachable may still run).
//
// Usage:
//
//	go run ./scripts/sizebreak [-top=N] [-json] [-reach] BINARY
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	top := flag.Int("top", 20, "symbols to list by size")
	asJSON := flag.Bool("json", false, "print one JSON object")
	reach := flag.Bool("reach", false, "add a lower bound on unreachable code (arm64)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: sizebreak [-top=N] [-json] [-reach] BINARY\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if *top < 0 {
		fmt.Fprintf(os.Stderr, "sizebreak: -top must not be negative\n")
		os.Exit(2)
	}
	bin, err := loadBinary(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sizebreak: %v\n", err)
		os.Exit(1)
	}
	rep, err := analyze(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sizebreak: %v\n", err)
		os.Exit(1)
	}
	rep.Top = rep.Top[:min(*top, len(rep.Top))]
	if *reach {
		rep.Reach = reachability(bin)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "sizebreak: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printReport(os.Stdout, rep)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
