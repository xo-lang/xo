package check

import (
	"github.com/xo-lang/xo/internal/ast"
)

// whileLet checks `while let pattern = value { ... }` (core 3.5, decision
// 0134): the value is computed before each iteration, the pattern may be
// refutable, and the loop ends when it does not match. `while let e = opt`
// is shorthand for `while let Some(e) = opt`, as with `if let` (core 2.5).
func (c *checker) whileLet(x *ast.WhileStmt) {
	vt := c.infer(x.Value, nil)
	c.checkLoopBound(x.Bound, nil)
	c.loopBody(labelName(x.Label), false, x.Body, func() {
		c.refutablePattern(x.Pattern, vt)
	})
}

// refutablePattern binds the pattern of a `while let` against a value of
// type vt, reading a bare name against an Option as `Some(name)` (the
// same rule as `if let` in ifExpr).
func (c *checker) refutablePattern(pat ast.Pattern, vt Type) {
	if bp, ok := pat.(*ast.BindPat); ok && c.optionElem(vt) != nil {
		c.checkName(bp.Name, "variable")
		c.pattern(&ast.VariantPat{Base: bp.Base, Path: []*ast.Ident{{Base: bp.Base, Name: "Some"}}, Parens: true, Fields: []*ast.PatField{{Base: bp.Base, Pattern: bp}}}, vt, true)
		return
	}
	c.pattern(pat, vt, true)
}

// whileHead is the expression a `while` loop tests: its condition or the
// value of a `while let`; nil for `while { }`.
func whileHead(x *ast.WhileStmt) ast.Expr {
	if x.Value != nil {
		return x.Value
	}
	return x.Cond
}
