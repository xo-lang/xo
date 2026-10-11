package check

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/token"
)

// consumerDirs are the xo.mod directories of the consuming modules: the
// main modules checked and the modules of their workspaces. Only their
// `vouch` lines count (decision 0156).
func (c *checker) consumerDirs() []string {
	var dirs []string
	for d := range c.deps.resolved {
		dirs = append(dirs, d)
	}
	for d, m := range c.deps.depDirs {
		if m.Workspace {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// consumerVouches are the vouch lines of the consuming modules, read
// once per check.
func (c *checker) consumerVouches() []mod.Vouch {
	if c.vouches == nil {
		c.vouches = mod.VouchesOf(c.consumerDirs())
		if c.vouches == nil {
			c.vouches = []mod.Vouch{}
		}
	}
	return c.vouches
}

// EscapeHatch is a `use c` or impure `use go` line of a required module
// version, with the vouch that covers it (decision 0156); xo audit
// lists them.
type EscapeHatch struct {
	Module  string // module path of the dependency
	Version string
	Kind    string // "c" or "go"
	Target  string // header or Go import path
	Span    token.Span
	// Claim is the `uses` of a `use c` line (nil: none written).
	Claim []string
	// Vouch is the vouch line that covers it, nil when unvouched.
	Vouch *mod.Vouch
	// Effects are the effects the build charges it.
	Effects []string
}

// escapeHatch checks the escape hatch u (kind "c" or "go", target the
// header or Go path) of module m. For a module of a required version
// (not the main module, not a workspace module) it needs a vouch of a
// consuming module for that exact version, else XO1316. It returns the
// vouch (nil when there is none) and whether m is such a dependency.
func (c *checker) escapeHatch(m *Module, u *ast.UseDecl, kind, target string, claim []string) (*mod.Vouch, bool) {
	if m == nil || m.IsStd || c.deps.depDirs == nil {
		return nil, false
	}
	r, ok := c.findModRoot(m.Dir)
	if !ok {
		return nil, false
	}
	dm, ok := c.deps.depDirs[r.dir]
	if !ok || dm.Workspace {
		return nil, false
	}
	v, stale := mod.FindVouch(c.consumerVouches(), dm.Path, dm.Version, kind, target)
	h := EscapeHatch{Module: dm.Path, Version: dm.Version, Kind: kind, Target: target, Span: u.Sp, Claim: claim, Vouch: v}
	switch {
	case kind == "go":
		h.Effects = []string{"go"}
	case v != nil && v.Effects != nil:
		h.Effects = append([]string{}, v.Effects.Effects...)
	default:
		h.Effects = []string{"ffi"}
	}
	c.hatches = append(c.hatches, h)
	if v != nil {
		return v, true
	}
	consumer := filepath.Join(relDir(c.deps.mainDir), "xo.mod")
	var d *diag.Diagnostic
	if kind == "c" {
		what := "its C code"
		if claim != nil {
			list := "none"
			if len(claim) > 0 {
				list = strings.Join(claim, ", ")
			}
			what = "its claim `uses " + list + "`"
		}
		d = c.errorf(diag.UnvouchedEscape, u.Sp, "`use c \"%s\"` of %s %s is not vouched for by the consuming module: %s counts only with a vouch, so the binding performs `ffi`", target, dm.Path, dm.Version, what)
	} else {
		d = c.errorf(diag.UnvouchedEscape, u.Sp, "`use go \"%s\"` of %s %s is not vouched for by the consuming module: Go code reaches files, the network, and processes without a capability", target, dm.Path, dm.Version)
	}
	for _, s := range stale {
		pos := token.Pos{Line: s.Line, Col: 1}
		note(d, token.Span{File: relDir(s.File), Start: pos, End: pos}, "this vouch reviewed %s %s; the build selects %s, whose code is not reviewed", s.Path, s.Version, dm.Version)
	}
	note(d, token.Span{}, "after reviewing the code, add to %s: %s", consumer, mod.SuggestVouch(dm.Path, dm.Version, kind, target, claim))
	note(d, token.Span{}, "only the consuming module vouches for a dependency's escape hatches; a vouch names the exact version it reviewed (decision 0156)")
	return nil, true
}

// EscapeHatches are the escape hatches of required module versions the
// check met, with their vouches (decision 0156).
func (r *Result) EscapeHatches() []EscapeHatch {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.hatches
}

// ConsumerVouches are the vouch lines of the consuming modules.
func (r *Result) ConsumerVouches() []mod.Vouch {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.consumerVouches()
}
