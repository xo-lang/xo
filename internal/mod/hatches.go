package mod

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gobind"
	"github.com/xo-lang/xo/internal/parser"
)

// Hatch is an escape hatch in a module's source: a `use c` line, or a
// `use go` of a Go package that is not pure (decision 0156).
type Hatch struct {
	Kind   string // "c" or "go"
	Target string // the header or Go import path
	// Claim is a `use c` line's own `uses` (nil: none given).
	Claim []string
	File  string
	Line  int
}

// ModuleHatches lists the escape hatches in the source files of the
// module at dir, over the files ModuleEffects reads. It is computed from
// syntax, so `xo get` sees a hatch an update adds before anything is
// written; the checker reports the same hatches as XO1316 for the
// packages a build imports.
func ModuleHatches(dir string) ([]Hatch, error) {
	var out []Hatch
	err := walkModuleSources(dir, func(p string, src []byte) {
		f, _ := parser.ParseFile(p, src, 0)
		if f == nil {
			return
		}
		for _, u := range f.Uses {
			h := Hatch{Target: u.Path, File: p, Line: u.Sp.Start.Line}
			switch {
			case u.C:
				h.Kind = "c"
				if u.Uses != nil {
					h.Claim = []string{}
					for _, e := range u.Uses {
						if e.Name == nil {
							continue
						}
						n := e.Name.Name
						if e.Sub != nil {
							n += "." + e.Sub.Name
						}
						h.Claim = append(h.Claim, n)
					}
				}
			case u.Go && !gobind.PurePackage(u.Path):
				h.Kind = "go"
			default:
				continue
			}
			out = append(out, h)
		}
	})
	return out, err
}

// walkModuleSources calls fn with each source file of the module at dir
// that ModuleEffects reads: not nested modules, vendor/, testdata/, dot
// directories, or _test.xo files.
func walkModuleSources(dir string, fn func(path string, src []byte)) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
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
		fn(p, src)
		return nil
	})
}

// checkHatches reports XO1316 for every escape hatch of a selected
// module version (not workspace modules) that the main module's xo.mod
// does not vouch for at that version. xo get, xo mod tidy, and xo mod
// accept-effects run it before writing, so an update that adds an
// unvouched `use c` or impure `use go` is refused like one that widens
// effects (XO1317); --accept-effects accepts effects, not hatches.
func (b *BuildList) checkHatches(main *File) {
	for _, m := range b.Modules {
		if m.Workspace {
			continue
		}
		hs, err := ModuleHatches(m.Dir)
		if err != nil {
			continue
		}
		for _, h := range hs {
			v, stale := FindVouch(main.Vouches, m.Path, m.Version, h.Kind, h.Target)
			if v != nil {
				continue
			}
			what := "Go code reaches files, the network, and processes without a capability"
			if h.Kind == "c" {
				what = "its C code counts only with a vouch, so the binding performs `ffi`"
			}
			notes := []string{}
			for _, s := range stale {
				notes = append(notes, fmt.Sprintf("%s:%d vouches for %s %s; the update selects %s, whose code is not reviewed", s.File, s.Line, s.Path, s.Version, m.Version))
			}
			notes = append(notes, "after reviewing the code, add to "+main.File+": "+SuggestVouch(m.Path, m.Version, h.Kind, h.Target, h.Claim))
			b.problem(&Problem{Code: diag.UnvouchedEscape, File: h.File, Line: h.Line, Module: m.Path,
				Msg:   fmt.Sprintf("`use %s \"%s\"` of %s %s is not vouched for by the consuming module: %s", h.Kind, h.Target, m.Path, m.Version, what),
				Notes: notes})
		}
	}
}
