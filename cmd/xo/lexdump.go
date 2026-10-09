package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
)

// runLexDump is the hidden `xo lexdump <files...>`: it prints each file's
// tokens, comments, and lexical diagnostics in the canonical format of
// lexer.Dump, the same format selfhost/lexer prints, so the Go lexer and
// the Xo lexer can be compared byte for byte. It exits 1 only when a file
// cannot be read; lexical errors are part of the output. With --bench=N it
// lexes every file N times and prints the time instead, in the same form
// as `selfhost/lexer --bench=N`.
func runLexDump(args []string, stdout, stderr io.Writer) int {
	repeat := 0
	var files []string
	for _, a := range args {
		if n, ok := strings.CutPrefix(a, "--bench="); ok {
			v, err := strconv.Atoi(n)
			if err != nil {
				fmt.Fprintln(stderr, "xo lexdump: --bench needs a number")
				return 2
			}
			repeat = v
			continue
		}
		files = append(files, a)
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "usage: xo lexdump [--bench=N] <files...>")
		return 2
	}
	srcs := make([][]byte, len(files))
	for i, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "xo lexdump: %v\n", err)
			return 1
		}
		srcs[i] = src
	}
	if repeat > 0 {
		t0 := time.Now()
		toks := 0
		for range repeat {
			for i, src := range srcs {
				var ds diag.List
				toks += len(lexer.Lex(files[i], src, &ds).Tokens)
			}
		}
		fmt.Fprintf(stdout, "lexed %d files x %d: %d tokens in %d ms\n", len(srcs), repeat, toks, time.Since(t0).Milliseconds())
		return 0
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for i, src := range srcs {
		w.WriteString(lexer.Dump(files[i], src))
	}
	return 0
}
