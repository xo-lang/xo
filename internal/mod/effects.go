package mod

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/parser"
)

// ModuleEffects is the effect set a module version's exported API can
// reach (decision 0156): the union of the effect labels in the `uses`
// clauses of its `pub` functions and methods, in every package of the
// module tree (nested modules, vendor/, testdata/, dot directories, and
// _test.xo files left out), sorted. The checker makes every effect a
// function reaches appear in a pub caller's `uses`, so the declared
// surface is the reachable set; it is computed from syntax, so no type
// check is needed. Effect variables are the caller's own and do not
// count.
func ModuleEffects(dir string) ([]string, error) {
	known := map[string]bool{}
	for _, l := range EffectLabels {
		known[l] = true
	}
	set := map[string]bool{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir {
				n := d.Name()
				if n == "vendor" || n == "testdata" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(p, "xo.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, _ := parser.ParseFile(p, src, 0)
		if f == nil {
			return nil
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FnDecl)
			if !ok || !fn.Pub {
				continue
			}
			for _, cl := range fn.Clauses {
				u, ok := cl.(*ast.UsesClause)
				if !ok {
					continue
				}
				for _, e := range u.Effects {
					if e.Name == nil {
						continue
					}
					n := e.Name.Name
					if e.Sub != nil {
						n += "." + e.Sub.Name
					}
					if known[n] {
						set[n] = true
					}
				}
			}
		}
		return nil
	})
	out := []string{}
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return normalizeEffects(out), err
}

// normalizeEffects drops a sub effect whose parent is in the set
// (`net` covers `net.dial`).
func normalizeEffects(ls []string) []string {
	have := map[string]bool{}
	for _, l := range ls {
		have[l] = true
	}
	out := []string{}
	for _, l := range ls {
		if i := strings.IndexByte(l, '.'); i > 0 && have[l[:i]] {
			continue
		}
		out = append(out, l)
	}
	return out
}

// AddedEffects are the labels of cur that locked does not admit
// (`fs` admits `fs.read`).
func AddedEffects(locked, cur []string) []string {
	c := &Ceiling{Effects: locked}
	var out []string
	for _, l := range cur {
		if !c.Admits(l) {
			out = append(out, l)
		}
	}
	return out
}

// EffectsText is the xo.lock spelling of an effect set: labels joined
// with commas, or `none`.
func EffectsText(ls []string) string {
	if len(ls) == 0 {
		return "none"
	}
	return strings.Join(ls, ",")
}

// parseEffectsText reads EffectsText; ok is false for an unknown label.
func parseEffectsText(s string) ([]string, bool) {
	if s == "none" {
		return []string{}, true
	}
	known := map[string]bool{}
	for _, l := range EffectLabels {
		known[l] = true
	}
	out := []string{}
	for _, l := range strings.Split(s, ",") {
		if !known[l] {
			return nil, false
		}
		out = append(out, l)
	}
	sort.Strings(out)
	return out, true
}

// checkEffects computes the effect set of every selected module version
// (not workspace modules) and compares it with the `effects` lines of
// the roots' locks (the union in a workspace): an effect a line does
// not admit is XO1317 on that line, unless opts accepts the module.
func (b *BuildList) checkEffects(locks map[string]*Lock, roots []*File, opts Options) {
	for _, m := range b.Modules {
		if m.Workspace {
			continue
		}
		cur, err := ModuleEffects(m.Dir)
		if err != nil {
			continue
		}
		b.Effects[m.Path] = cur
		for _, r := range roots {
			l := locks[r.Dir]
			if l == nil {
				continue
			}
			locked, ok := l.Effects[m.Path]
			if !ok {
				continue
			}
			if _, seen := b.Locked[m.Path]; !seen {
				b.Locked[m.Path] = locked
			}
			added := AddedEffects(locked, cur)
			if len(added) == 0 || opts.AcceptAll || opts.AcceptEffects[m.Path] {
				continue
			}
			b.problem(&Problem{Code: diag.EffectsWidened, File: l.File, Line: l.EffectLines[m.Path],
				Msg: fmt.Sprintf("%s %s reaches effects that xo.lock does not record for it: %s (locked: %s, now: %s)", m.Path, m.Version, "`"+strings.Join(added, "`, `")+"`", EffectsText(locked), EffectsText(cur)),
				Notes: []string{
					"the API of the selected version declares effects the version locked before did not; review why it needs them (decision 0156)",
					"to accept them: `xo mod accept-effects " + m.Path + "` (or `xo get --accept-effects " + m.Path + "@" + m.Version + "`); otherwise stay on the previous version",
				}})
		}
	}
}
