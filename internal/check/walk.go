package check

import (
	"reflect"

	"github.com/xo-lang/xo/internal/ast"
)

// children returns the direct child nodes of n in source order. Patterns
// and types are included only where they hold expressions.
func children(n ast.Node) []ast.Node {
	var out []ast.Node
	add := func(ns ...ast.Node) {
		for _, x := range ns {
			if x != nil && !isNilNode(x) {
				out = append(out, x)
			}
		}
	}
	switch x := n.(type) {
	case *ast.File:
		for _, d := range x.Decls {
			add(d)
		}
	case *ast.FnDecl:
		for _, cl := range x.Clauses {
			switch c := cl.(type) {
			case *ast.RequiresClause:
				add(c.Cond, c.Else)
			case *ast.EnsuresClause:
				add(c.Cond)
			}
		}
		add(x.Body)
	case *ast.TestDecl:
		add(x.Body)
	case *ast.ConstDecl:
		add(x.Value)
	case *ast.Block:
		for _, s := range x.Stmts {
			add(s)
		}
	case *ast.ExprStmt:
		add(x.X)
	case *ast.LetStmt:
		add(x.Value)
	case *ast.VarStmt:
		add(x.Value)
	case *ast.AssignStmt:
		add(x.Target, x.Value)
	case *ast.DeferStmt:
		add(x.Body)
	case *ast.ReturnStmt:
		add(x.Value)
	case *ast.ForStmt:
		add(x.Iter, x.Bound, x.Body)
	case *ast.WhileStmt:
		add(x.Cond, x.Value, x.Bound, x.Body)
	case *ast.StringLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				add(p.Expr)
			}
		}
	case *ast.ParenExpr:
		add(x.X)
	case *ast.TupleExpr:
		for _, e := range x.Elems {
			add(e)
		}
	case *ast.ListLit:
		for _, e := range x.Elems {
			add(e)
		}
	case *ast.SetLit:
		for _, e := range x.Elems {
			add(e)
		}
	case *ast.MapLit:
		for _, e := range x.Entries {
			add(e.Key, e.Value)
		}
	case *ast.StructLit:
		for _, f := range x.Fields {
			add(f.Value)
		}
	case *ast.UnaryExpr:
		add(x.X)
	case *ast.BinaryExpr:
		add(x.X, x.Y)
	case *ast.IsExpr:
		add(x.X)
	case *ast.RangeExpr:
		add(x.Lo, x.Hi)
	case *ast.SelectorExpr:
		add(x.X)
	case *ast.CallExpr:
		add(x.Fun)
		for _, a := range x.Args {
			add(a.Value)
		}
	case *ast.TypeArgsExpr:
		add(x.X)
	case *ast.IndexExpr:
		add(x.X, x.Index)
	case *ast.TryExpr:
		add(x.X)
	case *ast.WithExpr:
		add(x.X)
		for _, f := range x.Fields {
			add(f.Value)
		}
	case *ast.IfExpr:
		add(x.Cond, x.Value, x.Then, x.Else)
	case *ast.MatchExpr:
		add(x.Subject)
		for _, a := range x.Arms {
			add(a.Guard, a.Body)
		}
	case *ast.ScopeExpr:
		add(x.Body)
	case *ast.SelectExpr:
		for _, a := range x.Arms {
			add(a.Op, a.After, a.Body)
		}
	case *ast.FnLit:
		add(x.Body)
	case *ast.WhileExpr:
		add(x.Body)
	}
	return out
}

// isNilNode reports whether an interface holds a typed nil pointer.
func isNilNode(n ast.Node) bool {
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// inspect calls f for n and every node below it in source order. If f
// returns false the children of that node are skipped.
func inspect(n ast.Node, f func(ast.Node) bool) {
	if n == nil || isNilNode(n) {
		return
	}
	if !f(n) {
		return
	}
	for _, ch := range children(n) {
		inspect(ch, f)
	}
}
