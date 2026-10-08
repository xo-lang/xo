package ssa

import (
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

func itoa(i int) string { return strconv.Itoa(i) }

// expr lowers an expression to a value of its natural type. An
// expression of type Never (return, break, fault) leaves the builder in
// a dead block and yields undef.
func (s *fn) expr(e ast.Expr) *Value {
	b := s.b
	m := b.mod
	switch x := e.(type) {
	case *ast.ParenExpr:
		return s.expr(x.X)
	case *ast.IntLit:
		t := s.natOf(x)
		if t.Kind == KFloat {
			f, _ := strconv.ParseFloat(x.Value, 64)
			return s.add(OpConst, t, f)
		}
		n, err := strconv.ParseInt(x.Value, 10, 64)
		if err != nil {
			b.unsupported(x, "this Int literal")
		}
		return s.add(OpConst, m.Builtin(KInt), n)
	case *ast.FloatLit:
		f, err := strconv.ParseFloat(strings.ReplaceAll(x.Raw, "_", ""), 64)
		if err != nil {
			b.unsupported(x, "this Float literal")
		}
		return s.add(OpConst, m.Builtin(KFloat), f)
	case *ast.StringLit:
		return s.stringLit(x)
	case *ast.UnitExpr:
		return s.unit()
	case *ast.Ident:
		return s.ident(x)
	case *ast.TupleExpr:
		t := s.natOf(x)
		var vs []*Value
		for i, el := range x.Elems {
			vs = append(vs, s.valueAs(el, t.Elems[i]))
		}
		return s.add(OpTuple, t, nil, vs...)
	case *ast.ListLit:
		t := s.typeOf(x)
		if t.Kind != KList {
			b.unsupported(x, "this list literal")
			return s.undef(t)
		}
		l := s.call("list.new", t, "")
		for _, el := range x.Elems {
			l = s.call("list.push", t, "", l, s.valueAs(el, t.Elems[0]))
		}
		return l
	case *ast.StructLit:
		return s.structLit(x)
	case *ast.WithExpr:
		v := s.expr(x.X)
		for _, fi := range x.Fields {
			i, ft := s.fieldIndex(v.Type, fi.Name.Name)
			if i < 0 {
				b.unsupported(fi, "this field")
				continue
			}
			v = s.add(OpSetField, v.Type, i, v, s.valueAs(fi.Value, ft))
		}
		return v
	case *ast.UnaryExpr:
		v := s.expr(x.X)
		switch x.Op {
		case token.Minus:
			r := s.add(OpNeg, v.Type, nil, v)
			if v.Type.Kind == KInt {
				r.Pos = b.pos(x)
			}
			return r
		case token.Bang, token.Tilde:
			return s.add(OpNot, v.Type, nil, v)
		}
	case *ast.BinaryExpr:
		return s.binary(x)
	case *ast.SelectorExpr:
		return s.selector(x)
	case *ast.IndexExpr:
		l := s.expr(x.X)
		if l.Type.Kind != KList {
			break
		}
		i := s.valueAs(x.Index, m.Builtin(KInt))
		return s.call("list.index", l.Type.Elems[0], b.pos(x), l, i)
	case *ast.CallExpr:
		return s.callExpr(x)
	case *ast.TryExpr:
		return s.try(x)
	case *ast.FnLit:
		return s.closure(x)
	case *ast.IsExpr:
		// `x is P`: the pattern test as a Bool (P binds nothing)
		v := s.value(x.X)
		boolT := m.Builtin(KBool)
		join, no := s.newBlock(), s.newBlock()
		res := join.AddParam(boolT)
		s.pattern(v, x.Pattern, no)
		s.jump(join, s.bool(true))
		s.seal(no)
		s.cur = no
		s.jump(join, s.bool(false))
		s.seal(join)
		s.cur = join
		return res
	case *ast.Block:
		return s.block(x)
	case *ast.IfExpr:
		return s.ifExpr(x)
	case *ast.MatchExpr:
		return s.match(x)
	case *ast.WhileExpr:
		s.while("", nil, x.Body)
		return s.unit()
	}
	b.unsupported(e, "this expression")
	return s.undef(s.natOf(e))
}

func (s *fn) ident(x *ast.Ident) *Value {
	b := s.b
	if vo := b.info.VarOf(x); vo != nil {
		return s.read(vo)
	}
	if vo := b.info.Ctor(x); vo != nil {
		return s.variant(s.natOf(x), vo.Name, nil, x)
	}
	switch x.Name {
	case "true", "false":
		return s.bool(x.Name == "true")
	}
	if c, ok := b.info.Ref(x).(*check.ConstObj); ok && c.Decl != nil {
		return s.valueAs(c.Decl.Value, s.natOf(x))
	}
	if fo, ok := b.info.Ref(x).(*check.FuncObj); ok {
		return s.funcValue(fo, x)
	}
	b.unsupported(x, "the name `"+x.Name+"` here")
	return s.undef(s.natOf(x))
}

func (s *fn) selector(x *ast.SelectorExpr) *Value {
	b := s.b
	if vo := b.info.Ctor(x); vo != nil {
		return s.variant(s.natOf(x), vo.Name, nil, x)
	}
	if c, ok := b.info.Ref(x).(*check.ConstObj); ok && c.Decl != nil {
		return s.valueAs(c.Decl.Value, s.natOf(x))
	}
	v := s.expr(x.X)
	if v.Type.Kind == KCap {
		return s.add(OpCap, s.natOf(x), x.Sel.Name, v)
	}
	if i, ft := s.fieldIndex(v.Type, x.Sel.Name); i >= 0 {
		return s.add(OpField, ft, i, v)
	}
	b.unsupported(x, "the selector `."+x.Sel.Name+"`")
	return s.undef(s.natOf(x))
}

// variant builds variant name of enum t from field values.
func (s *fn) variant(t *Type, name string, args []*Value, at ast.Node) *Value {
	i := t.VariantIndex(name)
	if t.Kind != KEnum || i < 0 || len(args) != len(t.Variants[i].Fields) {
		s.b.unsupported(at, "constructing `"+name+"` here")
		return s.undef(t)
	}
	return s.add(OpVariant, t, i, args...)
}

func (s *fn) stringLit(x *ast.StringLit) *Value {
	m := s.b.mod
	st := m.Builtin(KStr)
	var acc *Value
	cat := func(v *Value) {
		if acc == nil {
			acc = v
		} else {
			acc = s.call("str.concat", st, "", acc, v)
		}
	}
	for _, p := range x.Parts {
		if p.Expr == nil {
			if p.Text != "" || len(x.Parts) == 1 {
				cat(s.add(OpConst, st, p.Text))
			}
			continue
		}
		v := s.expr(p.Expr)
		switch v.Type.Kind {
		case KStr:
		case KInt, KFloat:
			if acc != nil {
				// append the number's text without a temporary string
				acc = s.call("str.concat_"+strings.ToLower(v.Type.Name), st, "", acc, v)
				continue
			}
			if v.Type.Kind == KInt {
				v = s.call("str.of_int", st, "", v)
			} else {
				v = s.call("str.of_float", st, "", v)
			}
		case KBool:
			v = s.call("str.of_bool", st, "", v)
		default:
			s.b.unsupported(p.Expr, "interpolating a value of type `"+v.Type.Name+"`")
		}
		cat(v)
	}
	if acc == nil {
		return s.add(OpConst, st, "")
	}
	return acc
}

func (s *fn) structLit(x *ast.StructLit) *Value {
	b := s.b
	t := s.natOf(x)
	if t.Kind != KStruct {
		b.unsupported(x, "this struct literal")
		return s.undef(t)
	}
	named, _ := check.Resolve(check.Deep(b.info.NaturalType(x))).(*check.Named)
	vals := make([]*Value, len(t.Fields))
	// fields in source order, then defaults
	for _, fi := range x.Fields {
		i := t.FieldIndex(fi.Name.Name)
		if i < 0 {
			b.unsupported(fi, "this field")
			continue
		}
		vals[i] = s.valueAs(fi.Value, t.Fields[i].Type)
	}
	for i := range vals {
		if vals[i] != nil {
			continue
		}
		if named != nil && i < len(named.Obj.Fields) && named.Obj.Fields[i].Default != nil {
			vals[i] = s.valueAs(named.Obj.Fields[i].Default, t.Fields[i].Type)
			continue
		}
		b.unsupported(x, "a struct literal without the field `"+t.Fields[i].Name+"`")
		vals[i] = s.undef(t.Fields[i].Type)
	}
	return s.add(OpStruct, t, nil, vals...)
}

var arithOps = map[token.Kind]Op{
	token.Plus: OpAdd, token.Minus: OpSub, token.Star: OpMul, token.Slash: OpDiv, token.Percent: OpRem,
	token.Amp: OpAnd, token.Pipe: OpOr, token.Caret: OpXor, token.Shl: OpShl, token.Shr: OpShr,
}

var cmpOps = map[token.Kind]Op{
	token.Eq: OpEq, token.NotEq: OpNe, token.Lt: OpLt, token.LtEq: OpLe, token.Gt: OpGt, token.GtEq: OpGe,
}

// arith lowers `l op r`; Int operations that can fault carry the
// position (Pos set means checked).
func (s *fn) arith(op token.Kind, l, r *Value, at ast.Node) *Value {
	if l.Type.Kind == KStr && op == token.Plus {
		return s.call("str.concat", l.Type, "", l, r)
	}
	o, ok := arithOps[op]
	if !ok || (l.Type.Kind != KInt && l.Type.Kind != KFloat && l.Type.Kind != KBool) {
		s.b.unsupported(at, "this operator")
		return s.undef(l.Type)
	}
	v := s.add(o, l.Type, nil, l, r)
	if l.Type.Kind == KInt {
		switch o {
		case OpAdd, OpSub, OpMul, OpDiv, OpRem, OpShl, OpShr:
			v.Pos = s.b.pos(at)
		}
	}
	return v
}

func (s *fn) binary(x *ast.BinaryExpr) *Value {
	b := s.b
	m := b.mod
	boolT := m.Builtin(KBool)
	switch x.Op {
	case token.AndAnd, token.OrOr:
		// short circuit: join(l ? r : false) or join(l ? true : r)
		l := s.valueAs(x.X, boolT)
		join := s.newBlock()
		res := join.AddParam(boolT)
		yes, no := s.branch(l)
		if x.Op == token.AndAnd {
			s.cur = no
			s.jump(join, s.bool(false))
			s.cur = yes
		} else {
			s.cur = yes
			s.jump(join, s.bool(true))
			s.cur = no
		}
		s.jump(join, s.valueAs(x.Y, boolT))
		s.seal(join)
		s.cur = join
		return res
	case token.Coalesce:
		o := s.expr(x.X)
		t := s.typeOf(x)
		some := o.Type.VariantIndex("Some")
		if o.Type.Kind != KEnum || some < 0 {
			b.unsupported(x, "`??` on this value")
			return s.undef(t)
		}
		join := s.newBlock()
		res := join.AddParam(t)
		c := s.add(OpEq, boolT, nil, s.add(OpTag, m.Builtin(KInt), nil, o), s.cint(int64(some)))
		yes, no := s.branch(c)
		s.cur = yes
		s.jump(join, s.coerce(s.add(OpPayload, o.Type.Variants[some].Fields[0].Type, PayloadRef{some, 0}, o), t, x))
		s.cur = no
		s.jump(join, s.valueAs(x.Y, t))
		s.seal(join)
		s.cur = join
		return res
	}
	l := s.expr(x.X)
	r := s.valueAs(x.Y, l.Type)
	if o, ok := cmpOps[x.Op]; ok {
		switch l.Type.Kind {
		case KInt, KFloat, KBool:
			return s.add(o, boolT, nil, l, r)
		case KStr:
			if o == OpEq || o == OpNe {
				eq := s.call("str.eq", boolT, "", l, r)
				if o == OpNe {
					return s.add(OpNot, boolT, nil, eq)
				}
				return eq
			}
			c := s.call("str.cmp", m.Builtin(KInt), "", l, r)
			return s.add(o, boolT, nil, c, s.cint(0))
		case KStruct, KTuple, KEnum, KList, KUnit:
			// structural equality; backends generate a helper per type
			if o == OpEq || o == OpNe {
				return s.add(o, boolT, nil, l, r)
			}
		}
		b.unsupported(x, "comparing values of type `"+l.Type.Name+"`")
		return s.undef(boolT)
	}
	return s.arith(x.Op, l, r, x)
}

// ifExpr lowers `if` and `if let`, joining the branch values in a block
// parameter when the `if` has a value.
func (s *fn) ifExpr(x *ast.IfExpr) *Value {
	t := s.typeOf(x)
	hasVal := t.Kind != KUnit && !s.isNever(x)
	join := s.newBlock()
	var res *Value
	if hasVal {
		res = join.AddParam(t)
	}
	arm := func(v *Value, at ast.Node) {
		if hasVal {
			s.jump(join, s.coerce(v, t, at))
		} else {
			s.jump(join)
		}
	}
	var elseB *Block
	if x.Cond != nil {
		c := s.valueAs(x.Cond, s.b.mod.Builtin(KBool))
		var yes *Block
		yes, elseB = s.branch(c)
		s.cur = yes
	} else {
		v := s.value(x.Value)
		elseB = s.newBlock()
		// `if let e = opt` is shorthand for `if let Some(e) = opt`
		if bp, ok := x.Pattern.(*ast.BindPat); ok && v.Type.Kind == KEnum {
			some := v.Type.VariantIndex("Some")
			if vo := s.b.info.VarOf(bp.Name); vo != nil && some >= 0 && s.b.typ(vo.Type, bp) != v.Type {
				m := s.b.mod
				s.test(s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, v), s.cint(int64(some))), elseB)
				v = s.add(OpPayload, v.Type.Variants[some].Fields[0].Type, PayloadRef{some, 0}, v)
			}
		}
		s.pattern(v, x.Pattern, elseB)
		s.seal(elseB)
	}
	arm(s.block(x.Then), x.Then)
	s.cur = elseB
	if x.Else != nil {
		arm(s.expr(x.Else), x.Else)
	} else {
		arm(s.unit(), x)
	}
	s.seal(join)
	s.cur = join
	if res != nil {
		return res
	}
	if s.isNever(x) {
		return s.undef(t)
	}
	return s.unit()
}

// match lowers a match as a chain of pattern tests, one arm after the
// other. A later backend may turn tag tests into a switch.
func (s *fn) match(x *ast.MatchExpr) *Value {
	t := s.typeOf(x)
	hasVal := t.Kind != KUnit && !s.isNever(x)
	v := s.value(x.Subject)
	join := s.newBlock()
	var res *Value
	if hasVal {
		res = join.AddParam(t)
	}
	for _, a := range x.Arms {
		next := s.newBlock()
		s.pattern(v, a.Pattern, next)
		if a.Guard != nil {
			s.test(s.valueAs(a.Guard, s.b.mod.Builtin(KBool)), next)
		}
		var av *Value
		if es, ok := a.Body.(*ast.ExprStmt); ok {
			av = s.expr(es.X)
		} else {
			s.stmt(a.Body)
			av = s.unit()
		}
		if hasVal {
			s.jump(join, s.coerce(av, t, a))
		} else {
			s.jump(join)
		}
		s.seal(next)
		s.cur = next
	}
	// the checker proved the match exhaustive
	s.add(OpUnreachable, nil, nil)
	s.seal(join)
	s.cur = join
	if res != nil {
		return res
	}
	if s.isNever(x) {
		return s.undef(t)
	}
	return s.unit()
}

// pattern tests v against p in the current block, branching to fail
// when it does not match; it continues in the block where it matched,
// with the pattern's bindings written.
func (s *fn) pattern(v *Value, p ast.Pattern, fail *Block) {
	b := s.b
	m := b.mod
	boolT := m.Builtin(KBool)
	intT := m.Builtin(KInt)
	switch x := p.(type) {
	case *ast.WildcardPat:
	case *ast.BindPat:
		if vo := b.info.VarOf(x.Name); vo != nil {
			s.writeVar(vo, v)
		}
	case *ast.LitPat:
		lit := s.valueAs(x.Value, v.Type)
		var c *Value
		if v.Type.Kind == KStr {
			c = s.call("str.eq", boolT, "", v, lit)
		} else {
			c = s.add(OpEq, boolT, nil, v, lit)
		}
		s.test(c, fail)
	case *ast.RangePat:
		lo := s.valueAs(x.Lo, v.Type)
		s.test(s.add(OpGe, boolT, nil, v, lo), fail)
		hi := s.valueAs(x.Hi, v.Type)
		if x.Inclusive {
			s.test(s.add(OpLe, boolT, nil, v, hi), fail)
		} else {
			s.test(s.add(OpLt, boolT, nil, v, hi), fail)
		}
	case *ast.TuplePat:
		if v.Type.Kind != KTuple || len(x.Elems) != len(v.Type.Elems) {
			b.unsupported(x, "this tuple pattern")
			return
		}
		for i, el := range x.Elems {
			s.pattern(s.add(OpField, v.Type.Elems[i], i, v), el, fail)
		}
	case *ast.VariantPat:
		vo := b.info.PatVariant(x)
		tag := -1
		if vo != nil && v.Type.Kind == KEnum {
			tag = v.Type.VariantIndex(vo.Name)
		}
		if tag < 0 {
			b.unsupported(x, "this variant pattern")
			return
		}
		s.test(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, v), s.cint(int64(tag))), fail)
		fs := v.Type.Variants[tag].Fields
		for i, pf := range x.Fields {
			fi := i
			if pf.Name != nil {
				fi = -1
				for j, vf := range vo.Fields {
					if vf.Name == pf.Name.Name {
						fi = j
					}
				}
			}
			if fi < 0 || fi >= len(fs) {
				b.unsupported(pf, "this variant field")
				continue
			}
			sub := pf.Pattern
			if sub == nil && pf.Name != nil {
				continue
			}
			s.pattern(s.add(OpPayload, fs[fi].Type, PayloadRef{tag, fi}, v), sub, fail)
		}
	case *ast.StructPat:
		if v.Type.Kind != KStruct {
			b.unsupported(x, "this struct pattern")
			return
		}
		for _, pf := range x.Fields {
			i := -1
			if pf.Name != nil {
				i = v.Type.FieldIndex(pf.Name.Name)
			}
			if i < 0 || pf.Pattern == nil {
				b.unsupported(pf, "this struct pattern field")
				continue
			}
			s.pattern(s.add(OpField, v.Type.Fields[i].Type, i, v), pf.Pattern, fail)
		}
	case *ast.OrPat:
		// each alternative falls through to the next; bindings meet in
		// block parameters of ok
		ok := s.newBlock()
		for _, alt := range x.Alts {
			next := s.newBlock()
			s.pattern(v, alt, next)
			s.jump(ok)
			s.seal(next)
			s.cur = next
		}
		s.jump(fail)
		s.seal(ok)
		s.cur = ok
	default:
		b.unsupported(p, "this pattern")
	}
}
