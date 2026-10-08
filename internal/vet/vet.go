// Package vet implements `xo vet` (core 10.1): advisory checks over a
// checked program. Vet never reports errors; every finding is a warning
// (likely mistakes) or a note (inventories meant for review).
//
// Checks:
//   - the warnings of `xo check` (holes XO0900, XO0305, XO0804);
//   - XO0703: an effect named in a `uses` clause that the body never
//     performs (core 6.2: declaring more is allowed and noted by vet);
//   - XO0207: `x ?? [].f()`, a method chain on the right of `??` that
//     starts at a literal, so it applies only to the default;
//   - XO0208: every `.ignore_err("reason")`, with its reason, so discarded
//     errors can be reviewed;
//   - XO1103: per module, the `use go` packages and the functions that
//     perform `go` (decision 0041);
//   - XO0901: `dbg(...)` left in code (core 11.4).
package vet

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Report is the outcome of a vet run.
type Report struct {
	Modules     []string
	Diagnostics []diag.Diagnostic
	// CheckErrors counts the errors `xo check` reports for the same
	// program. Vet does not repeat them, but its findings may be
	// incomplete while they exist.
	CheckErrors int
}

// Vet checks the modules under dirs and runs the advisory checks on the
// root modules.
func Vet(dirs []string) (*Report, error) {
	res, err := check.Check(dirs)
	if err != nil {
		return nil, err
	}
	return Run(res), nil
}

// Run runs the advisory checks on a finished check run.
func Run(res *check.Result) *Report {
	r := &Report{}
	for _, d := range res.Diagnostics {
		if d.Severity == diag.Error {
			r.CheckErrors++
			continue
		}
		r.Diagnostics = append(r.Diagnostics, d)
	}
	v := &vetter{info: res.Info(), r: r}
	for _, m := range res.Modules {
		r.Modules = append(r.Modules, m.Path)
		v.module(m)
	}
	diag.Sort(r.Diagnostics)
	return r
}

type vetter struct {
	info *check.Info
	r    *Report
}

func (v *vetter) add(sev diag.Severity, code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	d := diag.New(code, sp, format, args...)
	d.Severity = sev
	v.r.Diagnostics = append(v.r.Diagnostics, d)
	return &v.r.Diagnostics[len(v.r.Diagnostics)-1]
}

func (v *vetter) module(m *check.Module) {
	for _, f := range m.Files {
		for _, d := range f.AST.Decls {
			v.exprs(d)
		}
	}
	for _, fo := range v.info.Funcs(m) {
		v.unusedEffects(fo)
	}
	v.goReport(m)
}

// unusedEffects reports effects named in the `uses` clauses of fo that its
// body never performs (XO0703).
func (v *vetter) unusedEffects(fo *check.FuncObj) {
	if fo.Decl == nil || fo.Decl.Body == nil || fo.Module.IsStd {
		return
	}
	performed := v.info.Performed(fo)
	labels := map[string]bool{}
	params := map[string]bool{}
	for _, e := range performed {
		if e.Param {
			params[e.Effect] = true
		} else {
			labels[e.Effect] = true
		}
	}
	isParam := map[string]bool{}
	for _, tp := range fo.Decl.TypeParams {
		if tp.Effects {
			isParam[tp.Name.Name] = true
		}
	}
	for _, cl := range fo.Decl.Clauses {
		u, ok := cl.(*ast.UsesClause)
		if !ok {
			continue
		}
		for _, e := range u.Effects {
			name := e.Name.Name
			if e.Sub != nil {
				name += "." + e.Sub.Name
			}
			used := false
			if isParam[name] {
				used = params[name]
			} else {
				for l := range labels {
					if l == name || strings.HasPrefix(l, name+".") || strings.HasPrefix(name, l+".") {
						used = true
					}
				}
			}
			if used {
				continue
			}
			d := v.add(diag.Warning, diag.UnusedEffect, e.Sp, "`%s` declares `uses %s` but never performs it", fo.DisplayName(), name)
			d.Notes = append(d.Notes, diag.NoteItem{Message: "declaring more effects than the body performs is allowed (core 6.2); keep it only if callers should already budget for it, such as an interface or a planned change"})
			if len(u.Effects) == 1 {
				d.Fixes = append(d.Fixes, diag.Fix{Title: "remove the `uses` clause", Edits: []diag.Edit{{Span: diag.FromSpan(u.Sp), Replace: ""}}})
			}
		}
	}
}

// exprs runs the expression level checks over one declaration.
func (v *vetter) exprs(d ast.Decl) {
	check.Inspect(d, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if x.Op == token.Coalesce {
				v.coalesceChain(x)
			}
		case *ast.CallExpr:
			v.call(x)
		}
		return true
	})
}

// coalesceChain reports `x ?? [].f()` (XO0207): `??` binds loosest, so the
// chain applies to the literal default only.
func (v *vetter) coalesceChain(b *ast.BinaryExpr) {
	base, sel := chainBase(b.Y)
	if sel == "" || !isLiteral(base) {
		return
	}
	d := v.add(diag.Warning, diag.CoalesceChainTrap, b.Y.Span(),
		"`.%s` applies only to the default `%s`, not to the result of `??`, because `??` binds loosest", sel, litText(base))
	d.Notes = append(d.Notes, diag.NoteItem{Message: "if the whole value was meant, parenthesize the `??`; if only the default was meant, bind it to a name first"})
	d.Fixes = append(d.Fixes, diag.Fix{Title: "apply the chain to the result of `??`", Edits: []diag.Edit{
		{Span: diag.PointSpan(b.Sp.File, b.X.Span().Start), Replace: "("},
		{Span: diag.PointSpan(b.Sp.File, base.Span().End), Replace: ")"},
	}})
}

// chainBase returns the innermost operand of a postfix chain and the first
// selector applied to it ("" when the chain has no selector).
func chainBase(e ast.Expr) (ast.Expr, string) {
	sel := ""
	for {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			sel = x.Sel.Name
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		case *ast.IndexExpr:
			e = x.X
		case *ast.TypeArgsExpr:
			e = x.X
		case *ast.TryExpr:
			e = x.X
		default:
			return e, sel
		}
	}
}

func isLiteral(e ast.Expr) bool {
	switch e.(type) {
	case *ast.ListLit, *ast.MapLit, *ast.SetLit, *ast.StringLit, *ast.IntLit, *ast.FloatLit,
		*ast.DurationLit, *ast.RuneLit, *ast.BytesLit, *ast.StructLit, *ast.TupleExpr:
		return true
	}
	return false
}

func litText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.ListLit:
		if len(x.Elems) == 0 {
			return "[]"
		}
		return "[...]"
	case *ast.MapLit:
		if len(x.Entries) == 0 {
			return "{}"
		}
		return "{...}"
	case *ast.SetLit:
		return "#{...}"
	case *ast.StringLit:
		if len(x.Parts) == 0 {
			return `""`
		}
		return "string literal"
	case *ast.IntLit:
		return x.Raw
	case *ast.FloatLit:
		return x.Raw
	case *ast.DurationLit:
		return x.Raw
	}
	return "literal"
}

// call reports `.ignore_err` (XO0208) and `dbg` (XO0901) calls.
func (v *vetter) call(call *ast.CallExpr) {
	ci := v.info.CallOf(call)
	if ci == nil || ci.Fn == nil || ci.Fn.Module == nil || !ci.Fn.Module.IsPrelude {
		return
	}
	switch {
	case ci.Fn.Name == "ignore_err" && ci.Fn.Recv != nil:
		reason := "(not a literal)"
		if len(call.Args) == 1 {
			if s, ok := call.Args[0].Value.(*ast.StringLit); ok && len(s.Parts) <= 1 {
				reason = ""
				if len(s.Parts) == 1 && s.Parts[0].Expr == nil {
					reason = s.Parts[0].Text
				}
			}
		}
		what := "a Result"
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if t := v.info.TypeOf(sel.X); t != nil {
				what = "`" + t.String() + "`"
			}
		}
		d := v.add(diag.Note, diag.IgnoredErrorReview, call.Sp, "error of %s ignored: %q", what, reason)
		if strings.TrimSpace(reason) == "" {
			d.Notes = append(d.Notes, diag.NoteItem{Message: "the reason is empty; say why this failure does not matter (core 2.6)"})
		}
	case ci.Fn.Name == "dbg" && ci.Fn.Recv == nil:
		d := v.add(diag.Warning, diag.DebugLeft, call.Sp, "`dbg(...)` left in code; `xo build --ci` rejects it (core 11.4)")
		if len(call.Args) == 1 {
			arg := call.Args[0].Value.Span()
			d.Fixes = append(d.Fixes, diag.Fix{Title: "remove `dbg`, keep the expression", Edits: []diag.Edit{
				{Span: diag.Span{File: call.Sp.File, Line: call.Sp.Start.Line, Col: call.Sp.Start.Col, EndLine: arg.Start.Line, EndCol: arg.Start.Col}, Replace: ""},
				{Span: diag.Span{File: call.Sp.File, Line: arg.End.Line, Col: arg.End.Col, EndLine: call.Sp.End.Line, EndCol: call.Sp.End.Col}, Replace: ""},
			}})
		}
	}
}

// goReport notes the Go packages a module imports with `use go` and the
// functions that perform `go` (XO1103), one note per module.
func (v *vetter) goReport(m *check.Module) {
	var uses []*ast.UseDecl
	seen := map[string]bool{}
	for _, f := range m.Files {
		for _, u := range f.AST.Uses {
			if u.Go && !seen[u.Path] {
				seen[u.Path] = true
				uses = append(uses, u)
			}
		}
	}
	type perf struct {
		name string
		sp   token.Span
	}
	var fns []perf
	for _, fo := range v.info.Funcs(m) {
		for _, e := range v.info.Performed(fo) {
			if e.Effect == "go" && !e.Param {
				fns = append(fns, perf{fo.DisplayName(), fo.Decl.Name.Sp})
			}
		}
	}
	if len(uses) == 0 && len(fns) == 0 {
		return
	}
	var pkgs []string
	for _, u := range uses {
		pkgs = append(pkgs, u.Path)
	}
	sort.Strings(pkgs)
	sp := token.Span{}
	if len(uses) > 0 {
		sp = uses[0].Sp
	} else {
		sp = fns[0].sp
	}
	names := make([]string, len(fns))
	for i, f := range fns {
		names[i] = f.name
	}
	msg := fmt.Sprintf("module `%s` uses Go packages [%s]; %d function(s) perform `go`: [%s]",
		m.Path, strings.Join(pkgs, ", "), len(fns), strings.Join(names, ", "))
	d := v.add(diag.Note, diag.GoUsageReport, sp, "%s", msg)
	for _, u := range uses {
		s := diag.FromSpan(u.Sp)
		d.Notes = append(d.Notes, diag.NoteItem{Message: fmt.Sprintf("use go %q", u.Path), Span: &s})
	}
	for _, f := range fns {
		s := diag.FromSpan(f.sp)
		d.Notes = append(d.Notes, diag.NoteItem{Message: fmt.Sprintf("`%s` performs `go`", f.name), Span: &s})
	}
}
