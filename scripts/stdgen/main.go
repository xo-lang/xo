// Command stdgen regenerates std/rt/std_gen_M.go, the Go backend
// runtime's declarations of std module M's types, and std/rt/std_sigs_M.go,
// the signature checks of its hand written functions, from std/*.xo
// (internal/stdgen), and removes a generated file no module has left:
//
//	go run ./scripts/stdgen          # write the file
//	go run ./scripts/stdgen -check   # exit 1 when a file is stale, missing, or left over
//
// go run ./scripts/stdsync does this and spec/std.md's prelude index and the
// habit goldens in one command, which is what a std/prelude.xo change needs;
// this command stays for a std type's change alone.
//
// It runs the xo the tests run (internal/xotool: $XO_CLI, else the
// checkout's bin/xo, built when stale).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xo-lang/xo/internal/stdgen"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	check := flag.Bool("check", false, "exit 1 when a generated file differs from the generator's output")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "stdgen:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	root, err := xotool.Root()
	if err != nil {
		return err
	}
	xo, err := xotool.Command()
	if err != nil {
		return err
	}
	stale, err := stdgen.Run(root, xo, check)
	if err != nil {
		return err
	}
	for _, s := range stale {
		fmt.Fprintln(os.Stderr, "stdgen:", s+": run go run ./scripts/stdsync")
	}
	if len(stale) > 0 {
		os.Exit(1)
	}
	return nil
}
