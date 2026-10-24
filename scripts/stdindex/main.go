// Command stdindex writes spec/std.md's prelude index from
// std/prelude.xo (internal/stdindex): run it after changing the prelude.
//
//	go run ./scripts/stdindex
//
// go run ./scripts/stdsync does this and the other files a prelude change
// derives (the habit goldens and std/rt's generated files); this command
// stays for the index alone.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/xo-lang/xo/internal/stdindex"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	root, err := xotool.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdindex:", err)
		os.Exit(1)
	}
	prelude, err := os.ReadFile(filepath.Join(root, "std", "prelude.xo"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdindex:", err)
		os.Exit(1)
	}
	specPath := filepath.Join(root, "spec", "std.md")
	spec, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdindex:", err)
		os.Exit(1)
	}
	out := stdindex.Splice(string(spec), stdindex.Index(string(prelude)))
	if out == string(spec) {
		fmt.Println("spec/std.md: the prelude index is current")
		return
	}
	if err := os.WriteFile(specPath, []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "stdindex:", err)
		os.Exit(1)
	}
	fmt.Println("spec/std.md: prelude index written")
}
