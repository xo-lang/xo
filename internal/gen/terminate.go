package gen

import (
	goast "go/ast"
	"go/parser"
	"go/token"
)

// terminates reports whether body, the Go statements of a function body,
// ends in a terminating statement (Go spec, "Terminating statements"), so
// the function needs no trailing panic. An if/else or a switch whose
// branches all return is one; endsInReturn only sees a final return.
func terminates(body string) bool {
	f, err := parser.ParseFile(token.NewFileSet(), "", "package p\nfunc _() {\n"+body+"\n}\n", parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	return terminatingList(f.Decls[0].(*goast.FuncDecl).Body.List)
}

func terminatingList(l []goast.Stmt) bool {
	// trailing empty statements do not count
	for len(l) > 0 {
		if _, ok := l[len(l)-1].(*goast.EmptyStmt); !ok {
			break
		}
		l = l[:len(l)-1]
	}
	return len(l) > 0 && terminating(l[len(l)-1], "")
}

func terminating(s goast.Stmt, label string) bool {
	switch s := s.(type) {
	case *goast.ReturnStmt:
		return true
	case *goast.BranchStmt:
		return s.Tok == token.GOTO
	case *goast.ExprStmt:
		c, ok := s.X.(*goast.CallExpr)
		if !ok {
			return false
		}
		id, ok := c.Fun.(*goast.Ident)
		return ok && id.Name == "panic"
	case *goast.BlockStmt:
		return terminatingList(s.List)
	case *goast.IfStmt:
		return s.Else != nil && terminatingList(s.Body.List) && terminating(s.Else, "")
	case *goast.LabeledStmt:
		return terminating(s.Stmt, s.Label.Name)
	case *goast.ForStmt:
		return s.Cond == nil && !breaks(s.Body, label)
	case *goast.SwitchStmt:
		return clausesTerminate(s.Body, label)
	case *goast.TypeSwitchStmt:
		return clausesTerminate(s.Body, label)
	case *goast.SelectStmt:
		return clausesTerminate(s.Body, label)
	}
	return false
}

// clausesTerminate is the rule for switch and select: no break out of the
// statement, a default (switch only), and every clause ends terminating
// (or, in a switch, in fallthrough).
func clausesTerminate(body *goast.BlockStmt, label string) bool {
	if breaks(body, label) {
		return false
	}
	def := false
	for _, c := range body.List {
		var list []goast.Stmt
		switch c := c.(type) {
		case *goast.CaseClause:
			def = def || c.List == nil
			list = c.Body
		case *goast.CommClause:
			def = true
			list = c.Body
		}
		if n := len(list); n > 0 {
			if b, ok := list[n-1].(*goast.BranchStmt); ok && b.Tok == token.FALLTHROUGH {
				continue
			}
		}
		if !terminatingList(list) {
			return false
		}
	}
	return def
}

// breaks reports whether n holds a break out of the statement whose body
// it is: an unlabeled break outside nested loops, switches, and selects,
// or a break naming label. Function literals are their own scope.
func breaks(n goast.Node, label string) bool {
	found := false
	var walk func(n goast.Node, nested bool)
	walk = func(n goast.Node, nested bool) {
		goast.Inspect(n, func(m goast.Node) bool {
			if found || m == nil {
				return false
			}
			switch m := m.(type) {
			case *goast.FuncLit:
				return false
			case *goast.BranchStmt:
				if m.Tok == token.BREAK && (m.Label == nil && !nested || m.Label != nil && m.Label.Name == label) {
					found = true
				}
			case *goast.ForStmt, *goast.RangeStmt, *goast.SwitchStmt, *goast.TypeSwitchStmt, *goast.SelectStmt:
				if m != n {
					walk(m, true)
					return false
				}
			}
			return true
		})
	}
	walk(n, false)
	return found
}
