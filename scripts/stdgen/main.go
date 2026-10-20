// Command stdgen regenerates std/rt/std_gen.go, the Go backend runtime's
// declarations of the std types, and std/rt/std_sigs_gen.go, the
// signature checks of the hand written std functions, from std/*.xo
// (internal/stdgen):
//
//	go run ./scripts/stdgen          # write the file
//	go run ./scripts/stdgen -check   # exit 1 when the file is stale
//
// It runs the xo the tests run (internal/xotool: $XO_CLI, else the
// checkout's bin/xo, built when stale).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	files, err := stdgen.Generate(root, xo)
	if err != nil {
		return err
	}
	for _, p := range []string{stdgen.Path, stdgen.SigsPath} {
		path := filepath.Join(root, p)
		if check {
			old, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !bytes.Equal(old, files[p]) {
				return fmt.Errorf("%s is stale: run go run ./scripts/stdgen", p)
			}
			continue
		}
		if err := os.WriteFile(path, files[p], 0o644); err != nil {
			return err
		}
	}
	return nil
}
