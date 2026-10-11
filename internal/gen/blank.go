package gen

import (
	"bytes"
	goast "go/ast"
	"go/parser"
	"go/token"
)

// dropBlankUses removes the `_ = x` statements of a generated Go file
// that Go does not need. The generator writes one after most bindings,
// so a binding the Xo program never reads still compiles ("declared and
// not used"); most are read anyway. A `_ = x` goes when x is a
// parameter, not local to a function (package level, or another file's),
// or a local that is read elsewhere. When unsure it stays: a wrong
// removal fails to compile, it never changes behavior. The file's
// identifiers are resolved by go/parser's scopes, so shadowing and
// closures count right. src is returned unchanged if it does not parse.
func dropBlankUses(src []byte) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return src
	}
	notUse := map[*goast.Ident]bool{}
	type blank struct {
		stmt *goast.AssignStmt
		obj  *goast.Object
	}
	var blanks []blank
	local := map[*goast.Object]bool{}
	goast.Inspect(f, func(n goast.Node) bool {
		switch n := n.(type) {
		case *goast.AssignStmt:
			if n.Tok == token.ASSIGN && len(n.Lhs) == 1 && len(n.Rhs) == 1 {
				if l, ok := n.Lhs[0].(*goast.Ident); ok && l.Name == "_" {
					if r, ok := n.Rhs[0].(*goast.Ident); ok {
						notUse[r] = true
						blanks = append(blanks, blank{n, r.Obj})
					}
				}
			}
			for _, l := range n.Lhs {
				if id, ok := l.(*goast.Ident); ok {
					notUse[id] = true
					if n.Tok == token.DEFINE && id.Obj != nil {
						local[id.Obj] = true
					}
				}
			}
		case *goast.IncDecStmt:
			if id, ok := n.X.(*goast.Ident); ok {
				notUse[id] = true
			}
		case *goast.RangeStmt:
			for _, e := range []goast.Expr{n.Key, n.Value} {
				if id, ok := e.(*goast.Ident); ok {
					notUse[id] = true
					if n.Tok == token.DEFINE && id.Obj != nil {
						local[id.Obj] = true
					}
				}
			}
		case *goast.BlockStmt:
			// `var x T` inside a function
			for _, s := range n.List {
				if d, ok := s.(*goast.DeclStmt); ok {
					if g, ok := d.Decl.(*goast.GenDecl); ok && g.Tok == token.VAR {
						for _, sp := range g.Specs {
							for _, id := range sp.(*goast.ValueSpec).Names {
								notUse[id] = true
								if id.Obj != nil {
									local[id.Obj] = true
								}
							}
						}
					}
				}
			}
		case *goast.TypeSwitchStmt:
			// the symbol of `switch v := x.(type)` is checked per
			// clause; keep its blank uses
			if a, ok := n.Assign.(*goast.AssignStmt); ok {
				for _, l := range a.Lhs {
					if id, ok := l.(*goast.Ident); ok {
						notUse[id] = true
						if id.Obj != nil {
							local[id.Obj] = true
						}
					}
				}
			}
		}
		return true
	})
	uses := map[*goast.Object]int{}
	goast.Inspect(f, func(n goast.Node) bool {
		if id, ok := n.(*goast.Ident); ok && id.Obj != nil && id.Obj.Kind == goast.Var && !notUse[id] {
			uses[id.Obj]++
		}
		return true
	})
	typeSwitchVar := map[*goast.Object]bool{}
	goast.Inspect(f, func(n goast.Node) bool {
		if ts, ok := n.(*goast.TypeSwitchStmt); ok {
			if a, ok := ts.Assign.(*goast.AssignStmt); ok {
				for _, l := range a.Lhs {
					if id, ok := l.(*goast.Ident); ok && id.Obj != nil {
						typeSwitchVar[id.Obj] = true
					}
				}
			}
		}
		return true
	})
	// the byte ranges of the lines to drop
	type span struct{ from, to int }
	var drop []span
	for _, b := range blanks {
		keep := b.obj != nil && local[b.obj] && (uses[b.obj] == 0 || typeSwitchVar[b.obj])
		if keep {
			continue
		}
		from := fset.Position(b.stmt.Pos()).Offset
		to := fset.Position(b.stmt.End()).Offset
		// only a statement alone on its line
		ls := bytes.LastIndexByte(src[:from], '\n') + 1
		le := bytes.IndexByte(src[to:], '\n')
		if le < 0 || len(bytes.TrimSpace(src[ls:from])) > 0 || len(bytes.TrimSpace(src[to:to+le])) > 0 {
			continue
		}
		drop = append(drop, span{ls, to + le + 1})
	}
	if len(drop) == 0 {
		return src
	}
	out := make([]byte, 0, len(src))
	at := 0
	for _, d := range drop {
		if d.from < at {
			continue
		}
		out = append(out, src[at:d.from]...)
		at = d.to
	}
	return append(out, src[at:]...)
}
