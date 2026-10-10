package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/parser"
)

// runASTDump is the hidden `xo astdump [--sig] <files...>`: it prints each
// file's syntax tree in the canonical text form of ast.Text, then its
// diagnostics, the same format selfhost/parser prints, so the Go parser
// and the Xo parser can be compared byte for byte (decision 0127). --sig
// parses in signature mode (std declaration files). It exits 1 only when a
// file cannot be read. With --bench=N it parses every file N times and
// prints the time instead, in the same form as `selfhost/parser --bench=N`.
func runASTDump(args []string, stdout, stderr io.Writer) int {
	repeat := 0
	var mode parser.Mode
	var files []string
	for _, a := range args {
		if n, ok := strings.CutPrefix(a, "--bench="); ok {
			v, err := strconv.Atoi(n)
			if err != nil {
				fmt.Fprintln(stderr, "xo astdump: --bench needs a number")
				return 2
			}
			repeat = v
			continue
		}
		if a == "--sig" {
			mode |= parser.SignatureMode
			continue
		}
		files = append(files, a)
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "usage: xo astdump [--sig] [--bench=N] <files...>")
		return 2
	}
	srcs := make([][]byte, len(files))
	for i, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "xo astdump: %v\n", err)
			return 1
		}
		srcs[i] = src
	}
	if repeat > 0 {
		t0 := time.Now()
		decls := 0
		for range repeat {
			for i, src := range srcs {
				f, _ := parser.ParseFile(files[i], src, mode)
				decls += len(f.Decls)
			}
		}
		fmt.Fprintf(stdout, "parsed %d files x %d: %d decls in %d ms\n", len(srcs), repeat, decls, time.Since(t0).Milliseconds())
		return 0
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for i, src := range srcs {
		w.WriteString(parser.Dump(files[i], src, mode))
	}
	return 0
}
