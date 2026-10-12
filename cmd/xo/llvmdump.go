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
	"github.com/xo-lang/xo/internal/llvm"
)

// runLLVMDump is the hidden `xo llvmdump [--each] [--test] [--func=NAME]
// [--bench=N] <dirs...>` (decision 0166): it checks the modules under
// dirs as a native build does and prints the LLVM IR the LLVM backend
// generates for them (llvm.Generate without the target's attributes),
// the text the self hosted emitter (selfhost/llvm) prints:
//
//	== ir
//	<module>
//
// A run the checker rejects prints `== rejected`, one the SSA builder or
// the emitter cannot lower `== error <message>`. Fault positions are
// relative to the working directory. --each lowers every dir as a run of
// its own, each preceded by `=== llvm <dir>`; --test lowers the test
// binary of `xo test --no-gc`; --func=NAME prints only the definition
// named NAME (`x.main`, `xo.dup.Str`: llvm.SplitIR's names). --bench=N
// checks and builds the SSA once and runs the emitter alone
// (llvm.GenerateSSA) N times, printing the time.
func runLLVMDump(args []string, stdout, stderr io.Writer) int {
	repeat := 0
	each := false
	tests := false
	only := ""
	var dirs []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--bench="):
			v, err := strconv.Atoi(strings.TrimPrefix(a, "--bench="))
			if err != nil {
				fmt.Fprintln(stderr, "xo llvmdump: --bench needs a number")
				return 2
			}
			repeat = v
		case strings.HasPrefix(a, "--func="):
			only = strings.TrimPrefix(a, "--func=")
		case a == "--each":
			each = true
		case a == "--test":
			tests = true
		default:
			dirs = append(dirs, a)
		}
	}
	if len(dirs) == 0 {
		fmt.Fprintln(stderr, "usage: xo llvmdump [--each] [--test] [--func=NAME] [--bench=N] <dirs...>")
		return 2
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "xo llvmdump: %v\n", err)
		return 1
	}
	opts := llvm.Options{LineRoot: wd, Tests: tests}
	runs := [][]string{dirs}
	if each {
		runs = nil
		for _, d := range dirs {
			runs = append(runs, []string{d})
		}
	}
	if repeat > 0 {
		var total time.Duration
		funcs := 0
		for _, r := range runs {
			res, ok, err := build.CheckNative(r, false)
			if err != nil || !ok {
				continue
			}
			m, err := llvm.PrepareSSA(res, opts)
			if err != nil {
				continue
			}
			t0 := time.Now()
			for range repeat {
				p, err := llvm.GenerateSSA(m)
				if err == nil {
					funcs += p.Funcs
				}
			}
			total += time.Since(t0)
		}
		fmt.Fprintf(stdout, "lowered %d runs x %d: %d functions in %d ms\n", len(runs), repeat, funcs, total.Milliseconds())
		return 0
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for _, r := range runs {
		if each {
			fmt.Fprintf(w, "=== llvm %s\n", r[0])
		}
		res, ok, err := build.CheckNative(r, false)
		if err != nil {
			fmt.Fprintf(w, "== error %v\n", oneLine(err.Error()))
			continue
		}
		if !ok {
			w.WriteString("== rejected\n")
			continue
		}
		prog, err := llvm.Generate(res, opts)
		if err != nil {
			fmt.Fprintf(w, "== error %s\n", oneLine(err.Error()))
			continue
		}
		w.WriteString("== ir\n")
		if only == "" {
			w.Write(prog.IR)
			continue
		}
		parts, err := llvm.SplitIR(string(prog.IR))
		if err != nil {
			fmt.Fprintf(w, "== error %v\n", err)
			continue
		}
		w.WriteString(parts.Defs[only])
	}
	return 0
}

// oneLine joins the lines of a message (an error list) with "; ".
func oneLine(s string) string {
	return strings.Join(strings.Split(strings.TrimRight(s, "\n"), "\n"), "; ")
}
