package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/xo-lang/xo/internal/lexer"
)

// runLexDump is the hidden `xo lexdump <files...>`: it prints each file's
// tokens, comments, and lexical diagnostics in the canonical format of
// lexer.Dump, the same format selfhost/lexer prints, so the Go lexer and
// the Xo lexer can be compared byte for byte. It exits 1 only when a file
// cannot be read; lexical errors are part of the output.
func runLexDump(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: xo lexdump <files...>")
		return 2
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for _, path := range args {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "xo lexdump: %v\n", err)
			return 1
		}
		w.WriteString(lexer.Dump(path, src))
	}
	return 0
}
