package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/ssa"
)

// runAsmDump is the hidden `xo asmdump [--os=darwin|linux] [--no-inline]
// [--each] [--test] [--bench=N] <dirs...>` (decision 0158): it checks
// the modules under dirs as a native build does, builds the SSA as `xo
// ssadump` does (fault positions relative to the working directory),
// and prints the assembly of the arm64 emitter (build.Arm64AsmDump), the
// text the self hosted emitter (selfhost/arm64) prints. --each builds
// every dir as a run of its own, each preceded by `=== asm <dir>`; a run
// the checker rejects prints `== rejected`, one that does not build
// `== error <why>`. --test generates the test binary. --bench=N builds
// each run N times and prints the time spent in the emitter alone.
func runAsmDump(args []string, stdout, stderr io.Writer) int {
	repeat := 0
	each, tests, noInline := false, false, false
	goos := "darwin"
	var dirs []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--bench="):
			v, err := strconv.Atoi(strings.TrimPrefix(a, "--bench="))
			if err != nil {
				fmt.Fprintln(stderr, "xo asmdump: --bench needs a number")
				return 2
			}
			repeat = v
		case strings.HasPrefix(a, "--os="):
			goos = strings.TrimPrefix(a, "--os=")
			if goos != "darwin" && goos != "linux" {
				fmt.Fprintf(stderr, "xo asmdump: --os=%s: want darwin or linux\n", goos)
				return 2
			}
		case a == "--no-inline":
			noInline = true
		case a == "--each":
			each = true
		case a == "--test":
			tests = true
		default:
			dirs = append(dirs, a)
		}
	}
	if len(dirs) == 0 {
		fmt.Fprintln(stderr, "usage: xo asmdump [--os=darwin|linux] [--no-inline] [--each] [--test] [--bench=N] <dirs...>")
		return 2
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "xo asmdump: %v\n", err)
		return 1
	}
	opts := ssa.Options{LineRoot: wd, Tests: tests}
	runs := [][]string{dirs}
	if each {
		runs = nil
		for _, d := range dirs {
			runs = append(runs, []string{d})
		}
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	if repeat > 0 {
		var total time.Duration
		lines := 0
		for _, r := range runs {
			res, ok, err := build.CheckNative(r, false)
			if err != nil || !ok {
				continue
			}
			for range repeat {
				asm, took, err := build.Arm64AsmDump(res, opts, goos, noInline)
				if err != nil {
					break
				}
				total += took
				lines += strings.Count(string(asm), "\n")
			}
		}
		fmt.Fprintf(w, "emitted %d runs x %d: %d lines in %d ms\n", len(runs), repeat, lines, total.Milliseconds())
		return 0
	}
	for _, r := range runs {
		if each {
			fmt.Fprintf(w, "=== asm %s\n", r[0])
		}
		t := time.Now()
		res, ok, err := build.CheckNative(r, false)
		t = build.PhaseLap("check", t)
		if err != nil {
			fmt.Fprintf(w, "== error %v\n", err)
			continue
		}
		if !ok {
			w.WriteString("== rejected\n")
			continue
		}
		asm, _, err := build.Arm64AsmDump(res, opts, goos, noInline)
		if err != nil {
			fmt.Fprintf(w, "== error %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
			continue
		}
		t = time.Now()
		w.Write(asm)
		w.Flush()
		build.PhaseLap("print", t)
	}
	return 0
}
