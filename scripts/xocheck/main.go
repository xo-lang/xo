// Command xocheck runs `xo check` on directories of this checkout with an
// xo that reports what one built from the current sources would, without
// building one when it can (owner, 2026-10-21): the checkout's current
// bin/xo, else a cached build whose checker sources are the tree's
// (internal/xotool CheckerXo), else it builds bin/xo first
// (go run ./scripts/xobuild). The tree's std is used (XO_STD).
//
//	go run ./scripts/xocheck [xo check flags] DIR...   # e.g. --no-gc compiler/ssa
//
// With --no-gc the SSA builder (compiler/ssa, XO1201) counts as a checker
// source too. A directory inside std is refused with what to check
// instead: std's modules are checked through a program that uses them
// (2026-10-24: `xocheck std/native/llrt` printed 7,530 "module
// native/llrt/raw not found" errors).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	args := os.Args[1:]
	root, err := xotool.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "xocheck:", err)
		os.Exit(2)
	}
	if why := stdDir(root, args); why != "" {
		fmt.Fprintln(os.Stderr, "xocheck:", why)
		os.Exit(2)
	}
	dirs := slices.Clone(xotool.CheckerDirs)
	if slices.Contains(args, "--no-gc") {
		dirs = append(dirs, "compiler/ssa")
	}
	xo, why, ok := xotool.CheckerXo(root, dirs)
	if !ok {
		fmt.Fprintln(os.Stderr, "xocheck: no xo with this tree's checker sources; building bin/xo")
		xo, err = xotool.Ensure(root, os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "xocheck:", err)
			os.Exit(2)
		}
		why = "the tree's bin/xo, just built"
	}
	fmt.Fprintf(os.Stderr, "xocheck: %s (%s)\n", xo, why)
	cmd := exec.Command(xo, append([]string{"check"}, args...)...)
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "xocheck:", err)
		os.Exit(2)
	}
}

// stdDir is why args name a directory inside the checkout's std, "" when
// none does: std's modules resolve their `use` lines (native/llrt/raw and
// the rest) only as a program's dependencies, so checking one on its own
// fails on every one of them.
func stdDir(root string, args []string) string {
	std := filepath.Join(root, "std")
	for _, a := range args {
		if len(a) > 0 && a[0] == '-' {
			continue
		}
		abs, err := filepath.Abs(a)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(std, abs)
		if err != nil || rel == ".." || (len(rel) >= 3 && rel[:3] == "../") {
			continue
		}
		return fmt.Sprintf("%s is inside std, whose modules are checked as a program's dependencies, not on their own (their use lines, such as native/llrt/raw, resolve only from a program): check the directory of a program that uses the module instead", a)
	}
	return ""
}
