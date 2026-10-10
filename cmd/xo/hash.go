package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/defhash"
	"github.com/xo-lang/xo/internal/mod"
)

// queryHash is `xo query hash <name>` (core 4.3, decision 0125): the
// definition hashes of the module holding dir. It needs no check run.
func queryHash(dir, name string, asJSON bool, stdout, stderr io.Writer) int {
	root := mod.ModRoot(dir)
	modPath := ""
	if root != "" {
		if f, _ := mod.ReadFile(root); f != nil {
			modPath = f.Path
		}
	}
	if root == "" || modPath == "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(stderr, "xo query hash: %v\n", err)
			return 2
		}
		root, modPath = abs, filepath.Base(abs)
	}
	defs, err := defhash.Module(root, modPath)
	if err != nil {
		fmt.Fprintf(stderr, "xo query hash: %v\n", err)
		return 2
	}
	found := defhash.Find(defs, name)
	if len(found) == 0 {
		msg := fmt.Sprintf("%q does not name a definition of the module %s", name, modPath)
		if near := defhash.Nearest(defs, name, 3); len(near) > 0 {
			msg += " (did you mean " + strings.Join(near, ", ") + "?)"
		}
		fmt.Fprintf(stderr, "xo query hash: %s\n", msg)
		return 1
	}
	if asJSON {
		if err := writeJSON(stdout, struct {
			List []defhash.Def `json:"list"`
		}{found}); err != nil {
			fmt.Fprintf(stderr, "xo query: %v\n", err)
			return 2
		}
		return 0
	}
	for _, d := range found {
		fmt.Fprintf(stdout, "%s  %s %s (%s:%d)\n  own %s\n", d.Hash, d.Kind, d.ID, d.File, d.Line, d.Own)
	}
	return 0
}
