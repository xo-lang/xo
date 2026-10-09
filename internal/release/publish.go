package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
)

// Options configure Publish.
type Options struct {
	// Version is the version to release; "" picks the next minor version
	// (the next patch before v1.0.0) when nothing breaks, and that same
	// non major version when something does, so breaking changes fail
	// unless a major version is asked for.
	Version string
	// Check compares only: uncommitted changes are allowed and the
	// working tree is what is compared.
	Check bool
	// Verify, when set, checks the module at root with workspaces off
	// and returns its diagnostics (not with Check).
	Verify func(root string) []diag.Diagnostic
}

// Result is the outcome of `xo publish`.
type Result struct {
	Ok        bool              `json:"ok"`
	Module    string            `json:"module"`
	Previous  string            `json:"previous,omitempty"` // "" for a first release
	Version   string            `json:"version"`
	Bump      string            `json:"bump"`      // what the changes need: major, minor, patch
	Suggested string            `json:"suggested"` // the smallest version the changes allow
	Changes   []Change          `json:"changes"`
	Breaking  []Change          `json:"breaking"`
	Tag       string            `json:"tag,omitempty"` // the command that publishes, when ok
	Diags     []diag.Diagnostic `json:"diagnostics"`
}

func (r *Result) fail(code diag.Code, file string, line int, msg string, notes ...string) {
	if line < 1 {
		line = 1
	}
	d := diag.Diagnostic{Code: string(code), Severity: diag.Error, Message: msg, Span: diag.Span{File: file, Line: line, Col: 1}}
	for _, n := range notes {
		d.Notes = append(d.Notes, diag.NoteItem{Message: n})
	}
	r.Diags = append(r.Diags, d)
}

// next is v with the given part bumped.
func next(v, part string) string {
	if v == "" {
		return "v0.1.0"
	}
	core, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	parts := strings.Split(core, ".")
	n := make([]int, 3)
	for i := range n {
		n[i], _ = strconv.Atoi(parts[i])
	}
	pre := strings.Contains(v, "-")
	switch part {
	case "major":
		n[0], n[1], n[2] = n[0]+1, 0, 0
	case "minor":
		n[1], n[2] = n[1]+1, 0
	default:
		if !pre {
			n[2]++
		}
	}
	return fmt.Sprintf("v%d.%d.%d", n[0], n[1], n[2])
}

func minor(v string) int {
	core, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
}

// allowsBreaking reports whether going from prev to v may break: a new
// major version, or a new minor version before v1.0.0.
func allowsBreaking(prev, v string) bool {
	pm, vm := mod.Major(prev), mod.Major(v)
	if vm > pm {
		return true
	}
	return vm == 0 && pm == 0 && minor(v) > minor(prev)
}

// Publish checks a release of the module in dir: it compares the exports
// and effects with the previous vX.Y.Z tag of the git repository and
// rejects a minor or patch release that breaks them (XO1310). Without
// Check it also requires a clean working tree. It never tags or pushes:
// Result.Tag is the command that publishes.
func Publish(dir string, opts Options) *Result {
	r := &Result{Changes: []Change{}, Breaking: []Change{}, Diags: []diag.Diagnostic{}}
	root := mod.ModRoot(dir)
	if root == "" {
		r.fail(diag.InvalidRelease, dir, 0, "no xo.mod in "+dir+" or above")
		return r
	}
	modFile := filepath.Join(root, "xo.mod")
	f, probs := mod.ReadFile(root)
	for _, p := range probs {
		r.fail(p.Code, p.File, p.Line, p.Msg, p.Notes...)
	}
	if f == nil || f.Path == "" {
		return r
	}
	r.Module = f.Path
	top, err := mod.Git(root, "rev-parse", "--show-toplevel")
	if err != nil || !sameDir(strings.TrimSpace(top), root) {
		r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("module %s is not the root of a git repository: a version is a tag of the repository holding the module at its root", f.Path))
		return r
	}
	tags, err := mod.LocalTags(root)
	if err != nil {
		r.fail(diag.InvalidRelease, modFile, 0, "cannot list the git tags: "+err.Error())
		return r
	}
	if opts.Version != "" && !mod.ValidVersion(opts.Version) {
		r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("invalid version %q: want vX.Y.Z", opts.Version))
		return r
	}
	// the previous release: the highest tag below the version (the
	// highest tag when no version is given)
	for _, t := range tags {
		if opts.Version == "" || mod.Compare(t, opts.Version) < 0 {
			r.Previous = t
		}
	}
	if r.Previous != "" {
		tmp, err := os.MkdirTemp("", "xo-publish-")
		if err != nil {
			r.fail(diag.InvalidRelease, modFile, 0, err.Error())
			return r
		}
		defer os.RemoveAll(tmp)
		if err := mod.ExtractTag(root, r.Previous, tmp); err != nil {
			r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("cannot read %s: %v", r.Previous, err))
			return r
		}
		oldPath := f.Path
		if of, _ := mod.ReadFile(tmp); of != nil && of.Path != "" {
			oldPath = of.Path
		}
		oldAPI, err := ModuleAPI(tmp, oldPath)
		if err != nil {
			r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("cannot read the API of %s: %v", r.Previous, err))
			return r
		}
		newAPI, err := ModuleAPI(root, f.Path)
		if err != nil {
			r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("cannot read the API: %v", err))
			return r
		}
		if oldPath != f.Path {
			// a new major version path: compare the packages by their
			// path inside the module
			for i := range oldAPI {
				oldAPI[i].Package = f.Path + strings.TrimPrefix(oldAPI[i].Package, oldPath)
			}
		}
		r.Changes = Diff(oldAPI, newAPI)
		if r.Changes == nil {
			r.Changes = []Change{}
		}
	}
	r.Bump = Bump(r.Changes)
	base := "minor"
	if r.Previous == "" || mod.Major(r.Previous) == 0 {
		base = "patch"
	}
	switch {
	case r.Previous == "":
		_, n := mod.MajorSuffix(f.Path)
		r.Suggested = "v0.1.0"
		if n >= 2 {
			r.Suggested = fmt.Sprintf("v%d.0.0", n)
		}
	case r.Bump == "major" && mod.Major(r.Previous) == 0:
		r.Suggested = next(r.Previous, "minor")
	case r.Bump == "major":
		r.Suggested = next(r.Previous, "major")
	case r.Bump == "minor":
		r.Suggested = next(r.Previous, "minor")
	default:
		r.Suggested = next(r.Previous, "patch")
	}
	r.Version = opts.Version
	if r.Version == "" {
		r.Version = r.Suggested
		if r.Bump == "major" {
			// assume a compatible release: breaking changes fail
			r.Version = next(r.Previous, base)
		}
	}
	// the version itself
	for _, t := range tags {
		if t == r.Version {
			r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("%s is already a tag of %s", r.Version, f.Path), "a published version never changes; release "+r.Suggested+" instead")
		}
	}
	if r.Previous != "" && mod.Compare(r.Version, r.Previous) <= 0 {
		r.fail(diag.InvalidRelease, modFile, 0, fmt.Sprintf("%s is not above the last release %s", r.Version, r.Previous))
	}
	if msg := mod.MajorMismatch(f.Path, r.Version); msg != "" {
		base, _ := mod.MajorSuffix(f.Path)
		r.fail(diag.InvalidRelease, modFile, 0, msg, fmt.Sprintf("change the module line of xo.mod to `module %s/v%d` (and the imports of its own packages) for a major version", base, mod.Major(r.Version)))
	}
	// breaking changes
	breakOK := r.Previous == "" || allowsBreaking(r.Previous, r.Version)
	for _, c := range r.Changes {
		if !c.Breaking {
			continue
		}
		r.Breaking = append(r.Breaking, c)
		if breakOK {
			continue
		}
		file, line := c.File, c.Line
		if file == "" {
			file, line = modFile, 0
		}
		var notes []string
		if c.Old != "" {
			notes = append(notes, r.Previous+": "+firstLine(c.Old))
		}
		if c.New != "" {
			notes = append(notes, "now: "+firstLine(c.New))
		}
		notes = append(notes, fmt.Sprintf("%s breaks callers of %s; release %s (a new major version) or keep the export compatible", c.Describe(), r.Previous, r.Suggested))
		r.fail(diag.BreakingRelease, relTo(file), line, fmt.Sprintf("%s in %s, a %s release after %s", c.Describe(), r.Version, kindOf(r.Previous, r.Version), r.Previous), notes...)
	}
	if !opts.Check {
		out, err := mod.Git(root, "status", "--porcelain")
		if err != nil {
			r.fail(diag.InvalidRelease, modFile, 0, "git status: "+err.Error())
		} else if s := strings.TrimSpace(out); s != "" {
			r.fail(diag.InvalidRelease, modFile, 0, "the working tree has uncommitted changes; a tag publishes the last commit", "commit or stash them first (`git status`), or use --check to compare the working tree")
		}
		// the version must build for its users: its own requirements,
		// without any workspace, and a clean check
		b := mod.Load(root, mod.Options{NoWorkspace: true})
		for _, p := range b.Problems {
			if diag.SeverityOf(p.Code) == diag.Error {
				r.fail(p.Code, p.File, p.Line, p.Msg, p.Notes...)
			}
		}
		if opts.Verify != nil {
			for _, d := range opts.Verify(root) {
				if d.Severity == diag.Error {
					r.Diags = append(r.Diags, d)
				}
			}
		}
	}
	r.Ok = len(r.Diags) == 0
	if r.Ok {
		r.Tag = "git tag " + r.Version + " && git push origin " + r.Version
	}
	return r
}

func kindOf(prev, v string) string {
	switch {
	case mod.Major(v) > mod.Major(prev):
		return "major"
	case minor(v) > minor(prev) || mod.Major(v) > mod.Major(prev):
		return "minor"
	}
	return "patch"
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// relTo makes file relative to the working directory when it lies below
// it (diagnostic paths).
func relTo(file string) string {
	wd, err := os.Getwd()
	if err != nil {
		return file
	}
	if rel, err := filepath.Rel(wd, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}
