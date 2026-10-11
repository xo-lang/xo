package mod

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/parser"
)

// Change is one requirement changed by xo get or xo mod tidy.
type Change struct {
	Path string `json:"path"`
	Old  string `json:"old,omitempty"` // "" when added
	New  string `json:"new,omitempty"` // "" when removed
}

// Imports lists the import paths of the main module in mainDir that name
// other modules (remote paths outside the main module), sorted. Nested
// modules (directories with their own xo.mod), vendor/, testdata/, and
// dot directories are skipped.
func Imports(mainDir, mainPath string) ([]string, error) {
	set := map[string]bool{}
	err := filepath.WalkDir(mainDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != mainDir {
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
		if !strings.HasSuffix(p, ".xo") {
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
		for _, u := range f.Uses {
			if u.Go || u.C || u.Path == "" || !IsRemote(u.Path) || u.Path == mainPath || strings.HasPrefix(u.Path, mainPath+"/") {
				continue
			}
			set[u.Path] = true
		}
		return nil
	})
	var out []string
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, err
}

// Owner returns the module of mods that holds the package path (the
// longest module path that is path or a prefix of it).
func Owner(mods []Module, path string) (Module, bool) {
	var best Module
	ok := false
	for _, m := range mods {
		if (path == m.Path || strings.HasPrefix(path, m.Path+"/")) && len(m.Path) > len(best.Path) {
			best, ok = m, true
		}
	}
	return best, ok
}

// findModule finds the module that holds a package path: the longest
// prefix of it (with at least two elements) whose repository has vX.Y.Z
// tags. It returns the module path and its latest version.
func findModule(pkg string) (string, string, error) {
	elems := strings.Split(pkg, "/")
	var firstErr error
	for n := len(elems); n >= 2; n-- {
		p := strings.Join(elems[:n], "/")
		v, err := Latest(p)
		if err == nil {
			return p, v, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no module found")
	}
	return "", "", firstErr
}

// errs keeps the error problems.
func errs(ps []*Problem) []*Problem {
	var out []*Problem
	for _, p := range ps {
		if diag.SeverityOf(p.Code) == diag.Error {
			out = append(out, p)
		}
	}
	return out
}

// Get is `xo get <path>[@version]` in the main module in mainDir: it
// requires the module at the version (latest tag when none is given;
// `@none` removes the requirement), resolves the graph, and rewrites
// xo.mod and xo.lock. On problems nothing is written.
func Get(mainDir, arg string) ([]Change, []*Problem) {
	return GetAccept(mainDir, arg, false)
}

// GetAccept is Get; with accept (`xo get --accept-effects`) the module's
// effects are recorded in xo.lock even when they are wider than the
// line there (decision 0156).
func GetAccept(mainDir, arg string, accept bool) ([]Change, []*Problem) {
	path, version, _ := strings.Cut(arg, "@")
	main, probs := ReadFile(mainDir)
	if len(probs) > 0 {
		return nil, probs
	}
	if !ValidPath(path) {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: fmt.Sprintf("invalid module path %q", path), Notes: []string{"a module path is its location, such as github.com/acme/billing (core 4.2)"}}}
	}
	if version != "" && version != "latest" && version != "none" && !ValidVersion(version) {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: fmt.Sprintf("invalid version %q: want vX.Y.Z, latest, or none", version)}}
	}
	if version == "none" {
		if requireLine(main, path) == 0 {
			return nil, nil
		}
	} else if version == "" || version == "latest" {
		// the path may name a package inside a module
		mp, v, err := findModule(path)
		if err != nil {
			return nil, []*Problem{{Code: offlineOr(diag.ModuleFetch), Module: path, File: main.File, Msg: fmt.Sprintf("cannot find a version of %s: %v", path, err), Notes: []string{"git location: " + RepoURL(path)}}}
		}
		path, version = mp, v
	}
	old := ""
	for _, r := range main.Requires {
		if r.Path == path {
			old = r.Version
		}
	}
	data, err := os.ReadFile(main.File)
	if err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: err.Error()}}
	}
	nv := version
	if version == "none" {
		nv = ""
	}
	if old == nv {
		// still refresh the lock
		return nil, writeLockFor(mainDir, acceptOpts(path, accept))
	}
	updated := SetRequire(data, path, nv)
	if err := os.WriteFile(main.File, updated, 0o644); err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: err.Error()}}
	}
	if ps := writeLockFor(mainDir, acceptOpts(path, accept)); len(errs(ps)) > 0 {
		os.WriteFile(main.File, data, 0o644)
		return nil, ps
	}
	return []Change{{Path: path, Old: old, New: nv}}, nil
}

// acceptOpts accepts the effects of module path when accept is set.
func acceptOpts(path string, accept bool) Options {
	if !accept {
		return Options{CheckHatches: true}
	}
	return Options{AcceptEffects: map[string]bool{path: true}, CheckHatches: true}
}

func offlineOr(c diag.Code) diag.Code {
	if Offline() {
		return diag.ModuleOffline
	}
	return c
}

// writeLockFor resolves the main module without workspace or vendor and
// writes its xo.lock: the hashes, and the effects of every selected
// module (decision 0156): recorded as they are the first time, narrowed
// when they shrank, and XO1317 (nothing written) when they grew, unless
// accept accepts them.
func writeLockFor(mainDir string, accept Options) []*Problem {
	_, ps := writeLockChanges(mainDir, accept)
	return ps
}

// EffectChange is an effect line of xo.lock written by xo get, xo mod
// tidy, or xo mod accept-effects.
type EffectChange struct {
	Path string   `json:"path"`
	Old  []string `json:"old"` // nil when the module had no line
	New  []string `json:"new"`
}

func writeLockChanges(mainDir string, accept Options) ([]EffectChange, []*Problem) {
	opts := Options{NoWorkspace: true, NoVendor: true, Update: true, AcceptEffects: accept.AcceptEffects, AcceptAll: accept.AcceptAll, CheckHatches: accept.CheckHatches}
	b := Load(mainDir, opts)
	if opts.CheckHatches && len(errs(b.Problems)) == 0 && b.Main != nil {
		b.checkHatches(b.Main)
	}
	if len(errs(b.Problems)) > 0 {
		return nil, b.Problems
	}
	var changes []EffectChange
	for _, m := range b.Modules {
		cur, ok := b.Effects[m.Path]
		if !ok {
			continue
		}
		old, had := b.Locked[m.Path]
		if had && EffectsText(old) == EffectsText(cur) {
			continue
		}
		c := EffectChange{Path: m.Path, New: cur}
		if had {
			c.Old = old
		}
		changes = append(changes, c)
	}
	if err := WriteLock(mainDir, b.Hashes, b.Effects); err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: filepath.Join(mainDir, "xo.lock"), Msg: err.Error()}}
	}
	return changes, nil
}

// AcceptEffects is `xo mod accept-effects [<module path>...]`: it records
// the current effects of the named modules (every module when none is
// named) in xo.lock, accepting effects wider than the lines there
// (decision 0156). It returns the lines that changed.
func AcceptEffects(mainDir string, paths []string) ([]EffectChange, []*Problem) {
	opts := Options{AcceptAll: len(paths) == 0, AcceptEffects: map[string]bool{}, CheckHatches: true}
	for _, p := range paths {
		opts.AcceptEffects[p] = true
	}
	return writeLockChanges(mainDir, opts)
}

// Tidy is `xo mod tidy`: it adds a requirement (the latest version) for
// every imported module xo.mod does not require, removes requirements
// nothing imports unless they raise a version the rest of the graph
// selects, and rewrites xo.lock with exactly the hashes of the selection.
// Workspaces and vendor/ are ignored: the lock never records overrides.
func Tidy(mainDir string) ([]Change, []*Problem) {
	main, probs := ReadFile(mainDir)
	if len(errs(probs)) > 0 {
		return nil, probs
	}
	imports, err := Imports(mainDir, main.Path)
	if err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: err.Error()}}
	}
	var changes []Change
	var extra []Require
	b := Load(mainDir, Options{NoWorkspace: true, NoVendor: true, Update: true})
	if len(errs(b.Problems)) > 0 {
		return nil, b.Problems
	}
	for _, imp := range imports {
		if _, ok := Owner(b.Modules, imp); ok {
			continue
		}
		done := false
		for _, r := range extra {
			if imp == r.Path || strings.HasPrefix(imp, r.Path+"/") {
				done = true
			}
		}
		if done {
			continue
		}
		mp, v, err := findModule(imp)
		if err != nil {
			return nil, []*Problem{{Code: offlineOr(diag.ModuleFetch), Module: imp, File: main.File, Msg: fmt.Sprintf("no module provides %s: %v", imp, err), Notes: []string{"git location tried: " + RepoURL(imp)}}}
		}
		extra = append(extra, Require{Path: mp, Version: v})
		changes = append(changes, Change{Path: mp, New: v})
	}
	// requirements nothing imports
	used := func(p string) bool {
		for _, imp := range imports {
			if imp == p || strings.HasPrefix(imp, p+"/") {
				return true
			}
		}
		return false
	}
	var unused []Require
	for _, r := range main.Requires {
		if !used(r.Path) {
			unused = append(unused, r)
		}
	}
	drop := map[string]bool{}
	if len(unused) > 0 {
		// resolve without them: drop one unless the rest selects a lower
		// version of it (then it is kept, raising that version)
		data, _ := os.ReadFile(main.File)
		trial := data
		for _, r := range unused {
			trial = SetRequire(trial, r.Path, "")
		}
		os.WriteFile(main.File, trial, 0o644)
		without := Load(mainDir, Options{NoWorkspace: true, NoVendor: true, Update: true, Extra: extra})
		os.WriteFile(main.File, data, 0o644)
		sel := map[string]string{}
		for _, m := range without.Modules {
			sel[m.Path] = m.Version
		}
		for _, r := range unused {
			if v, ok := sel[r.Path]; !ok || Compare(v, r.Version) >= 0 {
				drop[r.Path] = true
				changes = append(changes, Change{Path: r.Path, Old: r.Version})
			}
		}
	}
	data, err := os.ReadFile(main.File)
	if err != nil {
		return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: err.Error()}}
	}
	orig := data
	for p := range drop {
		data = SetRequire(data, p, "")
	}
	for _, r := range extra {
		data = SetRequire(data, r.Path, r.Version)
	}
	if string(data) != string(orig) {
		if err := os.WriteFile(main.File, data, 0o644); err != nil {
			return nil, []*Problem{{Code: diag.ModFileInvalid, File: main.File, Msg: err.Error()}}
		}
	}
	if ps := writeLockFor(mainDir, Options{}); len(errs(ps)) > 0 {
		os.WriteFile(main.File, orig, 0o644)
		return nil, ps
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

// Vendor is `xo vendor`: it copies the tree of every selected module
// version into vendor/<module path>/ and lists them in vendor/modules.txt;
// builds then use vendor/ and never the cache or the network. xo.lock
// must already hold every hash (`xo mod tidy`). With no requirements it
// removes vendor/.
func Vendor(mainDir string) ([]Module, []*Problem) {
	b := Load(mainDir, Options{NoWorkspace: true, NoVendor: true})
	if len(errs(b.Problems)) > 0 {
		return nil, b.Problems
	}
	vdir := filepath.Join(mainDir, "vendor")
	makeWritable(vdir)
	if err := os.RemoveAll(vdir); err != nil {
		return nil, []*Problem{{Code: diag.VendorInconsistent, File: b.Main.File, Msg: err.Error()}}
	}
	if len(b.Modules) == 0 {
		return nil, nil
	}
	var list strings.Builder
	list.WriteString("# vendor/modules.txt: written by `xo vendor`; do not edit.\n")
	for _, m := range b.Modules {
		dst := filepath.Join(vdir, filepath.FromSlash(m.Path))
		if err := copyTree(m.Dir, dst); err != nil {
			return nil, []*Problem{{Code: diag.VendorInconsistent, Module: m.Path, File: b.Main.File, Msg: fmt.Sprintf("vendoring %s %s: %v", m.Path, m.Version, err)}}
		}
		fmt.Fprintf(&list, "# %s %s\n", m.Path, m.Version)
	}
	if err := os.WriteFile(filepath.Join(vdir, "modules.txt"), []byte(list.String()), 0o644); err != nil {
		return nil, []*Problem{{Code: diag.VendorInconsistent, File: b.Main.File, Msg: err.Error()}}
	}
	out := Load(mainDir, Options{NoWorkspace: true})
	if len(errs(out.Problems)) > 0 {
		return nil, out.Problems
	}
	return out.Modules, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
