package gobind

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CLIResult is one package of `xo bind --json`.
type CLIResult struct {
	Path     string            `json:"path"`
	Decl     string            `json:"decl,omitempty"`
	Stats    *Stats            `json:"stats,omitempty"`
	Skipped  map[string]string `json:"skipped,omitempty"`
	Error    string            `json:"error,omitempty"`
	Standard bool              `json:"standard"`
	Module   string            `json:"module,omitempty"`
}

// RunCLI is `xo bind` for Go packages after its flags (decision 0041): it
// (re)binds paths into the binding cache of the project that contains dir
// and prints a summary per package, or the results as JSON. It returns
// the exit status. cmd/xo runs it in process and the helper xo-gobind
// (cmd/xo-gobind, decision 0178 stage 6) runs it for the self hosted
// `xo`, so both print the same bytes.
func RunCLI(paths []string, dir string, asJSON bool, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "xo bind: no Go import paths given")
		return 2
	}
	root, err := ProjectRoot(dir)
	if err != nil {
		fmt.Fprintf(stderr, "xo bind: %v\n", err)
		return 2
	}
	pins := ReadPins(filepath.Join(root, "xo.mod"))
	reruns := ReadRerun(filepath.Join(root, "xo.mod"))
	cache := CacheDir(root)
	status := 0
	var results []CLIResult
	for _, p := range paths {
		mod, ver := PinFor(pins, p)
		b, err := Bind(p, Options{Dir: cache, Version: ver, Module: mod, Rerun: reruns[p]})
		if err != nil {
			status = 1
			results = append(results, CLIResult{Path: p, Error: err.Error()})
			if !asJSON {
				fmt.Fprintf(stderr, "xo bind: %v\n", err)
			}
			continue
		}
		st := b.Stats
		results = append(results, CLIResult{Path: p, Decl: b.DeclPath, Stats: &st, Skipped: b.Skipped, Standard: b.Standard, Module: b.Module})
		if !asJSON {
			fmt.Fprintln(stdout, b.Summary())
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

// ProjectRoot is the directory of the nearest xo.mod at or above dir, or
// dir itself (absolute).
func ProjectRoot(dir string) (string, error) {
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
