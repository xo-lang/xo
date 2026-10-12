// Command xo-gobind binds Go packages for `use go` (decision 0041): the
// half of `xo bind` that stays Go after the CLI moves to Xo (decision
// 0178 stage 6), since it reads Go's export data with go/types. The self
// hosted `xo` execs it for `xo bind <path>...`, and its checker execs it
// to bind a `use go` package missing from the binding cache.
//
//	xo-gobind [--dir DIR] [--json] <Go import path>...
//
// The output, the files, and the exit status are `xo bind`'s
// (gobind.RunCLI).
package main

import (
	"flag"
	"os"

	"github.com/xo-lang/xo/internal/gobind"
)

func main() {
	fl := flag.NewFlagSet("xo-gobind", flag.ContinueOnError)
	dir := fl.String("dir", ".", "a directory of the Xo project (its xo.mod root holds the cache)")
	asJSON := fl.Bool("json", false, "print the coverage of each package as JSON")
	if err := fl.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	os.Exit(gobind.RunCLI(fl.Args(), *dir, *asJSON, os.Stdout, os.Stderr))
}
