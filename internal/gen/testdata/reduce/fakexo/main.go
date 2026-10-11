// Command fakexo stands in for xo in TestReduceDiffFake: builds cost
// nothing, so the test exercises the passes of scripts/reduce-diff.py.
//
// `fakexo build [--backend=B] [--no-gc] -o OUT .` succeeds when main.xo
// has a main and balanced braces and parentheses. OUT is a link to
// fakexo, and OUT.txt what it prints when run without arguments: "ok",
// plus "xor" for the backend `bad` when the program has ` ^ `.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "build" {
		b, err := os.ReadFile(os.Args[0] + ".txt")
		if err != nil {
			os.Exit(2)
		}
		os.Stdout.Write(b)
		return
	}
	be, out := "go", ""
	for i := 2; i < len(os.Args); i++ {
		a := os.Args[i]
		switch {
		case strings.HasPrefix(a, "--backend="):
			be = strings.TrimPrefix(a, "--backend=")
		case a == "-o" && i+1 < len(os.Args):
			out = os.Args[i+1]
			i++
		}
	}
	b, err := os.ReadFile("main.xo")
	src := string(b)
	if err != nil || out == "" || !strings.Contains(src, "fn main") ||
		strings.Count(src, "{") != strings.Count(src, "}") ||
		strings.Count(src, "(") != strings.Count(src, ")") {
		fmt.Fprintln(os.Stderr, "fakexo: does not build")
		os.Exit(1)
	}
	text := "ok\n"
	if be == "bad" && strings.Contains(src, " ^ ") {
		text += "xor\n"
	}
	self, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	if err := os.Link(self, out); err != nil {
		// another file system: copy
		b, err := os.ReadFile(self)
		if err != nil || os.WriteFile(out, b, 0o755) != nil {
			os.Exit(1)
		}
	}
	if err := os.WriteFile(out+".txt", []byte(text), 0o644); err != nil {
		os.Exit(1)
	}
}
