package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/xo-lang/xo/internal/gobind"
)

// bindResult is one package of `xo bind --json`.
type bindResult struct {
	Path     string            `json:"path"`
	Decl     string            `json:"decl,omitempty"`
	Stats    *gobind.Stats     `json:"stats,omitempty"`
	Skipped  map[string]string `json:"skipped,omitempty"`
	Error    string            `json:"error,omitempty"`
	Standard bool              `json:"standard"`
	Module   string            `json:"module,omitempty"`
}

// runBind is `xo bind`: it (re)binds Go packages into the binding cache of
// the project that contains --dir (decision 0041, internal/gobind).
func runBind(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("bind", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "a directory of the Xo project (its xo.mod root holds the cache)")
	asJSON := fl.Bool("json", false, "print the coverage of each package as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintln(stderr, "xo bind: no Go import paths given")
		return 2
	}
	root, err := projectRoot(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "xo bind: %v\n", err)
		return 2
	}
	pins := gobind.ReadPins(filepath.Join(root, "xo.mod"))
	reruns := gobind.ReadRerun(filepath.Join(root, "xo.mod"))
	cache := gobind.CacheDir(root)
	status := 0
	var results []bindResult
	for _, p := range fl.Args() {
		b, err := gobind.Bind(p, gobind.Options{Dir: cache, Version: gobind.PinFor(pins, p), Rerun: reruns[p]})
		if err != nil {
			status = 1
			results = append(results, bindResult{Path: p, Error: err.Error()})
			if !*asJSON {
				fmt.Fprintf(stderr, "xo bind: %v\n", err)
			}
			continue
		}
		st := b.Stats
		results = append(results, bindResult{Path: p, Decl: b.DeclPath, Stats: &st, Skipped: b.Skipped, Standard: b.Standard, Module: b.Module})
		if !*asJSON {
			fmt.Fprintln(stdout, b.Summary())
			fmt.Fprintf(stdout, "  wrote %s\n", b.DeclPath)
		}
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
	}
	return status
}

// projectRoot is the directory of the nearest xo.mod at or above dir, or
// dir itself.
func projectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; {
		if _, err := os.Stat(filepath.Join(d, "xo.mod")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return abs, nil
		}
		d = parent
	}
}
