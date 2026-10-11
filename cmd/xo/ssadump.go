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

// runSSADump is the hidden `xo ssadump [--stage=S] [--each] [--test]
// [--bench=N] <dirs...>` (decision 0150): it checks the modules under dirs as a native
// build does (with the implementation modules of std/native) and prints
// the SSA module after each stage named (ssa.StageNames: build, unwind,
// landings, cleanup, rc; default build, the builder's output before any
// pass; verify, the verifier's verdict after rc, and mutants, its
// verdicts with one retain or release removed), in the form of
// ssa.Module.String, the form the self hosted SSA
// builder (selfhost/ssa) prints:
//
//	== stage <name>
//	<module>
//	== errors
//	<pos>: <message>
//
// A run the checker rejects prints `== rejected`; the errors section is
// left out when the builder reports none. Fault positions are relative to
// the working directory. --each builds every dir as a run of its own,
// each preceded by `=== ssa <dir>`. --test builds the test binary of
// `xo test --no-gc` (ssa.Options.Tests: a function per test block and a
// main that runs the one XO_TEST names). --bench=N checks once and runs the
// builder alone (ssa.BuildRaw) N times, printing the time.
func runSSADump(args []string, stdout, stderr io.Writer) int {
	repeat := 0
	each := false
	tests := false
	want := map[string]bool{"build": true}
	var dirs []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--bench="):
			v, err := strconv.Atoi(strings.TrimPrefix(a, "--bench="))
			if err != nil {
				fmt.Fprintln(stderr, "xo ssadump: --bench needs a number")
				return 2
			}
			repeat = v
		case strings.HasPrefix(a, "--stage="):
			want = map[string]bool{}
			for _, s := range strings.Split(strings.TrimPrefix(a, "--stage="), ",") {
				ok := s == "all"
				if s == "verify" || s == "mutants" {
					// the verifier after rc, and its verdicts on the
					// module with one retain or release removed
					// (VerifyMutants); not part of all
					want[s] = true
					ok = true
				}
				for _, n := range ssa.StageNames {
					if s == n || s == "all" {
						want[n] = true
						ok = true
					}
				}
				if !ok {
					fmt.Fprintf(stderr, "xo ssadump: unknown stage %q (want %s, all, verify, or mutants)\n", s, strings.Join(ssa.StageNames, ", "))
					return 2
				}
			}
		case a == "--each":
			each = true
		case a == "--test":
			tests = true
		default:
			dirs = append(dirs, a)
		}
	}
	if len(dirs) == 0 {
		fmt.Fprintf(stderr, "usage: xo ssadump [--stage=%s] [--each] [--test] [--bench=N] <dirs...>\n", strings.Join(ssa.StageNames, ","))
		return 2
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "xo ssadump: %v\n", err)
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
	if repeat > 0 {
		var total time.Duration
		funcs := 0
		for _, r := range runs {
			res, ok, err := build.CheckNative(r, false)
			if err != nil || !ok {
				continue
			}
			t0 := time.Now()
			for range repeat {
				m, _ := ssa.BuildRaw(res, opts)
				funcs += len(m.Funcs)
			}
			total += time.Since(t0)
		}
		fmt.Fprintf(stdout, "built %d runs x %d: %d functions in %d ms\n", len(runs), repeat, funcs, total.Milliseconds())
		return 0
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for _, r := range runs {
		if each {
			fmt.Fprintf(w, "=== ssa %s\n", r[0])
		}
		res, ok, err := build.CheckNative(r, false)
		if err != nil {
			fmt.Fprintf(w, "== error %v\n", err)
			continue
		}
		if !ok {
			w.WriteString("== rejected\n")
			continue
		}
		stages, err := ssa.BuildStages(res, opts, want)
		for _, st := range stages {
			fmt.Fprintf(w, "== stage %s\n", st.Name)
			w.WriteString(st.Text)
		}
		if err != nil {
			w.WriteString("== errors\n")
			if es, ok := err.(ssa.Errors); ok {
				for _, e := range es {
					fmt.Fprintln(w, e.Error())
				}
			} else {
				fmt.Fprintln(w, err)
			}
		}
	}
	return 0
}
