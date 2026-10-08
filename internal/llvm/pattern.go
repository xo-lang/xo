package llvm

import (
	"strconv"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// pattern tests the value of type t at ptr against p, branching to fail
// when it does not match; on a match it falls through with the pattern's
// bindings stored. rename maps names bound by a later alternative of an or
// pattern to the bindings of the first (the arm body uses those).
func (f *fn) pattern(p ast.Pattern, ptr string, t *ty, fail string, rename map[string]*check.VarObj) {
	g := f.g
	if f.term {
		return
	}
	switch x := p.(type) {
	case *ast.WildcardPat:
		return
	case *ast.BindPat:
		v := g.info.VarOf(x.Name)
		if rename != nil {
			if r, ok := rename[x.Name.Name]; ok {
				v = r
			}
		}
		if v == nil {
			return
		}
		l := f.declare(v, x)
		val := f.load(t, ptr)
		f.retain(val)
		f.emit("store %s %s, ptr %s", t.ll, val.v, l.ptr)
	case *ast.LitPat:
		cur := f.load(t, ptr)
		lit := f.litValue(x.Value, t)
		c := f.compare("==", cur, lit, x)
		ok := f.label("lit")
		f.condBr(c.v, ok, fail)
		f.startBlock(ok)
	case *ast.RangePat:
		cur := f.load(t, ptr)
		lo := f.litValue(x.Lo, t)
		hi := f.litValue(x.Hi, t)
		c1 := f.compare(">=", cur, lo, x)
		op := "<"
		if x.Inclusive {
			op = "<="
		}
		c2 := f.compare(op, cur, hi, x)
		c := f.def("and i1 %s, %s", c1.v, c2.v)
		ok := f.label("rng")
		f.condBr(c, ok, fail)
		f.startBlock(ok)
	case *ast.OrPat:
		matched := f.label("or")
		var first map[string]*check.VarObj
		for i, alt := range x.Alts {
			next := fail
			if i < len(x.Alts)-1 {
				next = f.label("alt")
			}
			if i == 0 {
				first = bindsOf(g, alt)
				f.pattern(alt, ptr, t, next, rename)
			} else {
				f.pattern(alt, ptr, t, next, mergeRename(rename, first))
			}
			f.br(matched)
			if next != fail {
				f.startBlock(next)
			}
		}
		f.startBlock(matched)
	case *ast.TuplePat:
		if t.k != kTuple && !(t.k == kUnit && len(x.Elems) == 0) {
			g.unsupported(x, "this tuple pattern")
			return
		}
		for i, el := range x.Elems {
			fp := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", t.ll, ptr, i)
			f.pattern(el, fp, t.fields[i].t, fail, rename)
		}
	case *ast.VariantPat:
		vo := g.info.PatVariant(x)
		if vo == nil || t.k != kEnum {
			g.unsupported(x, "this variant pattern")
			return
		}
		idx, v := t.variant(vo.Name)
		if v == nil {
			g.unsupported(x, "this variant pattern")
			return
		}
		tag := f.def("load i64, ptr %s", ptr)
		c := f.def("icmp eq i64 %s, %d", tag, idx)
		ok := f.label("var")
		f.condBr(c, ok, fail)
		f.startBlock(ok)
		pos := 0
		for _, pf := range x.Fields {
			j := pos
			if pf.Name != nil {
				j = -1
				for k, vf := range vo.Fields {
					if vf.Name == pf.Name.Name {
						j = k
					}
				}
			} else {
				pos++
			}
			if j < 0 || j >= len(v.fields) {
				g.unsupported(pf, "this variant field pattern")
				continue
			}
			fp, ft := f.payloadPtr(ptr, t, idx, j, pf)
			f.pattern(pf.Pattern, fp, ft, fail, rename)
		}
	case *ast.StructPat:
		if t.k != kStruct {
			g.unsupported(x, "this struct pattern")
			return
		}
		for _, pf := range x.Fields {
			name := ""
			if pf.Name != nil {
				name = pf.Name.Name
			} else if bp, ok := pf.Pattern.(*ast.BindPat); ok {
				name = bp.Name.Name
			}
			i := t.fieldIndex(name)
			if i < 0 {
				g.unsupported(pf, "this struct field pattern")
				continue
			}
			fl := t.fields[i]
			fp := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", t.ll, ptr, i)
			if fl.boxed {
				fp = f.def("load ptr, ptr %s", fp)
			}
			f.pattern(pf.Pattern, fp, g.fieldTy(fl, pf), fail, rename)
		}
	case *ast.ListPat:
		g.unsupported(x, "list patterns")
	default:
		g.unsupported(p, "this pattern")
	}
}

// bindsOf maps the names a pattern binds to their bindings.
func bindsOf(g *gen, p ast.Pattern) map[string]*check.VarObj {
	out := map[string]*check.VarObj{}
	check.Inspect(p, func(n ast.Node) bool {
		if bp, ok := n.(*ast.BindPat); ok {
			if v := g.info.VarOf(bp.Name); v != nil {
				out[bp.Name.Name] = v
			}
		}
		return true
	})
	return out
}

func mergeRename(outer, first map[string]*check.VarObj) map[string]*check.VarObj {
	m := map[string]*check.VarObj{}
	for k, v := range outer {
		m[k] = v
	}
	for k, v := range first {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

// litValue is the value of a literal pattern.
func (f *fn) litValue(e ast.Expr, t *ty) val {
	switch x := e.(type) {
	case *ast.IntLit:
		if t.k == kFloat {
			return val{t, floatLit(x.Value)}
		}
		return val{t, x.Value}
	case *ast.FloatLit:
		return val{t, floatLit(x.Raw)}
	case *ast.UnaryExpr:
		if x.Op == token.Minus {
			switch y := x.X.(type) {
			case *ast.IntLit:
				if t.k == kFloat {
					return val{t, floatLit("-" + y.Value)}
				}
				if _, err := strconv.ParseInt("-"+y.Value, 10, 64); err == nil {
					return val{t, "-" + y.Value}
				}
			case *ast.FloatLit:
				return val{t, floatLit("-" + y.Raw)}
			}
		}
	case *ast.Ident:
		if x.Name == "true" || x.Name == "false" {
			return val{t, x.Name}
		}
	case *ast.StringLit:
		return f.stringLit(x)
	}
	f.g.unsupported(e, "this literal pattern")
	return val{t, "poison"}
}

// match emits `match subject { arms }` delivering the arm's value to d.
func (f *fn) match(x *ast.MatchExpr, d dest) {
	subj := f.val(x.Subject)
	if f.term {
		return
	}
	p := f.spill(subj)
	end := f.label("mend")
	for _, arm := range x.Arms {
		next := f.label("arm")
		f.pattern(arm.Pattern, p, subj.t, next, nil)
		if arm.Guard != nil {
			gv := f.val(arm.Guard)
			body := f.label("guarded")
			f.condBr(gv.v, body, next)
			f.startBlock(body)
		}
		f.armBody(arm.Body, d)
		if !f.term {
			f.br(end)
		}
		f.startBlock(next)
	}
	f.faultCall("@xo_fault_unreachable(ptr %s)", f.g.cstr(f.g.loc(x)))
	f.startBlock(end)
}

// armBody emits a match arm body: an expression or a statement.
func (f *fn) armBody(s ast.Stmt, d dest) {
	if es, ok := s.(*ast.ExprStmt); ok {
		f.into(es.X, d)
		return
	}
	f.stmt(s)
	f.deliverUnit(d)
}

// ifExpr emits `if` and `if let`.
func (f *fn) ifExpr(x *ast.IfExpr, d dest) {
	then, els, end := f.label("then"), f.label("else"), f.label("iend")
	if x.Pattern != nil {
		v := f.val(x.Value)
		if f.term {
			return
		}
		pt := v.t
		p := f.spill(v)
		pat := x.Pattern
		// `if let e = opt` is shorthand for `if let Some(e) = opt`
		if bp, ok := pat.(*ast.BindPat); ok && f.g.optionElem(v.t.t) != nil {
			vt := f.g.info.VarOf(bp.Name)
			if vt != nil && f.g.key(f.t(vt.Type)) != v.t.key {
				fp, ft := f.payloadPtr(p, v.t, 1, 0, x)
				tag := f.def("load i64, ptr %s", p)
				c := f.def("icmp eq i64 %s, 1", tag)
				ok := f.label("some")
				f.condBr(c, ok, els)
				f.startBlock(ok)
				f.pattern(pat, fp, ft, els, nil)
				f.br(then)
				goto branches
			}
		}
		f.pattern(pat, p, pt, els, nil)
		f.br(then)
	} else {
		c := f.val(x.Cond)
		if f.term {
			return
		}
		f.condBr(c.v, then, els)
	}
branches:
	f.startBlock(then)
	f.block(x.Then, d)
	if !f.term {
		f.br(end)
	}
	f.startBlock(els)
	if x.Else == nil {
		f.deliverUnit(d)
	} else {
		f.into(x.Else, d)
	}
	if !f.term {
		f.br(end)
	}
	f.startBlock(end)
}
