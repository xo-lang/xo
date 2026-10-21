// Command stdgen regenerates std/rt/std_gen_M.go, the Go backend
// runtime's declarations of std module M's types, and std/rt/std_sigs_M.go,
// the signature checks of its hand written functions, from std/*.xo
// (internal/stdgen), and removes a generated file no module has left:
//
//	go run ./scripts/stdgen          # write the file
//	go run ./scripts/stdgen -check   # exit 1 when a file is stale, missing, or left over
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
	"sort"

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
	have, err := stdgen.Generated(root)
	if err != nil {
		return err
	}
	for _, p := range have {
		if _, ok := files[p]; ok {
			continue
		}
		if check {
			return fmt.Errorf("%s is left over (no std module generates it): run go run ./scripts/stdgen", p)
		}
		if err := os.Remove(filepath.Join(root, p)); err != nil {
			return err
		}
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		path := filepath.Join(root, p)
		if check {
			old, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("%s is missing: run go run ./scripts/stdgen", p)
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
