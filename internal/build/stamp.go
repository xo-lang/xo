package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
)

// Version stamping (decision 0165): `xo build --stamp NAME=VALUE` sets
// the value of `const NAME: Str = "..."` in the main module for this
// build, so a program can print its version (or commit, or build date)
// with no file read and no flag at run time. The constant's literal is
// replaced in the checked syntax tree before code generation, so every
// backend sees the stamped text, and constants built from it
// (`const BANNER: Str = NAME + " (xo)"`) follow.

// ParseStamp splits a `--stamp` argument `NAME=VALUE`.
func ParseStamp(arg string) (name, value string, err error) {
	name, value, ok := strings.Cut(arg, "=")
	if !ok || name == "" {
		return "", "", fmt.Errorf("--stamp %q: want NAME=VALUE (NAME is a `const NAME: Str` of the main module)", arg)
	}
	return name, value, nil
}

// applyStamps replaces the literals of the stamped constants of the root
// modules; a name that is not a stamped constant is XO1501.
func applyStamps(res *check.Result, stamps map[string]string) []diag.Diagnostic {
	if len(stamps) == 0 {
		return nil
	}
	var roots []*check.Module
	for _, m := range res.Info().AllModules() {
		if m.Root && !m.IsStd {
			roots = append(roots, m)
		}
	}
	names := make([]string, 0, len(stamps))
	for n := range stamps {
		names = append(names, n)
	}
	sort.Strings(names)
	var ds []diag.Diagnostic
	for _, name := range names {
		found := false
		for _, m := range roots {
			co, ok := m.Scope[name].(*check.ConstObj)
			if !ok || co.Decl == nil {
				continue
			}
			found = true
			lit, ok := co.Decl.Value.(*ast.StringLit)
			if !ok || !isStrType(co.Type) || !plainString(lit) {
				ds = append(ds, diag.Diagnostic{
					Code: string(diag.StampTarget), Severity: diag.Error,
					Message: fmt.Sprintf("--stamp %s: `%s` must be declared `const %s: Str = \"...\"` with a plain string literal to be stamped", name, name, name),
					Span:    diag.FromSpan(co.Decl.Span()),
				})
				continue
			}
			lit.Parts = []ast.StringPart{{Text: stamps[name]}}
		}
		if !found {
			span := diag.Span{}
			if len(roots) > 0 {
				span.File = roots[0].Dir
			}
			ds = append(ds, diag.Diagnostic{
				Code: string(diag.StampTarget), Severity: diag.Error,
				Message: fmt.Sprintf("--stamp %s: the main module has no constant `%s`; declare `const %s: Str = \"dev\"` and read it", name, name, name),
				Span:    span,
			})
		}
	}
	return ds
}

// isStrType reports whether t is the prelude's Str.
func isStrType(t check.Type) bool {
	n, ok := check.Resolve(t).(*check.Named)
	return ok && n.Obj.Name == "Str" && n.Obj.Module != nil && n.Obj.Module.IsPrelude
}

// plainString reports whether lit has no interpolation.
func plainString(lit *ast.StringLit) bool {
	for _, p := range lit.Parts {
		if p.Expr != nil {
			return false
		}
	}
	return true
}
