package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/cbind"
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
	cMode := fl.Bool("c", false, "bind C headers for `use c` (native builds, decision 0113) instead of Go packages")
	var links multiFlag
	fl.Var(&links, "link", "with --c: a `link` library or file (repeatable)")
	uses := fl.String("uses", "", "with --c: the effects of the `use c` line (`none`, or labels separated by commas); default ffi")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *cMode {
		return runBindC(fl.Args(), *dir, links, *uses, *asJSON, stdout, stderr)
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

// multiFlag is a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// cBindResult is one header of `xo bind --c --json`.
type cBindResult struct {
	Header  string            `json:"header"`
	Path    string            `json:"path,omitempty"`
	Decl    string            `json:"decl,omitempty"`
	Bound   int               `json:"bound"`
	Total   int               `json:"total"`
	Skipped map[string]string `json:"skipped,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// runBindC is `xo bind --c`: it binds C headers (relative to --dir, or
// on the include path) into the binding cache of the project that
// contains --dir (decision 0113, internal/cbind).
func runBindC(headers []string, dir string, links []string, uses string, asJSON bool, stdout, stderr io.Writer) int {
	if len(headers) == 0 {
		fmt.Fprintln(stderr, "xo bind --c: no C headers given")
		return 2
	}
	root, err := projectRoot(dir)
	if err != nil {
		fmt.Fprintf(stderr, "xo bind --c: %v\n", err)
		return 2
	}
	var effs []string
	if uses != "" {
		effs = []string{}
		if uses != "none" {
			for _, e := range strings.Split(uses, ",") {
				effs = append(effs, strings.TrimSpace(e))
			}
		}
	}
	status := 0
	var results []cBindResult
	for _, h := range headers {
		b, err := cbind.Bind(cbind.Options{Header: h, ModDir: dir, Links: links, Uses: effs, Dir: cbind.CacheDir(root)})
		if err != nil {
			status = 1
			results = append(results, cBindResult{Header: h, Error: err.Error()})
			if !asJSON {
				fmt.Fprintf(stderr, "xo bind --c: %v\n", err)
			}
			continue
		}
		results = append(results, cBindResult{Header: h, Path: b.HeaderPath, Decl: b.DeclPath, Bound: b.Bound, Total: b.Total, Skipped: b.Skipped})
		if !asJSON {
			fmt.Fprintf(stdout, "%s (%s): functions %d/%d bound, %d skipped\n", h, b.HeaderPath, b.Bound, b.Total, len(b.Skipped))
			fmt.Fprintf(stdout, "  wrote %s\n", b.DeclPath)
		}
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
	}
	return status
}
