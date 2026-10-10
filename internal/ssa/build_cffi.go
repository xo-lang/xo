package ssa

import (
	"fmt"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// cCall lowers a call of a `use c` binding (decision 0113) to a call of
// its C adapter, `rt.<prefix><name>` (internal/cbind): scalars, Bools,
// and handles (the C pointer in a word) are one word, a Str or Bytes its
// bytes and length, a flat struct one word per field; a struct result
// is read back field by field through the adapter's getters, and a
// handle result is None for a NULL pointer.
func (s *fn) cCall(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	m := b.mod
	fo := ci.Fn
	bnd := fo.Module.C
	if fo.Recv != nil || bnd.Funcs[fo.Name] == nil {
		b.unsupported(x, "`"+fo.Module.Name+"."+fo.DisplayName()+"`")
		return s.undef(s.natOf(x))
	}
	sym := bnd.Sym(fo.Name)
	exprs := argExprs(fo.Params, x)
	var args []*Value
	for i, p := range fo.Params {
		e := exprs[i]
		if e == nil {
			b.unsupported(x, "a call with a missing argument")
			return s.undef(s.natOf(x))
		}
		t := b.typ(p.Type, x)
		v := s.valueAs(e, t)
		if t.Kind == KStruct {
			for k, f := range t.Fields {
				args = append(args, s.add(OpField, f.Type, k, v))
			}
			continue
		}
		args = append(args, v)
	}
	rt := b.typ(fo.Result, x)
	switch rt.Kind {
	case KUnit:
		s.rt(sym, nil, args...)
		return s.unit()
	case KStruct:
		s.rt(sym, nil, args...)
		var fs []*Value
		for k, f := range rt.Fields {
			fs = append(fs, s.rt(fmt.Sprintf("%s_r%d", sym, k), f.Type))
		}
		return s.add(OpStruct, rt, nil, fs...)
	case KEnum:
		// `H?` of a handle: None for a NULL pointer
		some := rt.VariantIndex("Some")
		none := rt.VariantIndex("None")
		if some < 0 || none < 0 {
			b.unsupported(x, "the result of `"+fo.Module.Name+"."+fo.Name+"`")
			return s.undef(rt)
		}
		intT := m.Builtin(KInt)
		p := s.rt(sym, intT, args...)
		join := s.newBlock()
		res := join.AddParam(rt)
		null, ok := s.branch(s.add(OpEq, m.Builtin(KBool), nil, p, s.cint(0)))
		s.cur = null
		s.jump(join, s.add(OpVariant, rt, none))
		s.cur = ok
		s.jump(join, s.add(OpVariant, rt, some, p))
		s.seal(join)
		s.cur = join
		return res
	}
	return s.rt(sym, rt, args...)
}
