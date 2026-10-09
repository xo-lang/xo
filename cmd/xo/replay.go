package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// traceHeader is the first line of a trace (std/rt/replay.go).
type traceHeader struct {
	Xotrace int         `json:"xotrace"`
	Mode    string      `json:"mode"`
	Args    []string    `json:"args"`
	Target  string      `json:"target"`
	Build   *build.Info `json:"build"`
}

// readTraceHeader reads the header of a .xotrace file or of the trace in
// a .xocrash bundle.
func readTraceHeader(path string) (*traceHeader, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var first []byte
	bundle := false
	var b struct {
		Kind  string            `json:"kind"`
		Trace []json.RawMessage `json:"trace"`
		Build *build.Info       `json:"build"`
	}
	if json.Unmarshal(data, &b) == nil && b.Kind != "" {
		bundle = true
		if len(b.Trace) == 0 {
			if b.Build == nil {
				return nil, true, fmt.Errorf("%s has no effect trace and no build information", path)
			}
			return nil, true, fmt.Errorf("%s has no effect trace: the run was not recorded (xo run --record) and kept no ring buffer (XO_RING=N)", path)
		}
		first = b.Trace[0]
	} else {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 1<<30)
		if sc.Scan() {
			first = sc.Bytes()
		}
	}
	var h traceHeader
	if json.Unmarshal(first, &h) != nil || h.Xotrace == 0 {
		return nil, bundle, fmt.Errorf("%s is not an Xo trace (.xotrace) or crash bundle (.xocrash)", path)
	}
	return &h, bundle, nil
}

// runReplay rebuilds the program of a trace or crash bundle and runs it
// with every effect result and interleaving taken from the trace (core
// 11.3).
func runReplay(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("replay", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", "", "the program's main module directory (default: the one recorded in the trace)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, "xo replay: give one .xotrace or .xocrash file")
		return 2
	}
	path, err := filepath.Abs(fl.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "xo replay: %v\n", err)
		return 2
	}
	h, _, err := readTraceHeader(path)
	if err != nil {
		fmt.Fprintf(stderr, "xo replay: %v\n", err)
		return 2
	}
	if h.Build == nil {
		fmt.Fprintln(stderr, "xo replay: the trace has no build information (the program was not built by xo build or xo run)")
		return 2
	}
	src := h.Build.Dir
	if *dir != "" {
		src = *dir
	}
	res, err := build.Build([]string{src}, build.Options{Mode: gen.Build, Holes: h.Build.Holes, NoMono: h.Build.NoMono, NoArenas: h.Build.NoArenas, Race: h.Build.Race, TZData: h.Build.TZData, Release: h.Build.Release})
	if err != nil {
		fmt.Fprintf(stderr, "xo replay: %v\n", err)
		return 1
	}
	printDiags(res.Diagnostics, stderr)
	if res.Failed {
		return 1
	}
	if changed := sourceChanges(h.Build.Sources, res.Info.Sources); len(changed) > 0 {
		fmt.Fprintf(stderr, "xo replay: warning: the sources changed since the recording (build %s, now %s):\n", h.Build.ID, res.Info.ID)
		for _, c := range changed {
			fmt.Fprintln(stderr, "  "+c)
		}
	}
	if host := build.Host().String(); h.Target != "" && h.Target != host {
		fmt.Fprintf(stderr, "xo replay: note: recorded on %s, replayed on %s\n", h.Target, host)
	}
	// offline: no standard input; every effect result comes from the trace
	return execBinaryIn(res.Binaries[build.Host()], h.Args, nil, stdout, stderr, "XO_REPLAY="+path)
}

// sourceChanges lists the differences between two source hash maps.
func sourceChanges(was, now map[string]string) []string {
	var out []string
	for p, h := range was {
		switch n, ok := now[p]; {
		case !ok:
			out = append(out, p+" (removed)")
		case n != h:
			out = append(out, p+" (changed)")
		}
	}
	for p := range now {
		if _, ok := was[p]; !ok {
			out = append(out, p+" (added)")
		}
	}
	sort.Strings(out)
	return out
}
