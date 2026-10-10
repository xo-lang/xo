// Command xoplay is the compiler half of the playground on xolang.dev
// (decision 0144). It is built for GOOS=wasip1 and runs in a Web Worker
// over an in-memory file system, so a visitor's code never leaves their
// browser. It has three commands, each printing one JSON object:
//
//	xoplay check DIR        diagnostics of the module in DIR
//	xoplay fmt FILE         FILE reformatted in place
//	xoplay gen [--test] DIR OUT
//	                        the Go source of the program (or with --test,
//	                        the test binary) in DIR, written
//	                        under OUT, the packages to compile in order,
//	                        and the files it needs in package rt
//
// The browser compiles the output of gen with the Go compiler and linker
// (also built for wasip1) against precompiled archives of package rt and
// the Go standard library. See knowledgebase/infrastructure/playground.md.
package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/gen"
)

func main() {
	out, err := run(os.Args[1:])
	if err != nil {
		out = map[string]any{"error": err.Error()}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.Encode(out)
	if err != nil {
		os.Exit(1)
	}
}

func run(args []string) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: xoplay check DIR | fmt FILE | gen [--test] DIR OUT")
	}
	switch {
	case args[0] == "check" && len(args) == 2:
		res, err := check.Check(args[1:])
		if err != nil {
			return nil, err
		}
		return diagnostics(res.Diagnostics), nil
	case args[0] == "fmt" && len(args) == 2:
		src, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		out, err := format.Source(args[1], src)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(args[1], out, 0o644); err != nil {
			return nil, err
		}
		return map[string]any{"changed": string(out) != string(src)}, nil
	case args[0] == "gen" && len(args) == 3:
		return generate(args[1], args[2], gen.Build)
	case args[0] == "gen" && len(args) == 4 && args[1] == "--test":
		return generate(args[2], args[3], gen.Test)
	}
	return nil, fmt.Errorf("usage: xoplay check DIR | fmt FILE | gen [--test] DIR OUT")
}

func diagnostics(ds []diag.Diagnostic) map[string]any {
	if ds == nil {
		ds = []diag.Diagnostic{}
	}
	return map[string]any{"diagnostics": ds}
}

// pkg is one Go package of the generated program, to compile with
// `compile -p Path -importcfg ... Files`.
type pkg struct {
	Path    string   `json:"path"`
	Files   []string `json:"files"`
	Imports []string `json:"imports"`
}

// generate lowers the program in dir and writes its Go files under out.
// The result lists the program's packages in dependency order (package
// main last) and the Go build tags it needs; the browser has archives for
// exactly one rt variant, so it refuses a program whose tags differ.
func generate(dir, out string, mode gen.Mode) (any, error) {
	opts := build.Options{Mode: mode}
	res, prog, err := build.Generate([]string{dir}, opts)
	if err != nil {
		return nil, err
	}
	if prog == nil {
		return diagnostics(res.Diagnostics), nil
	}
	if mode == gen.Test && prog.Tags == nil {
		// xo test links net/http into every test binary; a test binary
		// that never loads std/http cannot reach it either
		if !gen.ReachesStd(res.Check, "std/http") {
			prog.Tags = []string{"xo_nohttp"}
		}
	}
	byDir := map[string]*pkg{}
	runtime := []string{}
	for p, src := range prog.Files {
		if p == "go.mod" {
			continue
		}
		if strings.HasPrefix(p, "rt/") {
			// `use go` adapters: the browser's precompiled rt has them
			// for the packages it supports and refuses the others
			runtime = append(runtime, path.Base(p))
			continue
		}
		if !strings.HasSuffix(p, ".go") {
			return nil, fmt.Errorf("the playground cannot build this program: it needs %s (embedded data)", p)
		}
		full := filepath.Join(out, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, src, 0o644); err != nil {
			return nil, err
		}
		d := path.Dir(p)
		k := byDir[d]
		if k == nil {
			k = &pkg{Path: "main"}
			if d != "." {
				k.Path = gen.ModulePath + "/" + d
			}
			byDir[d] = k
		}
		k.Files = append(k.Files, full)
		f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, im := range f.Imports {
			ip, _ := strconv.Unquote(im.Path.Value)
			k.Imports = append(k.Imports, ip)
		}
	}
	byPath := map[string]*pkg{}
	for _, k := range byDir {
		sort.Strings(k.Files)
		k.Imports = uniq(k.Imports)
		byPath[k.Path] = k
	}
	// depth first, dependencies before their importers
	var order []*pkg
	state := map[string]int{}
	var visit func(k *pkg) error
	visit = func(k *pkg) error {
		switch state[k.Path] {
		case 1:
			return fmt.Errorf("import cycle through %s", k.Path)
		case 2:
			return nil
		}
		state[k.Path] = 1
		for _, ip := range k.Imports {
			if d := byPath[ip]; d != nil {
				if err := visit(d); err != nil {
					return err
				}
			}
		}
		state[k.Path] = 2
		order = append(order, k)
		return nil
	}
	var paths []string
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := visit(byPath[p]); err != nil {
			return nil, err
		}
	}
	tags := prog.Tags
	if tags == nil {
		tags = []string{}
	}
	ds := res.Diagnostics
	if ds == nil {
		ds = []diag.Diagnostic{}
	}
	sort.Strings(runtime)
	return map[string]any{"diagnostics": ds, "packages": order, "tags": tags, "runtime": runtime}, nil
}

func uniq(s []string) []string {
	sort.Strings(s)
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}
