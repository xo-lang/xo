package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/release"
)

// runPublish is `xo publish [--check] [--version vX.Y.Z]` (decision 0086
// step 4): compare the exports and effects with the previous tag and
// reject a minor or patch release that breaks them. Publishing is tagging;
// it prints the tag command and never runs git tag or git push.
func runPublish(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("publish", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "a directory of the module")
	version := fl.String("version", "", "the version to release (default: the next minor, or the next patch before v1.0.0)")
	only := fl.Bool("check", false, "compare only: allow uncommitted changes and compare the working tree")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() > 0 {
		*dir = fl.Arg(0)
	}
	opts := release.Options{Version: *version, Check: *only}
	if !*only {
		opts.Verify = func(root string) []diag.Diagnostic {
			old, had := os.LookupEnv("XO_WORK")
			os.Setenv("XO_WORK", "off")
			defer func() {
				if had {
					os.Setenv("XO_WORK", old)
				} else {
					os.Unsetenv("XO_WORK")
				}
			}()
			res, err := check.Check([]string{root})
			if err != nil {
				return []diag.Diagnostic{{Code: string(diag.InvalidRelease), Severity: diag.Error, Message: "xo check: " + err.Error(), Span: diag.Span{File: filepath.Join(root, "xo.mod"), Line: 1, Col: 1}}}
			}
			return res.Diagnostics
		}
	}
	r := release.Publish(*dir, opts)
	if *asJSON {
		encodeJSON(stdout, r)
	} else {
		printDiags(r.Diags, stderr)
		prev := r.Previous
		if prev == "" {
			prev = "no earlier tag"
		}
		fmt.Fprintf(stdout, "module %s: %s (after %s; the changes need a %s release, %s)\n", r.Module, r.Version, prev, r.Bump, r.Suggested)
		for _, c := range r.Changes {
			mark := " "
			if c.Breaking {
				mark = "!"
			}
			fmt.Fprintf(stdout, "%s %s\n", mark, c.Describe())
		}
		switch {
		case r.Ok && *only:
			fmt.Fprintf(stdout, "ok: %s may be released\n", r.Version)
		case r.Ok:
			fmt.Fprintf(stdout, "ok: publish with\n  %s\n", r.Tag)
		}
	}
	if !r.Ok {
		return 1
	}
	return 0
}

// apiChanges is what one requirement change of xo get or xo mod tidy
// does to the exports and effects of the module, with the exports whose
// effects exceed the requirement's ceiling.
type apiChanges struct {
	Path         string           `json:"path"`
	Old          string           `json:"old"`
	New          string           `json:"new"`
	Changes      []release.Change `json:"changes"`
	AboveCeiling []string         `json:"above_ceiling,omitempty"`
}

// upgradeDiffs compares the exports of the old and new version of every
// changed requirement of the main module in root.
func upgradeDiffs(root string, changes []mod.Change) []apiChanges {
	var out []apiChanges
	f, _ := mod.ReadFile(root)
	for _, c := range changes {
		if c.Old == "" || c.New == "" {
			continue
		}
		oldDir, p1 := mod.Download(c.Path, c.Old)
		newDir, p2 := mod.Download(c.Path, c.New)
		if p1 != nil || p2 != nil {
			continue
		}
		oldAPI, err1 := release.ModuleAPI(oldDir, c.Path)
		newAPI, err2 := release.ModuleAPI(newDir, c.Path)
		if err1 != nil || err2 != nil {
			continue
		}
		a := apiChanges{Path: c.Path, Old: c.Old, New: c.New, Changes: release.Diff(oldAPI, newAPI)}
		if a.Changes == nil {
			a.Changes = []release.Change{}
		}
		if f != nil {
			for _, r := range f.Requires {
				if r.Path != c.Path || r.Ceiling == nil {
					continue
				}
				for _, it := range newAPI {
					for _, e := range it.Effects {
						if !r.Ceiling.Admits(e) && isLabel(e) {
							a.AboveCeiling = append(a.AboveCeiling, fmt.Sprintf("%s.%s uses %s (ceiling: %s)", it.Package, it.Name, e, r.Ceiling))
						}
					}
				}
			}
		}
		out = append(out, a)
	}
	return out
}

func isLabel(e string) bool {
	for _, l := range mod.EffectLabels {
		if l == e {
			return true
		}
	}
	return false
}

func printAPIChanges(diffs []apiChanges, w io.Writer) {
	for _, d := range diffs {
		fmt.Fprintf(w, "  %s %s => %s:\n", d.Path, d.Old, d.New)
		if len(d.Changes) == 0 && len(d.AboveCeiling) == 0 {
			fmt.Fprintf(w, "    no API or effect changes\n")
			continue
		}
		for _, c := range d.Changes {
			mark := " "
			if c.Breaking {
				mark = "!"
			}
			fmt.Fprintf(w, "  %s %s\n", mark, c.Describe())
		}
		for _, a := range d.AboveCeiling {
			fmt.Fprintf(w, "  ! above the effect ceiling: %s; calls of it are XO1309\n", a)
		}
	}
}

// runInit is `xo init [--dir=<dir>] <module path>`: write xo.mod, and add
// xo.work to .gitignore unless a workspace already covers the directory
// (a single module repository keeps its workspace private; decision
// 0086).
func runInit(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("init", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "the module directory")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, "xo init: want one module path (xo init github.com/acme/billing)")
		return 2
	}
	path := fl.Arg(0)
	if !mod.ValidPath(path) {
		fmt.Fprintf(stderr, "xo init: invalid module path %q: a module path is its location, such as github.com/acme/billing (core 4.2)\n", path)
		return 2
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "xo init: %v\n", err)
		return 1
	}
	name := filepath.Join(*dir, "xo.mod")
	if _, err := os.Stat(name); err == nil {
		fmt.Fprintf(stderr, "xo init: %s already exists\n", name)
		return 1
	}
	if err := os.WriteFile(name, []byte(fmt.Sprintf("module %s\nedition %d\n", path, check.CurrentEdition)), 0o644); err != nil {
		fmt.Fprintf(stderr, "xo init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", name)
	if w := mod.NearestWork(*dir); w != "" {
		fmt.Fprintf(stdout, "the workspace %s is above it; add `use %s` to it to build the module from here\n", w, *dir)
		return 0
	}
	ignore := filepath.Join(*dir, ".gitignore")
	data, err := os.ReadFile(ignore)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "xo init: %v\n", err)
		return 1
	}
	for _, l := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(l); t == "xo.work" || t == "/xo.work" {
			return 0
		}
	}
	s := string(data)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	s += "# a local xo.work workspace (decision 0086) is not committed in a single module repository\n/xo.work\n"
	if err := os.WriteFile(ignore, []byte(s), 0o644); err != nil {
		fmt.Fprintf(stderr, "xo init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "added xo.work to %s\n", ignore)
	return 0
}
