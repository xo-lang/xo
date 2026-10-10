package ssa

import "github.com/xo-lang/xo/internal/ast"

// whileLet lowers `while let p = v { body }` (decision 0137): each
// iteration computes v and tests p, as `if let` does; the loop exits when
// the pattern does not match. `while let e = opt` reads e as `Some(e)`.
func (s *fn) whileLet(label string, x *ast.WhileStmt) {
	tick := s.loopBound(x.Bound)
	head, exit := s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	v := s.value(x.Value)
	if bp, ok := x.Pattern.(*ast.BindPat); ok && v.Type.Kind == KEnum {
		some := v.Type.VariantIndex("Some")
		if vo := s.b.info.VarOf(bp.Name); vo != nil && some >= 0 && s.b.typ(s.apply(vo.Type), bp) != v.Type {
			m := s.b.mod
			s.test(s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, v), s.cint(int64(some))), exit)
			v = s.add(OpPayload, v.Type.Variants[some].Fields[0].Type, PayloadRef{some, 0}, v)
		}
	}
	s.pattern(v, x.Pattern, exit)
	tick()
	s.loops = append(s.loops, &loopCtx{label: label, brk: exit, cont: head, scopes: len(s.scopes), exits: len(s.exits)})
	s.block(x.Body)
	s.loops = s.loops[:len(s.loops)-1]
	s.jump(head)
	s.seal(head)
	s.seal(exit)
	s.cur = exit
}
