package check

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Help for an `if` without `else` where a value is expected (core 3.5:
// such an `if` has type `()`). Codex r4 ended a closure with
// `if a { n.add(1) } else if b { m.add(1) }`: the first branch made the
// closure's result `Int`, and the open `else if` then failed with a bare
// `XO0401 expected Int, found ()`. The note names the missing `else`;
// when the expected type came only from a sibling branch (it was open
// before the chain was checked), the fix discards the branch values with
// `let _ =` (core 2.6), so the chain is the statement it was meant to be.

// missingElseNote adds the note to the XO0401 that coercing the `()` of
// the open `if` x reported since diagnostic n.
func (c *checker) missingElseNote(x *ast.IfExpr, want Type, n int) {
	d := c.mismatchSince(n, x.Sp)
	if d == nil {
		return
	}
	w := deep(valueWant(want))
	note(d, token.Span{}, "this `if` has no `else`, so it is a statement of type `()` (core 3.5), but a value of type `%s` is expected here; add an `else` branch with a value of that type", w)
}

// valueWant is the value type an expectation asks for: T of a function or
// closure tail that returns `T ! E`.
func valueWant(want Type) Type {
	if w, ok := want.(*retWant); ok {
		return w.T
	}
	return want
}

// missingElseFix adds the discarding fix to the XO0401 of the open `if`
// that ends the chain x, when the expected type was open before the chain
// (wantOpen) and so came from a branch.
func (c *checker) missingElseFix(x *ast.IfExpr, wantOpen bool, n int) {
	if !wantOpen {
		return
	}
	last := x
	for {
		next, ok := last.Else.(*ast.IfExpr)
		if !ok {
			break
		}
		last = next
	}
	if last.Else != nil {
		return // the chain ends with `else { }`
	}
	d := c.mismatchSince(n, last.Sp)
	if d == nil || len(d.Fixes) > 0 {
		return
	}
	var edits []diag.Edit
	for e := x; e != nil; {
		if tail := blockTail(e.Then); tail != nil {
			t := c.exprType[tail]
			if v, et, isRes := c.resultParts(t); isRes && isOpenVar(et) {
				// the tail of a closure that may fail is recorded as
				// `Result[T, _]`; its value is T
				t = v
			}
			if t == nil || isBad(t) || c.isNever(t) || isUnit(t) {
				// nothing to discard
			} else if _, _, isRes := c.resultParts(t); isRes {
				return // a Result needs `.ignore_err("reason")`, not `let _ =`
			} else {
				edits = append(edits, insertEdit(tail.Span().File, tail.Span().Start, "let _ = "))
			}
		}
		e, _ = e.Else.(*ast.IfExpr)
	}
	if len(edits) > 0 {
		note(d, token.Span{}, "the expected type comes from the first branch; if the `if` is a statement, discard the branch values with `let _ =` (core 2.6)")
		fix(d, "discard the branch values with `let _ =`", edits...)
	}
}

// mismatchSince returns the XO0401 at sp reported since diagnostic n.
func (c *checker) mismatchSince(n int, sp token.Span) *diag.Diagnostic {
	for i := len(c.diags.Items) - 1; i >= n && i >= 0; i-- {
		d := c.diags.Items[i]
		if d.Code == string(diag.TypeMismatch) && d.Span.Line == sp.Start.Line && d.Span.Col == sp.Start.Col {
			return d
		}
	}
	return nil
}

// blockTail returns the expression that gives a block its value, or nil.
func blockTail(b *ast.Block) ast.Expr {
	if b == nil || len(b.Stmts) == 0 {
		return nil
	}
	if es, ok := b.Stmts[len(b.Stmts)-1].(*ast.ExprStmt); ok {
		return es.X
	}
	return nil
}
