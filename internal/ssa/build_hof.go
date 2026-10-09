package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// mapValuesHof lowers `m.values().any(<closure>)` and `.all(<closure>)`
// as a loop over m's live entries with the closure's body inline, its
// parameter bound to each value where it lies in the map: no list of the
// values, no closure call, and no count traffic for values the body only
// reads (the Go backend iterates in place too, rt.Map_values_seq). Map
// values shared between tasks (a Mutex's) have atomic counts, so copying
// them into a list and passing each to a closure cost several atomic
// updates per entry. ok is false when the call does not qualify: the
// body must not leave the closure early (`return`, `?`), defer, or assign
// (an inline assignment could reach a captured binding).
func (s *fn) mapValuesHof(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	fo := ci.Fn
	if fo == nil || fo.Recv == nil || fo.Recv.Name != "List" || fo.Recv.Module == nil || !fo.Recv.Module.IsPrelude {
		return nil, false
	}
	if fo.Name != "any" && fo.Name != "all" {
		return nil, false
	}
	if len(x.Args) != 1 || x.Args[0].Name != nil {
		return nil, false
	}
	lit, ok := x.Args[0].Value.(*ast.FnLit)
	if !ok || len(lit.Params) != 1 || lit.Body == nil {
		return nil, false
	}
	vc, ok := ci.Recv.(*ast.CallExpr)
	if !ok || len(vc.Args) != 0 {
		return nil, false
	}
	vci := s.b.info.CallOf(vc)
	if vci == nil || vci.Recv == nil || vci.Fn == nil || vci.Fn.Recv == nil || vci.Fn.Name != "values" || vci.Fn.Recv.Name != "Map" || vci.Fn.Recv.Module == nil || !vci.Fn.Recv.Module.IsPrelude {
		return nil, false
	}
	ct, isFn := check.Resolve(s.natCheck(lit)).(*check.Func)
	if !isFn || failErr(s.apply(ct.Err)) != nil || !inlineableBody(lit.Body) {
		return nil, false
	}
	ps := s.b.info.ClosureParams(lit)
	if len(ps) != 1 {
		return nil, false
	}
	t := s.typeOf(vci.Recv)
	if !t.Coll || len(t.Name) > 4 && t.Name[:4] == "Set[" {
		return nil, false
	}
	mv := s.valueAs(vci.Recv, t)
	m := s.b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	isAny := fo.Name == "any"
	keys := s.add(OpField, t.Fields[0].Type, 0, mv)
	vals := s.add(OpField, t.Fields[1].Type, 1, mv)
	live := s.add(OpField, t.Fields[2].Type, 2, mv)
	n := s.call("list.len", intT, "", keys)
	idx := local("i")
	s.writeVar(idx, s.cint(0))
	head, test, body, latch, done, exit := s.newBlock(), s.newBlock(), s.newBlock(), s.newBlock(), s.newBlock(), s.newBlock()
	res := exit.AddParam(boolT)
	s.jump(head)
	s.cur = head
	i := s.read(idx)
	s.cur.Branch(s.add(OpLt, boolT, nil, i, n), test, done)
	s.seal(done)
	s.cur = done
	s.jump(exit, s.bool(!isAny))
	s.seal(test)
	s.cur = test
	s.cur.Branch(s.call("list.index", boolT, "", live, i), body, latch)
	s.seal(body)
	s.cur = body
	if ps[0] != nil {
		s.writeVar(ps[0], s.call("list.index", mapVal(t), "", vals, i))
	}
	v := s.block(lit.Body)
	if !s.isDead() {
		c := s.coerce(v, boolT, lit.Body)
		if !isAny {
			c = s.add(OpNot, boolT, nil, c)
		}
		// any: stop at the first true; all: at the first false
		stop := s.newBlock()
		s.cur.Branch(c, stop, latch)
		s.seal(stop)
		s.cur = stop
		s.jump(exit, s.bool(isAny))
	}
	s.seal(latch)
	s.cur = latch
	s.writeVar(idx, s.add(OpAdd, intT, nil, s.read(idx), s.cint(1)))
	s.jump(head)
	s.seal(head)
	s.seal(exit)
	s.cur = exit
	return res, true
}

// inlineableBody reports whether a closure body can run inline in its
// caller: no `return`, `?`, `defer`, or assignment outside the closures
// it contains.
func inlineableBody(body *ast.Block) bool {
	ok := true
	check.Inspect(body, func(n ast.Node) bool {
		if !ok {
			return false
		}
		switch n.(type) {
		case *ast.FnLit:
			return false
		case *ast.ReturnStmt, *ast.TryExpr, *ast.DeferStmt, *ast.AssignStmt:
			ok = false
			return false
		}
		return true
	})
	return ok
}
