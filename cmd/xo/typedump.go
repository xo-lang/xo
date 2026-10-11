package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/check"
)

// runTypeDump is the hidden `xo typedump [--section=S] [--each]
// [--bench=N] <dirs...>`: it checks the modules under dirs and prints what
// the checker resolved in the canonical text form of check.Result.Dump
// (decision 0141), the form the self hosted checker (selfhost/check)
// prints, so the two can be compared a section at a time. --section is a
// comma separated list of names, types, sigs, diags, and full (default
// all).
// --each checks every dir as a run of its own, each preceded by
// `=== check <dir>`. --native checks as a native build does, with the
// implementation modules of std/native (decision 0074). --bench=N checks
// N times and prints the time instead.
func runTypeDump(args []string, stdout, stderr io.Writer) int {
	repeat := 0
	each := false
	native := false
	sec := check.AllSections
	var dirs []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "--bench="):
			v, err := strconv.Atoi(strings.TrimPrefix(a, "--bench="))
			if err != nil {
				fmt.Fprintln(stderr, "xo typedump: --bench needs a number")
				return 2
			}
			repeat = v
		case strings.HasPrefix(a, "--section="):
			s, err := check.ParseSections(strings.TrimPrefix(a, "--section="))
			if err != nil {
				fmt.Fprintf(stderr, "xo typedump: %v\n", err)
				return 2
			}
			sec = s
		case a == "--each":
			each = true
		case a == "--native":
			native = true
		default:
			dirs = append(dirs, a)
		}
	}
	if len(dirs) == 0 {
		fmt.Fprintln(stderr, "usage: xo typedump [--section=names,types,sigs,diags,full] [--each] [--native] [--bench=N] <dirs...>")
		return 2
	}
	runs := [][]string{dirs}
	if each {
		runs = nil
		for _, d := range dirs {
			runs = append(runs, []string{d})
		}
	}
	if repeat > 0 {
		t0 := time.Now()
		mods := 0
		for range repeat {
			for _, r := range runs {
				res, err := check.CheckOpts(r, check.Options{Native: native})
				if err != nil {
					fmt.Fprintf(stderr, "xo typedump: %v\n", err)
					return 1
				}
				mods += len(res.Modules)
			}
		}
		fmt.Fprintf(stdout, "checked %d runs x %d: %d modules in %d ms\n", len(runs), repeat, mods, time.Since(t0).Milliseconds())
		return 0
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for _, r := range runs {
		if each {
			fmt.Fprintf(w, "=== check %s\n", r[0])
		}
		res, err := check.CheckOpts(r, check.Options{Native: native})
		if err != nil {
			fmt.Fprintf(w, "== error %v\n", err)
			continue
		}
		w.WriteString(res.Dump(sec))
	}
	return 0
}
