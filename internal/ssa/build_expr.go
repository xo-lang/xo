package ssa

import (
	"fmt"
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
			f, _ := strconv.ParseFloat(strings.ReplaceAll(x.Value, "_", ""), 64)
			if t.F32() {
				f = float64(float32(f))
			}
			return s.add(OpConst, t, f)
		}
		if t.Kind != KInt {
			t = m.Builtin(KInt)
		}
		return s.intLit(x, t)
	case *ast.RuneLit:
		// a Rune, or the code point where an integer is expected
		// (decision 0079)
		if t := s.natOf(x); t.Kind == KInt {
			return s.add(OpConst, t, int64(x.Value))
		}
	case *ast.BytesLit:
		return s.add(OpConst, m.Builtin(KStr), x.Value)
	case *ast.FloatLit:
		f, err := strconv.ParseFloat(strings.ReplaceAll(x.Raw, "_", ""), 64)
		if err != nil {
			b.unsupported(x, "this Float literal")
		}
		if t := s.natOf(x); t.F32() {
			return s.add(OpConst, t, float64(float32(f)))
		}
		return s.add(OpConst, m.Builtin(KFloat), f)
	case *ast.StringLit:
		return s.stringLit(x)
	case *ast.DurationLit:
		return s.durationLit(x)
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
			// the success value of a failing function's tail (the
			// context type is the Result)
			t = s.natOf(x)
		}
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
		s.checkInvariant(v, s.natCheck(x), x)
		return v
	case *ast.UnaryExpr:
		v := s.expr(x.X)
		switch x.Op {
		case token.Minus:
			if v.Type.Sized() {
				return s.sizedNeg(v, x)
			}
			r := s.add(OpNeg, v.Type, nil, v)
			if v.Type.Kind == KInt {
				r.Pos = b.pos(x)
			}
			return r
		case token.Bang, token.Tilde:
			if v.Type.Sized() {
				return s.sizedNot(v)
			}
			return s.add(OpNot, v.Type, nil, v)
		}
	case *ast.BinaryExpr:
		return s.binary(x)
	case *ast.SelectorExpr:
		return s.selector(x)
	case *ast.IndexExpr:
		if r, ok := x.Index.(*ast.RangeExpr); ok {
			return s.slice(x, r)
		}
		l := s.expr(x.X)
		if l.Type.Kind == KStr && isPreludeNamed(s.natCheck(x.X), "Bytes") {
			i := s.valueAs(x.Index, m.Builtin(KInt))
			return s.call("bytes.index", m.IntType("UInt8"), b.pos(x), l, i)
		}
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
	case *ast.MapLit:
		// `{}` whose value is discarded (a match arm `_ => {}`) parses as
		// an empty Map literal of unresolved type: it is an empty block
		// (gotchas/fmt-empty-block-is-map.md)
		if n, ok := check.Resolve(s.natCheck(x)).(*check.Named); ok && len(x.Entries) == 0 && len(n.Args) == 2 {
			_, k := check.Resolve(n.Args[0]).(*check.Var)
			_, v := check.Resolve(n.Args[1]).(*check.Var)
			if k && v {
				return s.unit()
			}
		}
		var ks, vs []ast.Expr
		for _, en := range x.Entries {
			ks = append(ks, en.Key)
			vs = append(vs, en.Value)
		}
		if vs == nil {
			vs = []ast.Expr{}
		}
		return s.mapLit(s.typeOf(x), ks, vs, x)
	case *ast.SetLit:
		return s.mapLit(s.typeOf(x), x.Elems, nil, x)
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
	case *ast.ScopeExpr:
		return s.scopeExpr(x)
	case *ast.SelectExpr:
		return s.selectExpr(x)
	}
	b.unsupported(e, unsupportedExpr(e))
	return s.undef(s.natOf(e))
}

// unsupportedExpr names an expression the builder does not lower.
func unsupportedExpr(e ast.Expr) string {
	switch e.(type) {
	case *ast.MapLit:
		return "Map"
	case *ast.SetLit:
		return "Set"
	case *ast.ScopeExpr:
		return "`scope` and tasks"
	case *ast.SelectExpr:
		return "`select`"
	case *ast.DurationLit:
		return "Duration"
	case *ast.RuneLit:
		return "Rune"
	case *ast.BytesLit:
		return "Bytes"
	case *ast.HoleExpr:
		return "holes (`???`)"
	case *ast.RangeExpr:
		return "a range outside `for`"
	}
	return "this expression (" + strings.TrimPrefix(fmt.Sprintf("%T", e), "*ast.") + ")"
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
		return s.constValue(c, s.natOf(x))
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
		return s.constValue(c, s.natOf(x))
	}
	if x.Sel.Name == "await" {
		if n, ok := check.Resolve(s.natCheck(x.X)).(*check.Named); ok && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Task" {
			return s.await(x)
		}
	}
	v := s.expr(x.X)
	if v.Type.Kind == KCap {
		t := s.natOf(x)
		if t.Kind == KHandle {
			// os.tasks (core 7.1): the native backends have no program
			// wide group yet
			b.unsupported(x, "`"+x.Sel.Name+"` of a capability")
			return s.undef(t)
		}
		return s.add(OpCap, t, x.Sel.Name, v)
	}
	if i, ft := s.fieldIndex(v.Type, x.Sel.Name); i >= 0 {
		return s.unbox(s.add(OpField, ft, i, v))
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
		ct := s.natCheck(p.Expr)
		v := s.expr(p.Expr)
		if isPreludeNamed(ct, "Duration", "Time", "Bytes", "UInt64", "Rune") || !isBuiltinKind(v.Type) {
			// Display text through build_show.go
			if acc == nil {
				acc = s.add(OpConst, st, "")
			}
			acc = s.appendDisplay(acc, v, ct, p.Expr)
			continue
		}
		switch v.Type.Kind {
		case KStr:
		case KInt, KFloat:
			if acc != nil {
				// append the number's text without a temporary string
				acc = s.call("str.concat_"+lowerKind(v.Type), st, "", acc, v)
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
		if named != nil {
			// a hidden field of a std type's implementation (decision
			// 0074): its default
			if it := b.implType(named.Obj); it != nil {
				for _, f := range it.Fields {
					if f.Name == t.Fields[i].Name && f.Default != nil {
						vals[i] = s.valueAs(f.Default, t.Fields[i].Type)
						break
					}
				}
				if vals[i] != nil {
					continue
				}
			}
		}
		b.unsupported(x, "a struct literal without the field `"+t.Fields[i].Name+"`")
		vals[i] = s.undef(t.Fields[i].Type)
	}
	v := s.add(OpStruct, t, nil, vals...)
	if named != nil {
		s.checkInvariant(v, named, x)
	}
	return v
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
	if l.Type.Kind == KList && op == token.Plus && r.Type == l.Type {
		// `xs + ys`: xs, then each element of ys pushed (in place when
		// xs is not shared)
		return s.call(s.b.listConcat(l.Type), l.Type, "", l, r)
	}
	if l.Type.Sized() {
		return s.sizedArith(op, l, r, at)
	}
	o, ok := arithOps[op]
	if !ok || (l.Type.Kind != KInt && l.Type.Kind != KFloat && l.Type.Kind != KBool) {
		s.b.unsupported(at, "this operator")
		return s.undef(l.Type)
	}
	v := s.add(o, l.Type, nil, l, r)
	if l.Type.F32() {
		switch o {
		case OpAdd, OpSub, OpMul, OpDiv:
			// the double result rounded to float32
			return s.rt("f32_round", l.Type, v)
		}
	}
	if l.Type.Kind == KInt {
		switch o {
		case OpAdd, OpSub, OpMul, OpDiv, OpRem, OpShl, OpShr:
			v.Pos = s.b.pos(at)
		}
	}
	return v
}

func (s *fn) binary(x *ast.BinaryExpr) *Value {
	if s.isExpectCompare(x) {
		return s.expectCompare(x)
	}
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
	if x.Op == token.Eq || x.Op == token.NotEq {
		if lr, rr := s.toLowerRecv(x.X), s.toLowerRecv(x.Y); lr != nil && rr != nil {
			// `a.to_lower() == b.to_lower()` compares without making the
			// lowered strings (xo_str_eq_lower; the Go backend's
			// rt.Str_eq_lower)
			strT := m.Builtin(KStr)
			eq := s.rt("str_eq_lower", boolT, s.valueAs(lr, strT), s.valueAs(rr, strT))
			if x.Op == token.NotEq {
				return s.add(OpNot, boolT, nil, eq)
			}
			return eq
		}
	}
	l := s.expr(x.X)
	var r *Value
	if x.Op == token.Shl || x.Op == token.Shr {
		r = s.shiftCount(x.Y, l.Type)
	} else {
		r = s.valueAs(x.Y, l.Type)
	}
	if _, ok := cmpOps[x.Op]; ok {
		return s.compareValues(x.Op, l, r, x)
	}
	return s.arith(x.Op, l, r, x)
}

// compareValues lowers the comparison `l op r` of two values of one type.
func (s *fn) compareValues(op token.Kind, l, r *Value, x ast.Node) *Value {
	b := s.b
	m := b.mod
	boolT := m.Builtin(KBool)
	o := cmpOps[op]
	switch l.Type.Kind {
	case KInt, KFloat, KBool:
		return s.compare(o, l, r)
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
		if (o == OpEq || o == OpNe) && !comparable(l.Type, map[*Type]bool{}) {
			// a function value or a runtime handle (a request value,
			// a channel) outside an interface value: the backends'
			// equality helpers do not compare them
			b.unsupported(x, "`==` on values of type `"+l.Type.Name+"`, which hold functions or runtime handles")
			return s.undef(boolT)
		}
		if holdsColl(l.Type, map[*Type]bool{}) && (o == OpEq || o == OpNe) {
			// a Map or Set is equal to another with the same entries
			// in any order (decision 0074)
			eq := s.call(b.eqColl(l.Type), boolT, "", l, r)
			if o == OpNe {
				return s.add(OpNot, boolT, nil, eq)
			}
			return eq
		}
		// structural equality; backends generate a helper per type
		if o == OpEq || o == OpNe {
			return s.add(o, boolT, nil, l, r)
		}
		// order (`derive Ord`): std/rt's Cmp
		if name, ok := b.cmpFunc(l.Type); ok {
			return s.add(o, boolT, nil, s.call(name, m.Builtin(KInt), "", l, r), s.cint(0))
		}
	}
	b.unsupported(x, "comparing values of type `"+l.Type.Name+"`")
	return s.undef(boolT)
}

// toLowerRecv returns the receiver of a call `s.to_lower()` of the
// prelude's Str, or nil.
func (s *fn) toLowerRecv(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	c, ok := e.(*ast.CallExpr)
	if !ok || len(c.Args) != 0 {
		return nil
	}
	ci := s.b.info.CallOf(c)
	if ci == nil || ci.Recv == nil || ci.Fn == nil || ci.Fn.Recv == nil || ci.Fn.Name != "to_lower" || ci.Fn.Recv.Name != "Str" || ci.Fn.Recv.Module == nil || !ci.Fn.Recv.Module.IsPrelude {
		return nil
	}
	return ci.Recv
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
			if vo := s.b.info.VarOf(bp.Name); vo != nil && some >= 0 && s.b.typ(s.apply(vo.Type), bp) != v.Type {
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
		s.test(s.compare(OpGe, v, lo), fail)
		hi := s.valueAs(x.Hi, v.Type)
		if x.Inclusive {
			s.test(s.compare(OpLe, v, hi), fail)
		} else {
			s.test(s.compare(OpLt, v, hi), fail)
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
		if nt := b.info.PatNewtype(x); nt != nil && b.info.PatVariant(x) == nil {
			// `Email(p)` matches p against the underlying value
			if v.Type.Kind != KStruct || len(v.Type.Fields) != 1 {
				b.unsupported(x, "this newtype pattern")
				return
			}
			if len(x.Fields) == 1 && x.Fields[0].Pattern != nil {
				s.pattern(s.unbox(s.add(OpField, v.Type.Fields[0].Type, 0, v)), x.Fields[0].Pattern, fail)
			}
			return
		}
		vo := b.info.PatVariant(x)
		if vo != nil && v.Type.Kind != KEnum && isNeverResult(s.apply(b.info.PatType(x))) {
			// a Result[T, Never] is its T (decision 0074): Ok(p)
			// matches p against it, Err never matches
			if vo.Name == "Err" {
				s.jump(fail)
				s.dead()
				return
			}
			if vo.Name == "Ok" && len(x.Fields) == 1 {
				if x.Fields[0].Pattern != nil {
					s.pattern(v, x.Fields[0].Pattern, fail)
				}
				return
			}
		}
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
				// `Node(left, ..)`: binds the field's name
				if vo := b.info.VarOf(pf.Name); vo != nil {
					s.writeVar(vo, s.unbox(s.add(OpPayload, fs[fi].Type, PayloadRef{tag, fi}, v)))
				}
				continue
			}
			s.pattern(s.unbox(s.add(OpPayload, fs[fi].Type, PayloadRef{tag, fi}, v)), sub, fail)
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
			} else if bp, ok := pf.Pattern.(*ast.BindPat); ok {
				// `Bag{items}`: the field of the binding's name
				i = v.Type.FieldIndex(bp.Name.Name)
			}
			if i >= 0 && pf.Pattern == nil {
				// `Bag{items}`: binds the field's name
				if vo := b.info.VarOf(pf.Name); vo != nil {
					s.writeVar(vo, s.unbox(s.add(OpField, v.Type.Fields[i].Type, i, v)))
				}
				continue
			}
			if i < 0 {
				b.unsupported(pf, "this struct pattern field")
				continue
			}
			s.pattern(s.unbox(s.add(OpField, v.Type.Fields[i].Type, i, v)), pf.Pattern, fail)
		}
	case *ast.OrPat:
		// each alternative falls through to the next; bindings meet in
		// block parameters of ok
		// The checker gives each alternative bindings of its own; the arm
		// reads the first alternative's, so the others' are copied into
		// them by name
		ok := s.newBlock()
		first := s.patBindings(x.Alts[0])
		for i, alt := range x.Alts {
			next := s.newBlock()
			s.pattern(v, alt, next)
			if i > 0 {
				for name, vo := range s.patBindings(alt) {
					if fv := first[name]; fv != nil && fv != vo {
						s.writeVar(fv, s.read(vo))
					}
				}
			}
			s.jump(ok)
			s.seal(next)
			s.cur = next
		}
		s.jump(fail)
		s.seal(ok)
		s.cur = ok
	case *ast.ListPat:
		// the length, then each element against its pattern: before the
		// `..` from the front, after it from the back
		if v.Type.Kind != KList {
			b.unsupported(p, "this list pattern")
			return
		}
		et := v.Type.Elems[0]
		k := len(x.Elems)
		n := s.call("list.len", intT, "", v)
		if x.Rest < 0 {
			s.test(s.add(OpEq, boolT, nil, n, s.cint(int64(k))), fail)
		} else {
			s.test(s.add(OpGe, boolT, nil, n, s.cint(int64(k))), fail)
		}
		for i, e := range x.Elems {
			var idx *Value
			if x.Rest < 0 || i < x.Rest {
				idx = s.cint(int64(i))
			} else {
				idx = s.add(OpSub, intT, nil, n, s.cint(int64(k-i)))
			}
			s.pattern(s.call("list.index", et, "", v, idx), e, fail)
		}
	default:
		b.unsupported(p, "this pattern")
	}
}

// patBindings returns the bindings of the pattern p by name.
func (s *fn) patBindings(p ast.Pattern) map[string]*check.VarObj {
	out := map[string]*check.VarObj{}
	var walk func(p ast.Pattern)
	field := func(f *ast.PatField) {
		if f.Pattern != nil {
			walk(f.Pattern)
		} else if f.Name != nil {
			if vo := s.b.info.VarOf(f.Name); vo != nil {
				out[vo.Name] = vo
			}
		}
	}
	walk = func(p ast.Pattern) {
		switch x := p.(type) {
		case *ast.BindPat:
			if vo := s.b.info.VarOf(x.Name); vo != nil {
				out[vo.Name] = vo
			}
		case *ast.TuplePat:
			for _, e := range x.Elems {
				walk(e)
			}
		case *ast.ListPat:
			for _, e := range x.Elems {
				walk(e)
			}
		case *ast.OrPat:
			if len(x.Alts) > 0 {
				walk(x.Alts[0])
			}
		case *ast.VariantPat:
			for _, f := range x.Fields {
				field(f)
			}
		case *ast.StructPat:
			for _, f := range x.Fields {
				field(f)
			}
		}
	}
	walk(p)
	return out
}

// comparable reports whether `==` on values of t lowers: no function
// value or runtime handle in them, except inside interface values (which
// compare through their xo.eq function).
func comparable(t *Type, seen map[*Type]bool) bool {
	if seen[t] || isIface(t) {
		return true
	}
	seen[t] = true
	switch t.Kind {
	case KFunc, KHandle:
		return false
	}
	for _, f := range t.Fields {
		if !comparable(f.Type, seen) {
			return false
		}
	}
	for _, v := range t.Variants {
		for _, f := range v.Fields {
			if !comparable(f.Type, seen) {
				return false
			}
		}
	}
	for _, e := range t.Elems {
		if !comparable(e, seen) {
			return false
		}
	}
	return true
}
