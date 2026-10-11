package mod

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
)

// Module is one module of a build list besides the main module.
type Module struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"` // "" for a workspace module
	Dir     string `json:"dir"`
	// Workspace is set for a module built from its local directory
	// because the active xo.work uses it.
	Workspace bool `json:"workspace,omitempty"`
	// Vendored is set when Dir is under the main module's vendor/.
	Vendored bool `json:"vendored,omitempty"`
	// Direct is set when the main module requires it itself.
	Direct bool `json:"direct,omitempty"`
}

// Options configure Load.
type Options struct {
	// NoWorkspace ignores xo.work (xo get, xo mod tidy, xo vendor: the
	// lock is never written with workspace overrides).
	NoWorkspace bool
	// NoVendor ignores vendor/ (xo mod tidy, xo vendor).
	NoVendor bool
	// Update records the hashes of module versions xo.lock does not list
	// yet instead of reporting XO1305 (xo get, xo mod tidy). Mismatches
	// are still XO1304.
	Update bool
	// Extra are requirements added to the main module's (xo mod tidy
	// for imports xo.mod does not require yet).
	Extra []Require
	// AcceptEffects accepts the current effects of these module paths
	// (all modules when AcceptAll) instead of reporting XO1317 when they
	// are wider than xo.lock (xo mod accept-effects, xo get
	// --accept-effects; decision 0156).
	AcceptEffects map[string]bool
	AcceptAll     bool
}

// BuildList is the result of Load: the main module, the workspace, the
// selected module versions, and the problems found.
type BuildList struct {
	Main *File
	Work *Work // the active workspace, or nil
	// Modules are the selected modules other than the main one, sorted
	// by path: required versions (MVS), vendored copies, and the other
	// modules of the workspace.
	Modules []Module
	// Hashes are the xo.lock entries of this selection, computed from the
	// trees (written by xo get and xo mod tidy).
	Hashes map[string]string
	// Effects are the effect sets of the selected module versions (not
	// workspace modules) by module path (ModuleEffects), and Locked the
	// sets xo.lock records for them, where it has a line (decision
	// 0156).
	Effects map[string][]string
	Locked  map[string][]string
	// Vendor is set when the build uses the main module's vendor/.
	Vendor   bool
	Problems []*Problem
}

// Overrides lists the modules a workspace builds from local directories
// in place of a required version (release builds name them, XO1308).
func (b *BuildList) Overrides() []string {
	if b.Work == nil {
		return nil
	}
	required := map[string]bool{}
	for _, m := range b.Work.Modules {
		for _, r := range m.Requires {
			required[r.Path] = true
		}
	}
	var out []string
	for _, m := range b.Modules {
		if m.Workspace && required[m.Path] {
			out = append(out, m.Path)
		}
	}
	return out
}

func (b *BuildList) problem(p *Problem) {
	for _, q := range b.Problems {
		if q.Code == p.Code && q.Msg == p.Msg && q.File == p.File && q.Line == p.Line {
			return
		}
	}
	b.Problems = append(b.Problems, p)
}

// Load resolves the dependencies of the main module in mainDir (the
// directory of its xo.mod): with minimal version selection over the
// requirement graph (the highest version any reached xo.mod requires of
// each module), fetching versions missing from the module cache from git
// (unless offline), or from vendor/ when vendor/modules.txt exists, or
// with the workspace's local modules overriding their requirements. Every
// fetched or vendored tree is verified against xo.lock.
func Load(mainDir string, opts Options) *BuildList {
	abs, err := filepath.Abs(mainDir)
	if err == nil {
		mainDir = abs
	}
	b := &BuildList{Hashes: map[string]string{}, Effects: map[string][]string{}, Locked: map[string][]string{}}
	main, probs := ReadFile(mainDir)
	for _, p := range probs {
		b.problem(p)
	}
	if main == nil {
		return b
	}
	main.Requires = append(main.Requires, opts.Extra...)
	b.Main = main
	if !opts.NoWorkspace {
		w, probs := FindWork(mainDir)
		for _, p := range probs {
			b.problem(p)
		}
		b.Work = w
	}
	if b.Work == nil && !opts.NoVendor {
		if _, err := os.Stat(filepath.Join(mainDir, "vendor", "modules.txt")); err == nil {
			b.loadVendor(opts)
			return b
		}
	}

	// the roots of the requirement graph: the main module, or every
	// module of the workspace
	roots := []*File{main}
	local := map[string]*File{main.Path: main}
	if b.Work != nil {
		roots = nil
		for _, m := range b.Work.Modules {
			if m.Dir == main.Dir {
				m = main
			}
			roots = append(roots, m)
			local[m.Path] = m
		}
	}
	locks := map[string]*Lock{}
	for _, r := range roots {
		l, probs := ReadLock(r.Dir)
		for _, p := range probs {
			b.problem(p)
		}
		locks[r.Dir] = l
	}

	// minimal version selection: walk every reachable module version and
	// keep the highest version of each path
	type item struct {
		req  Require
		file string // the xo.mod that requires it
	}
	var queue []item
	direct := map[string]bool{}
	for _, r := range roots {
		for _, q := range r.Requires {
			queue = append(queue, item{q, r.File})
			if r == main {
				direct[q.Path] = true
			}
		}
	}
	selected := map[string]string{}
	dirs := map[string]string{} // "path version" -> cache dir
	visited := map[string]bool{}
	failed := map[string]bool{}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		p, v := it.req.Path, it.req.Version
		if local[p] != nil {
			continue // the main module or a workspace module
		}
		key := p + " " + v
		if visited[key] {
			continue
		}
		visited[key] = true
		if cur, ok := selected[p]; !ok || Compare(v, cur) > 0 {
			selected[p] = v
		}
		dir, prob := Download(p, v)
		if prob != nil {
			prob.File, prob.Line = it.file, it.req.Line
			b.problem(prob)
			failed[p] = true
			continue
		}
		dirs[key] = dir
		mf, probs := ReadFile(dir)
		for _, q := range probs {
			b.problem(q)
		}
		if mf == nil {
			continue
		}
		h, err := HashModFile(dir)
		if err == nil {
			b.Hashes[key+"/xo.mod"] = h
		}
		for _, q := range mf.Requires {
			queue = append(queue, item{q, mf.File})
		}
	}
	for p, v := range selected {
		if failed[p] {
			continue
		}
		key := p + " " + v
		dir := dirs[key]
		h, err := HashDir(dir)
		if err != nil {
			b.problem(&Problem{Code: diag.ModuleFetch, Module: p, File: main.File, Msg: fmt.Sprintf("cannot hash %s: %v", dir, err)})
			continue
		}
		b.Hashes[key] = h
		b.Modules = append(b.Modules, Module{Path: p, Version: v, Dir: dir, Direct: direct[p]})
	}
	if b.Work != nil {
		for _, m := range b.Work.Modules {
			if m.Dir != main.Dir {
				b.Modules = append(b.Modules, Module{Path: m.Path, Dir: m.Dir, Workspace: true, Direct: direct[m.Path]})
			}
		}
	}
	sort.Slice(b.Modules, func(i, j int) bool { return b.Modules[i].Path < b.Modules[j].Path })

	// verify every hash against xo.lock (the union of the workspace
	// modules' locks in a workspace)
	var keys []string
	for k := range b.Hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		got := b.Hashes[k]
		path, _, _ := strings.Cut(k, " ")
		found := false
		for _, r := range roots {
			l := locks[r.Dir]
			want, ok := l.Entries[k]
			if !ok {
				continue
			}
			found = true
			if want != got {
				b.problem(&Problem{Code: diag.ChecksumMismatch, Module: path, File: l.File, Line: l.Lines[k],
					Msg:   fmt.Sprintf("%s: content hash %s does not match xo.lock (%s)", k, got, want),
					Notes: []string{"the module version changed since it was locked (a moved tag or an edited cache or vendor copy); nothing was built", "if the change is expected, delete the line and run `xo mod tidy`"}})
				failed[path] = true
			}
		}
		if !found && !opts.Update {
			lf := locks[main.Dir].File
			b.problem(&Problem{Code: diag.LockMissing, Module: path, File: main.File, Line: requireLine(main, path),
				Msg:   fmt.Sprintf("xo.lock has no hash for %s", k),
				Notes: []string{"run `xo mod tidy` (or `xo get`) to record it in " + lf}})
		}
	}
	b.checkEffects(locks, roots, opts)
	if len(failed) > 0 {
		var keep []Module
		for _, m := range b.Modules {
			if !failed[m.Path] {
				keep = append(keep, m)
			}
		}
		b.Modules = keep
	}
	return b
}

func requireLine(f *File, path string) int {
	for _, r := range f.Requires {
		if r.Path == path {
			return r.Line
		}
	}
	return 0
}

// VendorModules parses vendor/modules.txt: "# <path> <version>" lines.
func VendorModules(mainDir string) ([]Module, error) {
	name := filepath.Join(mainDir, "vendor", "modules.txt")
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var out []Module
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 3 && f[0] == "#" && ValidPath(f[1]) && ValidVersion(f[2]) {
			out = append(out, Module{Path: f[1], Version: f[2], Dir: filepath.Join(mainDir, "vendor", filepath.FromSlash(f[1])), Vendored: true})
		}
	}
	return out, nil
}

// loadVendor builds from the main module's vendor/: no cache and no
// network. Every direct requirement must be vendored at its version, and
// every vendored tree must match xo.lock.
func (b *BuildList) loadVendor(opts Options) {
	main := b.Main
	b.Vendor = true
	mods, err := VendorModules(main.Dir)
	if err != nil {
		b.problem(&Problem{Code: diag.VendorInconsistent, File: main.File, Msg: fmt.Sprintf("cannot read vendor/modules.txt: %v", err)})
		return
	}
	lock, probs := ReadLock(main.Dir)
	for _, p := range probs {
		b.problem(p)
	}
	byPath := map[string]Module{}
	for _, m := range mods {
		byPath[m.Path] = m
	}
	bad := map[string]bool{}
	for _, r := range main.Requires {
		m, ok := byPath[r.Path]
		if !ok || Compare(m.Version, r.Version) < 0 {
			have := "is not vendored"
			if ok {
				have = "is vendored at " + m.Version
			}
			b.problem(&Problem{Code: diag.VendorInconsistent, Module: r.Path, File: main.File, Line: r.Line,
				Msg:   fmt.Sprintf("%s %s is required but %s", r.Path, r.Version, have),
				Notes: []string{"run `xo vendor` to bring vendor/ in line with xo.mod"}})
			bad[r.Path] = true
		}
	}
	for _, m := range mods {
		if bad[m.Path] {
			continue
		}
		m.Direct = requireLine(main, m.Path) > 0
		key := m.Path + " " + m.Version
		h, err := HashDir(m.Dir)
		if err != nil {
			b.problem(&Problem{Code: diag.VendorInconsistent, Module: m.Path, File: main.File, Msg: fmt.Sprintf("vendored %s: %v", key, err), Notes: []string{"run `xo vendor`"}})
			continue
		}
		b.Hashes[key] = h
		want, ok := lock.Entries[key]
		switch {
		case !ok:
			b.problem(&Problem{Code: diag.LockMissing, Module: m.Path, File: main.File, Line: requireLine(main, m.Path), Msg: fmt.Sprintf("xo.lock has no hash for vendored %s", key), Notes: []string{"run `xo mod tidy`, then `xo vendor`"}})
			continue
		case want != h:
			b.problem(&Problem{Code: diag.ChecksumMismatch, Module: m.Path, File: lock.File, Line: lock.Lines[key],
				Msg:   fmt.Sprintf("vendored %s: content hash %s does not match xo.lock (%s)", key, h, want),
				Notes: []string{"vendor/ was edited; run `xo vendor` to restore it"}})
			continue
		}
		b.Modules = append(b.Modules, m)
	}
	b.checkEffects(map[string]*Lock{main.Dir: lock}, []*File{main}, opts)
}
