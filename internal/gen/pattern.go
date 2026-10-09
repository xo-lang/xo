package gen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// match is a compiled pattern: conditions that test the subject (joined
// with &&, so later ones may assume earlier ones) and the bindings to make
// once they hold.
type match struct {
	conds []string
	binds []bind
	// predeclared bindings of or patterns, assigned inside the condition
	pre []string
}

type bind struct {
	v    *check.VarObj
	code string
	t    check.Type
	done bool // already declared (or pattern)
}

func (m *match) condition() string {
	if len(m.conds) == 0 {
		return "true"
	}
	return strings.Join(m.conds, " && ")
}

// pattern compiles p against the Go expression subj of type t.
func (f *fnGen) pattern(p ast.Pattern, subj string, t check.Type) *match {
	m := &match{}
	f.pat(p, subj, t, m)
	for _, l := range m.pre {
		f.line("%s", l)
	}
	return m
}

// declareBinds declares the bindings of a matched pattern.
func (f *fnGen) declareBinds(m *match) {
	for _, b := range m.binds {
		if b.done || b.v == nil {
			continue
		}
		name := f.declName(b.v)
		f.line("var %s %s = %s", name, f.goType(b.v.Type), b.code)
		f.line("_ = %s", name)
	}
}

func (f *fnGen) pat(p ast.Pattern, subj string, t check.Type, m *match) {
	if pt := f.info.PatType(p); pt != nil {
		t = pt
	}
	switch x := p.(type) {
	case nil, *ast.WildcardPat, *ast.BadPat:
	case *ast.BindPat:
		m.binds = append(m.binds, bind{v: f.info.VarOf(x.Name), code: subj, t: t})
	case *ast.LitPat:
		if id, ok := x.Value.(*ast.Ident); ok && (id.Name == "true" || id.Name == "false") {
			if id.Name == "true" {
				m.conds = append(m.conds, subj)
			} else {
				m.conds = append(m.conds, "!"+subj)
			}
			return
		}
		lit := f.litPatValue(x.Value, t)
		if f.g.isScalar(t) {
			m.conds = append(m.conds, fmt.Sprintf("%s == %s", subj, lit))
		} else {
			m.conds = append(m.conds, fmt.Sprintf("%sEq(%s, %s)", f.rt(), subj, lit))
		}
	case *ast.RangePat:
		lo, hi := f.litPatValue(x.Lo, t), f.litPatValue(x.Hi, t)
		op := "<"
		if x.Inclusive {
			op = "<="
		}
		m.conds = append(m.conds, fmt.Sprintf("(%s >= %s && %s %s %s)", subj, lo, subj, op, hi))
	case *ast.OrPat:
		f.orPat(x, subj, t, m)
	case *ast.TuplePat:
		tt, ok := check.Resolve(t).(*check.Tuple)
		if !ok {
			f.g.errorf(x, "tuple pattern on a non tuple")
			return
		}
		for i, e := range x.Elems {
			f.pat(e, fmt.Sprintf("%s.F%d", subj, i), tt.Elems[i], m)
		}
	case *ast.VariantPat:
		f.variantPat(x, subj, t, m)
	case *ast.StructPat:
		n := f.g.named(t)
		if n == nil {
			f.g.errorf(x, "struct pattern on a non struct")
			return
		}
		s := check.NewSubst(n.Obj.TParams, n.Args)
		for _, fp := range x.Fields {
			name := ""
			if fp.Name != nil {
				name = fp.Name.Name
			} else if bp, ok := fp.Pattern.(*ast.BindPat); ok {
				name = bp.Name.Name
			}
			for _, fo := range n.Obj.Fields {
				if fo.Name == name {
					fs := subj + "." + goField(name)
					if f.g.isBoxedField(n.Obj, fo) {
						fs += ".Get()"
					}
					f.pat(fp.Pattern, fs, s.Apply(fo.Type), m)
				}
			}
		}
	case *ast.ListPat:
		n := f.g.named(t)
		et := n.Args[0]
		k := len(x.Elems)
		if x.Rest < 0 {
			m.conds = append(m.conds, fmt.Sprintf("%sList_len(tk, %s) == %d", f.rt(), subj, k))
		} else {
			m.conds = append(m.conds, fmt.Sprintf("%sList_len(tk, %s) >= %d", f.rt(), subj, k))
		}
		for i, e := range x.Elems {
			var es string
			if x.Rest < 0 || i < x.Rest {
				es = fmt.Sprintf("%sList_Items(%s)[%d]", f.rt(), subj, i)
			} else {
				back := k - i
				es = fmt.Sprintf("%sList_Items(%s)[len(%sList_Items(%s))-%d]", f.rt(), subj, f.rt(), subj, back)
			}
			f.pat(e, es, et, m)
		}
	default:
		f.g.errorf(p, "unsupported pattern %T", p)
	}
}

// litPatValue is the Go value of a literal in a pattern.
func (f *fnGen) litPatValue(e ast.Expr, t check.Type) string {
	switch x := e.(type) {
	case *ast.IntLit:
		return f.intLit(x.Value, t)
	case *ast.UnaryExpr:
		if x.Op == token.Minus {
			if il, ok := x.X.(*ast.IntLit); ok {
				return f.intLit("-"+il.Value, t)
			}
			if fl, ok := x.X.(*ast.FloatLit); ok {
				return fmt.Sprintf("%s(-%s)", f.goType(t), fl.Raw)
			}
		}
	case *ast.FloatLit:
		return fmt.Sprintf("%s(%s)", f.goType(t), x.Raw)
	case *ast.RuneLit:
		if f.g.isIntType(t) {
			return f.intLit(strconv.Itoa(int(x.Value)), t)
		}
		return fmt.Sprintf("%sRune(%d)", f.rt(), x.Value)
	case *ast.StringLit:
		var s string
		for _, p := range x.Parts {
			s += p.Text
		}
		return strconv.Quote(s)
	case *ast.BytesLit:
		return fmt.Sprintf("%sBytes(%s)", f.rt(), strconv.Quote(x.Value))
	case *ast.DurationLit:
		return fmt.Sprintf("%sDuration(%s)", f.rt(), x.Nanos)
	}
	return f.val(e)
}

func (f *fnGen) orPat(x *ast.OrPat, subj string, t check.Type, m *match) {
	alts := make([]*match, len(x.Alts))
	hasBinds := false
	for i, a := range x.Alts {
		alts[i] = &match{}
		f.pat(a, subj, t, alts[i])
		if len(alts[i].binds) > 0 {
			hasBinds = true
		}
		m.pre = append(m.pre, alts[i].pre...)
	}
	if !hasBinds {
		var cs []string
		for _, a := range alts {
			cs = append(cs, "("+a.condition()+")")
		}
		m.conds = append(m.conds, "("+strings.Join(cs, " || ")+")")
		return
	}
	// bindings of the first alternative are declared up front and set by
	// whichever alternative matches
	byName := map[string]string{}
	for _, b := range alts[0].binds {
		if b.v == nil {
			continue
		}
		name := f.declName(b.v)
		m.pre = append(m.pre, fmt.Sprintf("var %s %s", name, f.goType(b.v.Type)), "_ = "+name)
		byName[b.v.Name] = name
		m.binds = append(m.binds, bind{v: b.v, done: true})
	}
	var b strings.Builder
	b.WriteString("func() bool {")
	for _, a := range alts {
		fmt.Fprintf(&b, " if %s {", a.condition())
		for _, bd := range a.binds {
			if bd.v == nil {
				continue
			}
			fmt.Fprintf(&b, " %s = %s;", byName[bd.v.Name], bd.code)
		}
		b.WriteString(" return true };")
	}
	b.WriteString(" return false }()")
	m.conds = append(m.conds, b.String())
}

func (f *fnGen) variantPat(x *ast.VariantPat, subj string, t check.Type, m *match) {
	v := f.info.PatVariant(x)
	if v == nil {
		f.g.errorf(x, "unresolved variant pattern")
		return
	}
	n := f.g.named(t)
	if v.Enum.Module.IsPrelude {
		switch v.Enum.Name {
		case "Option":
			if v.Name == "None" {
				m.conds = append(m.conds, "!"+subj+".Some")
				return
			}
			m.conds = append(m.conds, subj+".Some")
			f.variantFields(x, v, n, func(int) string { return subj + ".V" }, m)
			return
		case "Result":
			if v.Name == "Ok" {
				m.conds = append(m.conds, subj+".Ok")
				f.variantFields(x, v, n, func(int) string { return subj + ".V" }, m)
				return
			}
			m.conds = append(m.conds, "!"+subj+".Ok")
			f.variantFields(x, v, n, func(int) string { return subj + ".E" }, m)
			return
		}
	}
	m.conds = append(m.conds, fmt.Sprintf("%s.Tag == %d", subj, v.Index+1))
	if len(v.Fields) == 0 || !x.Parens {
		return
	}
	payload := fmt.Sprintf("%s.P.(%s)", subj, f.payloadType(v, n))
	f.variantFields(x, v, n, func(i int) string { return payload + "." + vfieldName(v.Fields[i], i) }, m)
}

// payloadType is the Go payload struct of a variant at an instantiation.
func (f *fnGen) payloadType(v *check.VariantObj, n *check.Named) string {
	var name string
	if v.Enum.Module.IsStd {
		name = f.rt() + stdPrefix(v.Enum.Module) + variantType(v)
	} else {
		name = f.pg.pkgRef(v.Enum.Module) + variantType(v)
	}
	if n != nil {
		if args := typeArgs(n); len(args) > 0 {
			name += "[" + f.pg.typeList(args) + "]"
		}
	}
	return name
}

// variantFields matches the field patterns of a variant pattern.
func (f *fnGen) variantFields(x *ast.VariantPat, v *check.VariantObj, n *check.Named, field func(i int) string, m *match) {
	var s *check.Subst
	if n != nil {
		s = check.NewSubst(v.Enum.TParams, n.Args)
	}
	used := make([]bool, len(v.Fields))
	pos := 0
	for _, fp := range x.Fields {
		idx := -1
		if fp.Name == nil {
			for pos < len(v.Fields) && used[pos] {
				pos++
			}
			idx = pos
			pos++
		} else {
			for i, vf := range v.Fields {
				if vf.Name == fp.Name.Name {
					idx = i
				}
			}
		}
		if idx < 0 || idx >= len(v.Fields) {
			continue
		}
		used[idx] = true
		ft := v.Fields[idx].Type
		if s != nil {
			ft = s.Apply(ft)
		}
		f.pat(fp.Pattern, field(idx), ft, m)
	}
}

// ---------------------------------------------------------------------------
// match and if

func (f *fnGen) matchExpr(x *ast.MatchExpr, d dest) {
	st := f.typeOf(x.Subject)
	subj := f.tmp("m")
	restore := f.inUnit(x.Subject)
	f.line("%s := %s", subj, f.val(x.Subject))
	restore()
	f.line("_ = %s", subj)
	if len(x.Arms) == 0 {
		f.line("%sUnreachable(%q)", f.rt(), f.g.loc(x))
		return
	}
	label := f.tmp("M")
	f.line("%s:", label)
	f.line("switch {")
	f.line("default:")
	f.ind++
	for _, arm := range x.Arms {
		f.at(arm)
		m := f.pattern(arm.Pattern, subj, st)
		f.line("if %s {", m.condition())
		f.ind++
		f.declareBinds(m)
		if arm.Guard != nil {
			g := f.val(arm.Guard)
			f.line("if %s {", g)
			f.ind++
		}
		f.armBody(arm.Body, d)
		f.line("break %s", label)
		if arm.Guard != nil {
			f.ind--
			f.line("}")
		}
		f.ind--
		f.line("}")
	}
	f.line("%sUnreachable(%q)", f.rt(), f.g.loc(x))
	f.ind--
	f.line("}")
}

// armBody generates a match or select arm body: an expression, a block,
// or a statement.
func (f *fnGen) armBody(s ast.Stmt, d dest) {
	if es, ok := s.(*ast.ExprStmt); ok {
		f.at(es)
		f.into(es.X, d)
		return
	}
	f.stmt(s)
	if !f.stmtNever(s) {
		f.deliverUnit(d)
	}
}

func (f *fnGen) ifExpr(x *ast.IfExpr, d dest) {
	if x.Cond != nil {
		restore := f.inUnit(x.Cond)
		c := f.val(x.Cond)
		restore()
		f.line("if %s {", c)
		f.ind++
		f.blockInline(x.Then, d)
		f.ind--
	} else {
		vt := f.nat(x.Value)
		v := f.tmp("l")
		restore := f.inUnit(x.Value)
		f.line("%s := %s", v, f.valAs(x.Value, vt))
		restore()
		var m *match
		if bp, ok := x.Pattern.(*ast.BindPat); ok && f.g.optionElem(vt) != nil {
			if pt := f.info.PatType(bp); pt != nil && f.goType(pt) != f.goType(vt) {
				// `if let e = opt` is `if let Some(e) = opt` (core 2.5)
				m = &match{conds: []string{v + ".Some"}, binds: []bind{{v: f.info.VarOf(bp.Name), code: v + ".V", t: pt}}}
			}
		}
		if m == nil {
			m = f.pattern(x.Pattern, v, vt)
		}
		f.line("if %s {", m.condition())
		f.ind++
		f.declareBinds(m)
		f.blockInline(x.Then, d)
		f.ind--
	}
	if x.Else == nil {
		f.line("}")
		if d.kind == dAssign || d.kind == dReturn {
			f.line("{")
			f.ind++
			f.deliverUnit(d)
			f.ind--
			f.line("}")
		}
		return
	}
	f.line("} else {")
	f.ind++
	switch e := x.Else.(type) {
	case *ast.Block:
		f.blockInline(e, d)
	default:
		f.into(e, d)
	}
	f.ind--
	f.line("}")
}

// blockInline generates a block whose braces are already open.
func (f *fnGen) blockInline(b *ast.Block, d dest) {
	if hasDefer(b) {
		f.iife(func() { f.stmts(b, d, true) })
		return
	}
	f.stmts(b, d, false)
}
