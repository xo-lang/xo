package main

import (
	"bufio"
	"fmt"
	"go/format"
	"io"
	"os"
	"strings"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// runGenDump is the hidden `xo gendump [--test] [--no-mono] [--no-ranges]
// [--no-arenas] [--release] [--no-error-args] [--each] <dirs...>`
// (decision 0167): it checks the modules under dirs as `xo build` does for
// the Go backend and prints the files gen.Generate makes (not the build
// information or the zone data internal/build adds), sorted by path, each
// after a line `== file <path>`. --each generates every dir as a run of
// its own, each preceded by `=== gen <dir>`; a run the checker rejects
// prints `== rejected`, one the generator fails on `== error <why>`.
// --test generates the test binary (gen.Test). `xo gendump --fmt` reads
// the output of selfhost/gen on standard input and puts its Go files
// through internal/gen's last step (FormatGenDump).
func runGenDump(args []string, stdout, stderr io.Writer) int {
	each := false
	opts := gen.Options{Mode: gen.Build, Arenas: true}
	var dirs []string
	for _, a := range args {
		switch a {
		case "--each":
			each = true
		case "--test":
			opts.Mode = gen.Test
		case "--no-mono":
			opts.NoMono = true
		case "--no-ranges":
			opts.NoRanges = true
		case "--no-arenas":
			opts.Arenas = false
		case "--release":
			opts.Release = true
		case "--no-error-args":
			opts.NoErrorArgs = true
		case "--fmt":
			// the output of selfhost/gen on standard input, with the Go
			// files put through internal/gen's last step (FormatFile)
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			io.WriteString(stdout, FormatGenDump(string(raw)))
			return 0
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "xo gendump: unknown flag %s\n", a)
				return 2
			}
			dirs = append(dirs, a)
		}
	}
	if len(dirs) == 0 {
		fmt.Fprintln(stderr, "usage: xo gendump [--test] [--no-mono] [--no-ranges] [--no-arenas] [--each] <dirs...>")
		return 2
	}
	runs := [][]string{dirs}
	if each {
		runs = nil
		for _, d := range dirs {
			runs = append(runs, []string{d})
		}
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for _, r := range runs {
		if each {
			fmt.Fprintf(w, "=== gen %s\n", r[0])
		}
		w.WriteString(build.GoGenDump(r, opts))
	}
	return 0
}

// FormatGenDump puts the Go files of a selfhost/gen dump (sections after
// `== file <path>.go`) through gen.FormatFile; a file that does not
// format is left as it is, after a line `== unformatted <why>`. After a
// line `== prepped` (selfhost/gen --prep: the Xo generator applied that
// step itself) the files only go through gofmt.
func FormatGenDump(text string) string {
	var b strings.Builder
	lines := strings.SplitAfter(text, "\n")
	prepped := false
	for i := 0; i < len(lines); {
		l := lines[i]
		if strings.HasPrefix(l, "=== gen ") {
			prepped = false
		}
		if l == "== prepped\n" {
			prepped = true
			i++
			continue
		}
		if !strings.HasPrefix(l, "== file ") || !strings.HasSuffix(strings.TrimSpace(l), ".go") {
			b.WriteString(l)
			i++
			continue
		}
		b.WriteString(l)
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "== ") && !strings.HasPrefix(lines[j], "=== gen ") {
			j++
		}
		body := strings.Join(lines[i+1:j], "")
		var out []byte
		var err error
		if prepped {
			out, err = format.Source([]byte(body))
		} else {
			out, err = gen.FormatFile([]byte(body))
		}
		if err != nil {
			b.WriteString("== unformatted " + strings.ReplaceAll(err.Error(), "\n", " ") + "\n")
			b.WriteString(body)
		} else {
			b.Write(out)
		}
		i = j
	}
	return b.String()
}
