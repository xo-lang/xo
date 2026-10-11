package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
)

// moduleFlags removes the module flags every command accepts (decision
// 0086) from args, before a `--`: `--offline` (XO_OFFLINE=1) and
// `--workspace=off|on` (XO_WORK=off). It returns the remaining args and a
// function restoring the environment.
func moduleFlags(args []string) ([]string, func(), error) {
	var out []string
	set := map[string]string{}
	for i, a := range args {
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		switch {
		case a == "--offline" || a == "-offline" || a == "--offline=true":
			set["XO_OFFLINE"] = "1"
		case a == "--offline=false":
			set["XO_OFFLINE"] = ""
		case strings.HasPrefix(a, "--workspace=") || strings.HasPrefix(a, "-workspace="):
			v := a[strings.Index(a, "=")+1:]
			switch v {
			case "off":
				set["XO_WORK"] = "off"
			case "on", "auto":
				set["XO_WORK"] = ""
			default:
				return nil, func() {}, fmt.Errorf("--workspace=%s: want off or on", v)
			}
		default:
			out = append(out, a)
		}
	}
	old := map[string]*string{}
	for k, v := range set {
		if prev, ok := os.LookupEnv(k); ok {
			old[k] = &prev
		} else {
			old[k] = nil
		}
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
	}
	restore := func() {
		for k, v := range old {
			if v == nil {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, *v)
			}
		}
	}
	return out, restore, nil
}

// printProblems prints module problems as diagnostics.
func printProblems(ps []*mod.Problem, asJSON bool, stdout, stderr io.Writer) {
	ds := problemDiags(ps)
	if asJSON {
		return
	}
	printDiags(ds, stderr)
}

func problemDiags(ps []*mod.Problem) []diag.Diagnostic {
	ds := []diag.Diagnostic{}
	for _, p := range ps {
		line := p.Line
		if line < 1 {
			line = 1
		}
		d := diag.Diagnostic{Code: string(p.Code), Severity: diag.SeverityOf(p.Code), Message: p.Msg, Span: diag.Span{File: p.File, Line: line, Col: 1}}
		for _, n := range p.Notes {
			d.Notes = append(d.Notes, diag.NoteItem{Message: n})
		}
		ds = append(ds, d)
	}
	return ds
}

// modRootOf is the main module directory for a module command.
func modRootOf(dir, cmd string, stderr io.Writer) (string, bool) {
	root := mod.ModRoot(dir)
	if root == "" {
		fmt.Fprintf(stderr, "xo %s: no xo.mod in %s or above\n", cmd, dir)
		return "", false
	}
	return root, true
}

func encodeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// runGet is `xo get <path>[@version]...`.
func runGet(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("get", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "a directory of the main module")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	update := fl.Bool("u", false, "upgrade to the latest versions (every requirement when no path is given)")
	accept := fl.Bool("accept-effects", false, "record the new versions' effects in xo.lock even when they are wider than the locked ones (decision 0156)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	root, ok := modRootOf(*dir, "get", stderr)
	if !ok {
		return 2
	}
	targets := fl.Args()
	if len(targets) == 0 && *update {
		f, probs := mod.ReadFile(root)
		if len(probs) > 0 {
			return reportChanges("get", nil, probs, *asJSON, stdout, stderr)
		}
		for _, r := range f.Requires {
			targets = append(targets, r.Path+"@latest")
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(stderr, "xo get: no module path given (xo get <path>[@version], or xo get -u)")
		return 2
	}
	var changes []mod.Change
	var probs []*mod.Problem
	for _, a := range targets {
		cs, ps := mod.GetAccept(root, a, *accept)
		changes = append(changes, cs...)
		probs = append(probs, ps...)
		if len(ps) > 0 {
			break
		}
	}
	return reportChanges("get", changes, probs, *asJSON, stdout, stderr, upgradeDiffs(root, changes)...)
}

// reportChanges prints requirement changes; diffs are the API and effect
// changes of upgraded or downgraded modules (xo get).
func reportChanges(cmd string, changes []mod.Change, probs []*mod.Problem, asJSON bool, stdout, stderr io.Writer, diffs ...apiChanges) int {
	if changes == nil {
		changes = []mod.Change{}
	}
	if asJSON {
		encodeJSON(stdout, struct {
			Ok          bool              `json:"ok"`
			Changes     []mod.Change      `json:"changes"`
			API         []apiChanges      `json:"api,omitempty"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}{len(probs) == 0, changes, diffs, problemDiags(probs)})
	} else {
		defer printAPIChanges(diffs, stdout)
		printProblems(probs, false, stdout, stderr)
		for _, c := range changes {
			switch {
			case c.Old == "":
				fmt.Fprintf(stdout, "added %s %s\n", c.Path, c.New)
			case c.New == "":
				fmt.Fprintf(stdout, "removed %s %s\n", c.Path, c.Old)
			default:
				fmt.Fprintf(stdout, "%s %s => %s\n", c.Path, c.Old, c.New)
			}
		}
	}
	if len(probs) > 0 {
		return 1
	}
	return 0
}

// runMod is `xo mod tidy` (and `xo mod clean`, which empties the module
// cache).
func runMod(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "xo mod: want a subcommand: tidy, accept-effects, clean, proxy")
		return 2
	}
	switch args[0] {
	case "tidy":
		fl := flag.NewFlagSet("mod tidy", flag.ContinueOnError)
		fl.SetOutput(stderr)
		dir := fl.String("dir", ".", "a directory of the main module")
		asJSON := fl.Bool("json", false, "print the result as JSON")
		if err := fl.Parse(args[1:]); err != nil {
			return 2
		}
		root, ok := modRootOf(*dir, "mod tidy", stderr)
		if !ok {
			return 2
		}
		changes, probs := mod.Tidy(root)
		return reportChanges("mod tidy", changes, probs, *asJSON, stdout, stderr)
	case "accept-effects":
		return runAcceptEffects(args[1:], stdout, stderr)
	case "clean":
		if err := mod.CleanCache(); err != nil {
			fmt.Fprintf(stderr, "xo mod clean: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "removed %s\n", mod.CacheDir())
		return 0
	case "proxy":
		return runModProxy(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "xo mod: unknown subcommand %q (want tidy, accept-effects, clean, or proxy)\n", args[0])
	return 2
}

// runAcceptEffects is `xo mod accept-effects [--dir=<dir>] [--json]
// [<module path>...]` (decision 0156): record the current effects of the
// named modules (all when none is named) in xo.lock, accepting effects
// wider than the locked ones, and print the lines that changed.
func runAcceptEffects(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("mod accept-effects", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "a directory of the main module")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	root, ok := modRootOf(*dir, "mod accept-effects", stderr)
	if !ok {
		return 2
	}
	changes, probs := mod.AcceptEffects(root, fl.Args())
	if changes == nil {
		changes = []mod.EffectChange{}
	}
	if *asJSON {
		encodeJSON(stdout, struct {
			Ok          bool               `json:"ok"`
			Effects     []mod.EffectChange `json:"effects"`
			Diagnostics []diag.Diagnostic  `json:"diagnostics"`
		}{len(probs) == 0, changes, problemDiags(probs)})
	} else {
		printProblems(probs, false, stdout, stderr)
		for _, c := range changes {
			old := "unlocked"
			if c.Old != nil {
				old = mod.EffectsText(c.Old)
			}
			fmt.Fprintf(stdout, "%s effects %s => %s\n", c.Path, old, mod.EffectsText(c.New))
		}
	}
	if len(probs) > 0 {
		return 1
	}
	return 0
}

// runModProxy is `xo mod proxy serve [--addr=host:port] [--fetch] [dir]`
// (decision 0126): serve a module cache read only over the module proxy
// protocol until the process is stopped.
func runModProxy(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "serve" {
		fmt.Fprintln(stderr, "xo mod proxy: want `serve [--addr=127.0.0.1:7070] [--fetch] [<cache dir>]`")
		return 2
	}
	fl := flag.NewFlagSet("mod proxy serve", flag.ContinueOnError)
	fl.SetOutput(stderr)
	addr := fl.String("addr", "127.0.0.1:7070", "the address to listen on")
	fetch := fl.Bool("fetch", false, "fetch module versions the cache lacks from git (a caching proxy)")
	if err := fl.Parse(args[1:]); err != nil {
		return 2
	}
	if fl.NArg() > 1 {
		fmt.Fprintln(stderr, "xo mod proxy serve: want at most one cache directory")
		return 2
	}
	dir := mod.CacheDir()
	if fl.NArg() == 1 {
		abs, err := filepath.Abs(fl.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "xo mod proxy serve: %v\n", err)
			return 2
		}
		dir = abs
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		if !*fetch {
			fmt.Fprintf(stderr, "xo mod proxy serve: %s is not a directory\n", dir)
			return 2
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(stderr, "xo mod proxy serve: %v\n", err)
			return 1
		}
	}
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "xo mod proxy serve: %v\n", err)
		return 1
	}
	mode := "read only"
	if *fetch {
		mode = "filling it from git"
	}
	fmt.Fprintf(stdout, "serving the module cache %s (%s) at http://%s\nclients: XO_PROXY=http://%s\n", dir, mode, l.Addr(), l.Addr())
	if err := http.Serve(l, mod.ProxyHandler(dir, *fetch)); err != nil {
		fmt.Fprintf(stderr, "xo mod proxy serve: %v\n", err)
		return 1
	}
	return 0
}

// runVendor is `xo vendor`.
func runVendor(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("vendor", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "a directory of the main module")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	root, ok := modRootOf(*dir, "vendor", stderr)
	if !ok {
		return 2
	}
	mods, probs := mod.Vendor(root)
	if mods == nil {
		mods = []mod.Module{}
	}
	if *asJSON {
		encodeJSON(stdout, struct {
			Ok          bool              `json:"ok"`
			Modules     []mod.Module      `json:"modules"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}{len(probs) == 0, mods, problemDiags(probs)})
	} else {
		printProblems(probs, false, stdout, stderr)
		for _, m := range mods {
			fmt.Fprintf(stdout, "vendored %s %s\n", m.Path, m.Version)
		}
	}
	if len(probs) > 0 {
		return 1
	}
	return 0
}

// workspaceRelease is the release check of `xo build --release` and
// `--ci` (decisions 0086, 0099): no workspace may override a required
// module.
func workspaceRelease(dirs []string) []diag.Diagnostic {
	if mod.WorkOff() || len(dirs) == 0 {
		return nil
	}
	root := mod.ModRoot(dirs[0])
	if root == "" {
		return nil
	}
	w, _ := mod.FindWork(root)
	if w == nil {
		return nil
	}
	b := mod.BuildList{Work: w}
	for _, m := range w.Modules {
		if m.Dir != root {
			b.Modules = append(b.Modules, mod.Module{Path: m.Path, Dir: m.Dir, Workspace: true})
		}
	}
	over := b.Overrides()
	if len(over) == 0 {
		return nil
	}
	d := diag.Diagnostic{Code: string(diag.WorkspaceRelease), Severity: diag.Error,
		Message: fmt.Sprintf("release build while the workspace %s overrides %s", w.File, strings.Join(over, ", ")),
		Span:    diag.Span{File: w.File, Line: 1, Col: 1}}
	d.Notes = append(d.Notes, diag.NoteItem{Message: "build with --workspace=off (or XO_WORK=off) to use the versions xo.mod requires and xo.lock pins"})
	return []diag.Diagnostic{d}
}
