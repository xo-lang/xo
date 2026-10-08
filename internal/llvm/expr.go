package llvm

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// ---------------------------------------------------------------------------
// Values, ownership, conversions

// val evaluates e converted to its type in context.
func (f *fn) val(e ast.Expr) val {
	v := f.expr(e)
	want := f.g.info.TypeOf(e)
	if want == nil {
		return v
	}
	return f.coerce(v, f.nat(e), f.t(want), e)
}

// valAs evaluates e converted to type want.
func (f *fn) valAs(e ast.Expr, want check.Type) val {
	return f.coerce(f.expr(e), f.nat(e), want, e)
}

// own is val for a value that is stored (bound, put in a field or list,
// captured, returned): a copy read out of a place retains its lists.
func (f *fn) own(e ast.Expr) val {
	v := f.expr(e)
	if placeLike(e) {
		f.retain(v)
	}
	want := f.g.info.TypeOf(e)
	if want == nil {
		return v
	}
	return f.coerce(v, f.nat(e), f.t(want), e)
}

func (f *fn) ownAs(e ast.Expr, want check.Type) val {
	v := f.expr(e)
	if placeLike(e) {
		f.retain(v)
	}
	return f.coerce(v, f.nat(e), want, e)
}

// placeLike reports expressions whose value may still be held elsewhere
// (a binding, a field, an element), as opposed to a fresh temporary.
func placeLike(e ast.Expr) bool {
	switch x := unparen(e).(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr, *ast.TryExpr:
		return true
	case *ast.BinaryExpr:
		return x.Op == token.Coalesce
	}
	return false
}

// retain marks the list buffers a value holds inline as shared.
func (f *fn) retain(v val) {
	if !v.t.rc || f.term {
		return
	}
	f.clobber()
	if v.t.k == kList {
		s := f.def("getelementptr %%xo.list, ptr %s, i32 0, i32 2", v.v)
		f.emit("store i64 1, ptr %s", s)
		return
	}
	p := f.spill(v)
	f.emit("call void %s(ptr %s)", f.g.retainFn(v.t, nil), p)
}

// coerce converts v from type `from` to type `to` (core 2.10): Some
// wrapping, Never, and Result conversion.
func (f *fn) coerce(v val, from, to check.Type, at ast.Node) val {
	if from == nil || to == nil {
		return v
	}
	g := f.g
	tt := g.lower(to, at)
	if check.IsNever(from) && !check.IsNever(to) {
		return val{tt, "poison"}
	}
	if v.t.key == tt.key {
		return v
	}
	if f.term {
		return val{tt, "poison"}
	}
	if el := g.optionElem(to); el != nil && g.optionElem(from) == nil {
		inner := f.coerce(v, from, el, at)
		return f.variant(tt, 1, []val{inner}, at)
	}
	if ft, fe, ok := g.resultParts(from); ok {
		if tt2, te2, ok := g.resultParts(to); ok {
			return f.convertResult(v, ft, fe, tt2, te2, tt, at)
		}
	}
	if v.t.ll == tt.ll && v.t.k == tt.k && v.t.k != kFn {
		return val{tt, v.v}
	}
	if v.t.k == kFn && tt.k == kFn {
		g.unsupported(at, "converting a function value to a function type that can fail")
		return val{tt, v.v}
	}
	g.unsupported(at, fmt.Sprintf("the conversion from `%s` to `%s`", from, to))
	return val{tt, "poison"}
}

// convertResult converts Result[ft, fe] to Result[tt2, te2].
func (f *fn) convertResult(v val, ft, fe, tt2, te2 check.Type, rt *ty, at ast.Node) val {
	p := f.spill(v)
	tag := f.def("extractvalue %s %s, 0", v.t.ll, v.v)
	slot := f.alloca(rt)
	okL, errL, end := f.label("rok"), f.label("rerr"), f.label("rend")
	isErr := f.def("icmp eq i64 %s, 1", tag)
	f.condBr(isErr, errL, okL)
	f.startBlock(okL)
	ov := f.payload(p, v.t, 0, 0, at)
	nv := f.variant(rt, 0, []val{f.coerce(ov, ft, tt2, at)}, at)
	f.emit("store %s %s, ptr %s", rt.ll, nv.v, slot)
	f.br(end)
	f.startBlock(errL)
	ev := f.payload(p, v.t, 1, 0, at)
	nv = f.variant(rt, 1, []val{f.coerceErr(ev, fe, te2, at)}, at)
	f.emit("store %s %s, ptr %s", rt.ll, nv.v, slot)
	f.br(end)
	f.startBlock(end)
	return f.load(rt, slot)
}

// coerceErr converts an error at a `?` site (core 2.6): the same type, or
// wrapping in a `from` variant.
func (f *fn) coerceErr(v val, from, to check.Type, at ast.Node) val {
	g := f.g
	if to == nil || from == nil {
		return v
	}
	tt := g.lower(to, at)
	if tt.key == v.t.key {
		return v
	}
	if check.IsNever(from) {
		return val{tt, "poison"}
	}
	if g.isPrelude(to, "Error") || g.isPrelude(to, "Display") {
		g.unsupported(at, "the open error type `Error` (use an enum error type)")
		return val{tt, "poison"}
	}
	if vo := g.info.FromVariant(to, from); vo != nil {
		idx, _ := tt.variant(vo.Name)
		return f.variant(tt, idx, []val{v}, at)
	}
	return f.coerce(v, from, to, at)
}

// ---------------------------------------------------------------------------
// Enums

// variant builds a value of enum t with variant idx and payload fields.
func (f *fn) variant(t *ty, idx int, fields []val, at ast.Node) val {
	if t.k != kEnum {
		f.g.unsupported(at, "constructing `"+t.name()+"`")
		return val{t, "poison"}
	}
	if f.term {
		return val{t, "poison"}
	}
	if t.words == 0 {
		return val{t, f.def("insertvalue %s zeroinitializer, i64 %d, 0", t.ll, idx)}
	}
	a := f.alloca(t)
	f.emit("store %s zeroinitializer, ptr %s", t.ll, a)
	f.emit("store i64 %d, ptr %s", idx, a)
	if len(fields) > 0 {
		v := t.vars[idx]
		pay := f.def("getelementptr %s, ptr %s, i32 0, i32 1", t.ll, a)
		for i, fv := range fields {
			fl := v.fields[i]
			fp := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", v.ll, pay, i)
			if fl.boxed {
				ft := f.g.fieldTy(fl, at)
				b := f.def("call ptr @xo_alloc(i64 %d)", max(ft.size, 1))
				f.emit("store %s %s, ptr %s", ft.ll, fv.v, b)
				f.emit("store ptr %s, ptr %s", b, fp)
			} else {
				f.emit("store %s %s, ptr %s", fv.t.ll, fv.v, fp)
			}
		}
	}
	return f.load(t, a)
}

// payloadPtr returns the address of field j of variant i of the enum at
// p (following a box).
func (f *fn) payloadPtr(p string, t *ty, i, j int, at ast.Node) (string, *ty) {
	v := t.vars[i]
	fl := v.fields[j]
	pay := f.def("getelementptr %s, ptr %s, i32 0, i32 1", t.ll, p)
	fp := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", v.ll, pay, j)
	ft := f.g.fieldTy(fl, at)
	if fl.boxed {
		fp = f.def("load ptr, ptr %s", fp)
	}
	return fp, ft
}

// payload loads field j of variant i of the enum at p.
func (f *fn) payload(p string, t *ty, i, j int, at ast.Node) val {
	fp, ft := f.payloadPtr(p, t, i, j, at)
	return f.load(ft, fp)
}

// tagOf returns the tag of an enum value.
func (f *fn) tagOf(v val) string {
	return f.def("extractvalue %s %s, 0", v.t.ll, v.v)
}

// ---------------------------------------------------------------------------
// Expressions

// expr evaluates e in its natural type.
func (f *fn) expr(e ast.Expr) val {
	g := f.g
	switch x := e.(type) {
	case *ast.ParenExpr:
		return f.expr(x.X)
	case *ast.IntLit:
		t := f.ty(f.nat(x), x)
		if t.k == kFloat {
			return val{t, floatLit(x.Value)}
		}
		if t.k != kInt {
			g.unsupported(x, "integer literals of type `"+t.name()+"`")
		}
		if _, err := strconv.ParseInt(x.Value, 10, 64); err != nil {
			g.unsupported(x, "this integer literal")
		}
		return val{t, x.Value}
	case *ast.FloatLit:
		t := f.ty(f.nat(x), x)
		if t.k != kFloat {
			g.unsupported(x, "float literals of type `"+t.name()+"`")
		}
		return val{t, floatLit(strings.ReplaceAll(x.Raw, "_", ""))}
	case *ast.StringLit:
		return f.stringLit(x)
	case *ast.UnitExpr:
		return f.unitVal()
	case *ast.Ident:
		return f.ident(x)
	case *ast.TupleExpr:
		t := f.ty(f.nat(x), x)
		tt := check.Resolve(t.t).(*check.Tuple)
		cur := "undef"
		for i, el := range x.Elems {
			v := f.ownAs(el, tt.Elems[i])
			cur = f.def("insertvalue %s %s, %s %s, %d", t.ll, cur, v.t.ll, v.v, i)
		}
		return val{t, cur}
	case *ast.ListLit:
		return f.listLit(x)
	case *ast.StructLit:
		return f.structLit(x)
	case *ast.WithExpr:
		return f.withExpr(x)
	case *ast.UnaryExpr:
		return f.unary(x)
	case *ast.BinaryExpr:
		return f.binary(x)
	case *ast.IsExpr:
		v := f.val(x.X)
		p := f.spill(v)
		bt := g.lower(g.preludeType("Bool"), x)
		slot := f.alloca(bt)
		f.emit("store i1 false, ptr %s", slot)
		fail := f.label("isno")
		f.pattern(x.Pattern, p, v.t, fail, nil)
		f.emit("store i1 true, ptr %s", slot)
		f.br(fail)
		f.startBlock(fail)
		return f.load(bt, slot)
	case *ast.SelectorExpr:
		return f.selector(x)
	case *ast.CallExpr:
		return f.call(x)
	case *ast.IndexExpr:
		return f.index(x)
	case *ast.TryExpr:
		return f.try(x)
	case *ast.FnLit:
		return f.closure(x)
	case *ast.IfExpr, *ast.MatchExpr, *ast.Block:
		return f.valueInto(e)
	case *ast.WhileExpr:
		f.loopWhile(nil, nil, x.Body)
		return val{f.ty(f.nat(x), x), "poison"}
	case *ast.RangeExpr:
		g.unsupported(x, "a range outside `for`")
	case *ast.DurationLit:
		g.unsupported(x, "Duration")
	case *ast.RuneLit:
		g.unsupported(x, "Rune")
	case *ast.BytesLit:
		g.unsupported(x, "Bytes")
	case *ast.MapLit:
		g.unsupported(x, "Map")
	case *ast.SetLit:
		g.unsupported(x, "Set")
	case *ast.HoleExpr:
		g.unsupported(x, "holes (`???`)")
	case *ast.ScopeExpr:
		g.unsupported(x, "`scope` and tasks")
	case *ast.SelectExpr:
		g.unsupported(x, "`select`")
	case *ast.TypeArgsExpr:
		g.unsupported(x, "explicit type arguments")
	default:
		g.unsupported(e, fmt.Sprintf("the expression %T", e))
	}
	t := f.typeOf(e)
	return val{f.g.lower(t, e), "poison"}
}

func floatLit(s string) string {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "0.0"
	}
	return fmt.Sprintf("0x%016X", math.Float64bits(v))
}

func (f *fn) ident(x *ast.Ident) val {
	g := f.g
	if v := g.info.VarOf(x); v != nil {
		l, ok := f.vars[v]
		if !ok {
			g.unsupported(x, "the binding `"+x.Name+"` here")
			return val{f.ty(v.Type, x), "poison"}
		}
		return f.load(l.t, l.ptr)
	}
	if vo := g.info.Ctor(x); vo != nil {
		return f.ctor(vo, f.nat(x), nil, x)
	}
	switch o := g.info.Ref(x).(type) {
	case *check.FuncObj:
		return f.funcValue(o, f.nat(x), x)
	case *check.ConstObj:
		return f.constVal(o, x)
	}
	bt := g.lower(g.preludeType("Bool"), x)
	switch x.Name {
	case "true":
		return val{bt, "true"}
	case "false":
		return val{bt, "false"}
	case "None":
		t := f.ty(f.nat(x), x)
		return f.variant(t, 0, nil, x)
	}
	g.unsupported(x, "the name `"+x.Name+"`")
	return val{f.ty(f.typeOf(x), x), "poison"}
}

// constVal evaluates a constant's initializer in place.
func (f *fn) constVal(o *check.ConstObj, at ast.Node) val {
	if o.Module.IsStd || o.Decl == nil {
		f.g.unsupported(at, "the std constant `"+o.Name+"`")
		return val{f.g.lower(o.Type, at), "poison"}
	}
	return f.valAs(o.Decl.Value, check.Deep(o.Type))
}

func (f *fn) selector(x *ast.SelectorExpr) val {
	g := f.g
	if vo := g.info.Ctor(x); vo != nil {
		return f.ctor(vo, f.nat(x), nil, x)
	}
	if ref := g.info.Ref(x); ref != nil {
		switch o := ref.(type) {
		case *check.ConstObj:
			return f.constVal(o, x)
		case *check.FuncObj:
			if o.Recv == nil {
				return f.funcValue(o, f.nat(x), x)
			}
			g.unsupported(x, "methods used as values")
			return val{f.ty(f.nat(x), x), "poison"}
		}
	}
	base := f.val(x.X)
	switch base.t.k {
	case kUnit:
		// a capability: `os.stdio`
		return val{f.ty(f.nat(x), x), "zeroinitializer"}
	case kStruct, kTuple:
		i := base.t.fieldIndex(x.Sel.Name)
		if i < 0 {
			break
		}
		fl := base.t.fields[i]
		v := f.def("extractvalue %s %s, %d", base.t.ll, base.v, i)
		ft := g.fieldTy(fl, x)
		if fl.boxed {
			return f.load(ft, v)
		}
		return val{ft, v}
	}
	g.unsupported(x, "the selector `."+x.Sel.Name+"` on `"+base.t.name()+"`")
	return val{f.ty(f.nat(x), x), "poison"}
}

func (f *fn) index(x *ast.IndexExpr) val {
	g := f.g
	if _, ok := x.Index.(*ast.RangeExpr); ok {
		g.unsupported(x, "slicing")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	base := f.val(x.X)
	if base.t.k != kList {
		g.unsupported(x, "indexing `"+base.t.name()+"`")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	i := f.val(x.Index)
	f.boundsCheck(base.v, i.v, x)
	p := f.def("getelementptr %s, ptr %s, i64 %s", base.t.elem.ll, f.listData(base.v), i.v)
	return val{base.t.elem, f.def("load %s, ptr %s"+tagElem, base.t.elem.ll, p)}
}

// try is `x?` on a Result (propagate the error) or an Option (return
// None).
func (f *fn) try(x *ast.TryExpr) val {
	g := f.g
	xt := f.nat(x.X)
	v := f.val(x.X)
	if f.term {
		return val{f.ty(f.nat(x), x), "poison"}
	}
	p := f.spill(v)
	tag := f.tagOf(v)
	okL, failL := f.label("tryok"), f.label("tryfail")
	if _, et, ok := g.resultParts(xt); ok {
		isErr := f.def("icmp eq i64 %s, 1", tag)
		f.condBr(isErr, failL, okL)
		f.startBlock(failL)
		if f.err == nil {
			g.unsupported(x, "`?` on a Result in a function that cannot fail")
		} else {
			ev := f.payload(p, v.t, 1, 0, x)
			conv := f.coerceErr(ev, et, f.err, x)
			f.emitRet(f.variant(f.ret, 1, []val{conv}, x))
		}
		f.startBlock(okL)
		return f.payload(p, v.t, 0, 0, x)
	}
	if g.optionElem(xt) == nil {
		g.unsupported(x, "`?` on `"+fmt.Sprint(xt)+"`")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	isNone := f.def("icmp eq i64 %s, 0", tag)
	f.condBr(isNone, failL, okL)
	f.startBlock(failL)
	if g.optionElem(f.res) == nil {
		g.unsupported(x, "`?` on an Option outside a function returning an Option")
	} else {
		none := f.variant(f.g.lower(f.res, x), 0, nil, x)
		if f.err != nil {
			none = f.variant(f.ret, 0, []val{none}, x)
		}
		f.emitRet(none)
	}
	f.startBlock(okL)
	return f.payload(p, v.t, 1, 0, x)
}

// ---------------------------------------------------------------------------
// Literals

func (f *fn) stringLit(x *ast.StringLit) val {
	g := f.g
	st := g.lower(g.preludeType("Str"), x)
	plain := true
	var text strings.Builder
	for _, p := range x.Parts {
		if p.Expr != nil {
			plain = false
			break
		}
		text.WriteString(p.Text)
	}
	if plain {
		return val{st, g.strConst(text.String())}
	}
	sb := f.def("call ptr @xo_sb_new()")
	for _, p := range x.Parts {
		if p.Expr == nil {
			if p.Text != "" {
				f.emit("call void @xo_sb_str(ptr %s, ptr %s, i64 %d)", sb, g.strPtr(p.Text), len(p.Text))
			}
			continue
		}
		v := f.val(p.Expr)
		f.appendDisplay(sb, v, p.Expr)
	}
	lp := f.alloca(g.lower(g.preludeType("Int"), x))
	ptr := f.def("call ptr @xo_sb_done(ptr %s, ptr %s)", sb, lp)
	n := f.def("load i64, ptr %s", lp)
	return f.mkStr(ptr, n)
}

// appendDisplay appends the Display text of v to a string builder.
func (f *fn) appendDisplay(sb string, v val, at ast.Node) {
	switch v.t.k {
	case kInt:
		f.emit("call void @xo_sb_int(ptr %s, i64 %s)", sb, v.v)
	case kFloat:
		f.emit("call void @xo_sb_float(ptr %s, double %s)", sb, v.v)
	case kBool:
		z := f.def("zext i1 %s to i32", v.v)
		f.emit("call void @xo_sb_bool(ptr %s, i32 %s)", sb, z)
	case kStr:
		p, n := f.strParts(v)
		f.emit("call void @xo_sb_str(ptr %s, ptr %s, i64 %s)", sb, p, n)
	default:
		f.g.unsupported(at, "interpolating a value of type `"+v.t.name()+"` (only Int, Float, Bool, Str)")
	}
}

func (f *fn) mkStr(ptr, n string) val {
	st := f.g.lower(f.g.preludeType("Str"), nil)
	a := f.def("insertvalue %%xo.str undef, ptr %s, 0", ptr)
	b := f.def("insertvalue %%xo.str %s, i64 %s, 1", a, n)
	return val{st, b}
}

func (f *fn) strParts(v val) (string, string) {
	return f.def("extractvalue %%xo.str %s, 0", v.v), f.def("extractvalue %%xo.str %s, 1", v.v)
}

func (f *fn) listLit(x *ast.ListLit) val {
	g := f.g
	t := f.ty(f.nat(x), x)
	if t.k != kList {
		g.unsupported(x, "this list literal")
		return val{t, "poison"}
	}
	if len(x.Elems) == 0 {
		return val{t, "@xo_list_empty"}
	}
	var vals []val
	for _, el := range x.Elems {
		vals = append(vals, f.ownAs(el, t.elem.t))
	}
	l := f.def("call ptr @xo_list_new(i64 %d, i64 %d)", len(vals), t.elem.size)
	data := f.listData(l)
	for i, v := range vals {
		p := f.def("getelementptr %s, ptr %s, i64 %d", t.elem.ll, data, i)
		f.emit("store %s %s, ptr %s"+tagElem, v.t.ll, v.v, p)
	}
	return val{t, l}
}

func (f *fn) structLit(x *ast.StructLit) val {
	g := f.g
	t := f.ty(f.nat(x), x)
	if t.k != kStruct {
		g.unsupported(x, "constructing `"+t.name()+"`")
		return val{t, "poison"}
	}
	given := map[string]ast.Expr{}
	for _, fi := range x.Fields {
		given[fi.Name.Name] = fi.Value
	}
	// evaluate in source order, then assemble
	vals := map[string]val{}
	for _, fi := range x.Fields {
		i := t.fieldIndex(fi.Name.Name)
		if i < 0 {
			continue
		}
		vals[fi.Name.Name] = f.ownAs(fi.Value, t.fields[i].ct)
	}
	cur := "undef"
	for i, fl := range t.fields {
		v, ok := vals[fl.name]
		if !ok {
			var def ast.Expr
			for _, fo := range t.obj.Fields {
				if fo.Name == fl.name {
					def = fo.Default
				}
			}
			if def == nil {
				g.unsupported(x, "the missing field `"+fl.name+"`")
				continue
			}
			v = f.valAs(def, fl.ct)
		}
		cur = f.insertField(t, cur, i, v, x)
	}
	return val{t, cur}
}

// insertField sets field i of struct value cur (boxing when needed).
func (f *fn) insertField(t *ty, cur string, i int, v val, at ast.Node) string {
	fl := t.fields[i]
	if fl.boxed {
		ft := f.g.fieldTy(fl, at)
		b := f.def("call ptr @xo_alloc(i64 %d)", max(ft.size, 1))
		f.emit("store %s %s, ptr %s", ft.ll, v.v, b)
		return f.def("insertvalue %s %s, ptr %s, %d", t.ll, cur, b, i)
	}
	return f.def("insertvalue %s %s, %s %s, %d", t.ll, cur, v.t.ll, v.v, i)
}

func (f *fn) withExpr(x *ast.WithExpr) val {
	base := f.own(x.X)
	t := base.t
	if t.k != kStruct {
		f.g.unsupported(x, "`with` on `"+t.name()+"`")
		return base
	}
	cur := base.v
	for _, fi := range x.Fields {
		i := t.fieldIndex(fi.Name.Name)
		if i < 0 {
			continue
		}
		v := f.ownAs(fi.Value, t.fields[i].ct)
		cur = f.insertField(t, cur, i, v, x)
	}
	return val{t, cur}
}

// ---------------------------------------------------------------------------
// Operators

func (f *fn) unary(x *ast.UnaryExpr) val {
	g := f.g
	switch x.Op {
	case token.Bang:
		v := f.val(x.X)
		return val{v.t, f.def("xor i1 %s, true", v.v)}
	case token.Tilde:
		v := f.val(x.X)
		return val{v.t, f.def("xor i64 %s, -1", v.v)}
	case token.Minus:
		t := f.ty(f.nat(x), x)
		if lit, ok := x.X.(*ast.IntLit); ok && t.k == kInt {
			if _, err := strconv.ParseInt("-"+lit.Value, 10, 64); err == nil {
				return val{t, "-" + lit.Value}
			}
		}
		v := f.val(x.X)
		switch v.t.k {
		case kFloat:
			return val{v.t, f.def("fneg double %s", v.v)}
		case kInt:
			if f.safe[x] {
				return val{v.t, f.def("sub nsw i64 0, %s", v.v)}
			}
			bad := f.def("icmp eq i64 %s, %d", v.v, int64(math.MinInt64))
			fail, ok := f.label("negov"), f.label("neg")
			f.unlikely(bad, fail, ok)
			f.startBlock(fail)
			f.faultCall("@xo_fault_neg(i64 %s, ptr %s)", v.v, g.cstr(g.loc(x)))
			f.startBlock(ok)
			return val{v.t, f.def("sub nsw i64 0, %s", v.v)}
		}
	}
	g.unsupported(x, "this unary operator")
	return val{f.ty(f.nat(x), x), "poison"}
}

var opNames = map[token.Kind]string{
	token.Plus: "+", token.Minus: "-", token.Star: "*", token.Slash: "/", token.Percent: "%",
	token.Amp: "&", token.Pipe: "|", token.Caret: "^", token.Shl: "<<", token.Shr: ">>",
	token.Eq: "==", token.NotEq: "!=", token.Lt: "<", token.LtEq: "<=", token.Gt: ">", token.GtEq: ">=",
}

func (f *fn) binary(x *ast.BinaryExpr) val {
	g := f.g
	switch x.Op {
	case token.AndAnd, token.OrOr:
		bt := g.lower(g.preludeType("Bool"), x)
		slot := f.alloca(bt)
		l := f.val(x.X)
		f.emit("store i1 %s, ptr %s", l.v, slot)
		rhs, end := f.label("rhs"), f.label("lend")
		if x.Op == token.AndAnd {
			f.condBr(l.v, rhs, end)
		} else {
			f.condBr(l.v, end, rhs)
		}
		f.startBlock(rhs)
		r := f.val(x.Y)
		if !f.term {
			f.emit("store i1 %s, ptr %s", r.v, slot)
		}
		f.startBlock(end)
		return f.load(bt, slot)
	case token.Coalesce:
		return f.coalesce(x)
	}
	op := opNames[x.Op]
	xt, yt := f.nat(x.X), f.nat(x.Y)
	switch x.Op {
	case token.Eq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
		common := xt
		if g.key(xt) != g.key(yt) && g.optionElem(yt) != nil && g.optionElem(xt) == nil {
			common = yt
		}
		a := f.valAs(x.X, common)
		b := f.valAs(x.Y, common)
		return f.compare(op, a, b, x)
	}
	a := f.val(x.X)
	b := f.val(x.Y)
	return f.arith(op, a, b, x, f.nat(x))
}

// compare emits a comparison of two values of the same type.
func (f *fn) compare(op string, a, b val, at ast.Node) val {
	g := f.g
	bt := g.lower(g.preludeType("Bool"), at)
	if f.term {
		return val{bt, "poison"}
	}
	switch a.t.k {
	case kInt, kBool:
		pred := map[string]string{"==": "eq", "!=": "ne", "<": "slt", "<=": "sle", ">": "sgt", ">=": "sge"}[op]
		if a.t.k == kBool {
			pred = map[string]string{"==": "eq", "!=": "ne", "<": "ult", "<=": "ule", ">": "ugt", ">=": "uge"}[op]
		}
		return val{bt, f.def("icmp %s %s %s, %s", pred, a.t.ll, a.v, b.v)}
	case kFloat:
		pred := map[string]string{"==": "oeq", "!=": "une", "<": "olt", "<=": "ole", ">": "ogt", ">=": "oge"}[op]
		return val{bt, f.def("fcmp %s double %s, %s", pred, a.v, b.v)}
	case kStr:
		ap, an := f.strParts(a)
		bp, bn := f.strParts(b)
		if op == "==" || op == "!=" {
			r := f.def("call i32 @xo_str_eq(ptr %s, i64 %s, ptr %s, i64 %s)", ap, an, bp, bn)
			pred := map[string]string{"==": "ne", "!=": "eq"}[op]
			return val{bt, f.def("icmp %s i32 %s, 0", pred, r)}
		}
		r := f.def("call i64 @xo_str_cmp(ptr %s, i64 %s, ptr %s, i64 %s)", ap, an, bp, bn)
		pred := map[string]string{"<": "slt", "<=": "sle", ">": "sgt", ">=": "sge"}[op]
		return val{bt, f.def("icmp %s i64 %s, 0", pred, r)}
	case kUnit:
		return val{bt, map[string]string{"==": "true", "!=": "false", "<": "false", "<=": "true", ">": "false", ">=": "true"}[op]}
	case kStruct, kTuple, kEnum, kList:
		if op != "==" && op != "!=" {
			g.unsupported(at, "ordering comparisons on `"+a.t.name()+"`")
			return val{bt, "poison"}
		}
		pa, pb := f.spill(a), f.spill(b)
		r := f.def("call i1 %s(ptr %s, ptr %s)", g.eqFn(a.t, at), pa, pb)
		if op == "!=" {
			r = f.def("xor i1 %s, true", r)
		}
		return val{bt, r}
	}
	g.unsupported(at, "comparing values of type `"+a.t.name()+"`")
	return val{bt, "poison"}
}

// eqScalar compares two scalar values for equality (helpers).
func (f *fn) eqScalar(t *ty, a, b string, at ast.Node) string {
	switch t.k {
	case kFloat:
		return f.def("fcmp oeq double %s, %s", a, b)
	case kStr:
		ap, an := f.strParts(val{t, a})
		bp, bn := f.strParts(val{t, b})
		r := f.def("call i32 @xo_str_eq(ptr %s, i64 %s, ptr %s, i64 %s)", ap, an, bp, bn)
		return f.def("icmp ne i32 %s, 0", r)
	case kInt, kBool:
		return f.def("icmp eq %s %s, %s", t.ll, a, b)
	}
	f.g.unsupported(at, "comparing values of type `"+t.name()+"`")
	return "false"
}

// arith emits an arithmetic or bitwise operation (at is the node whose
// position faults report and whose range analysis entry applies).
func (f *fn) arith(op string, a, b val, at ast.Node, rt check.Type) val {
	g := f.g
	if f.term {
		return val{a.t, "poison"}
	}
	switch a.t.k {
	case kStr:
		if op == "+" {
			ap, an := f.strParts(a)
			bp, bn := f.strParts(b)
			p := f.def("call ptr @xo_str_concat(ptr %s, i64 %s, ptr %s, i64 %s)", ap, an, bp, bn)
			n := f.def("add i64 %s, %s", an, bn)
			return f.mkStr(p, n)
		}
	case kFloat:
		ins := map[string]string{"+": "fadd", "-": "fsub", "*": "fmul", "/": "fdiv", "%": "frem"}[op]
		if ins != "" {
			return val{a.t, f.def("%s double %s, %s", ins, a.v, b.v)}
		}
	case kBool:
		ins := map[string]string{"&": "and", "|": "or", "^": "xor"}[op]
		if ins != "" {
			return val{a.t, f.def("%s i1 %s, %s", ins, a.v, b.v)}
		}
	case kInt:
		if b.t.k != kInt {
			break
		}
		return f.intOp(op, a, b, at)
	}
	g.unsupported(at, fmt.Sprintf("the operator %s on `%s`", op, a.t.name()))
	return val{g.lower(rt, at), "poison"}
}

// intOp emits a checked Int operation (core 2.1: overflow and division by
// zero fault with the operands and the position).
func (f *fn) intOp(op string, a, b val, at ast.Node) val {
	g := f.g
	safe := f.safe[at]
	loc := func() string { return g.cstr(g.loc(at)) }
	fault := func(cond string, call string, args ...any) {
		fail, ok := f.label("fault"), f.label("ok")
		f.unlikely(cond, fail, ok)
		known := f.uniq
		f.startBlock(fail)
		f.faultCall(call, args...)
		f.uniq = known
		f.cont(ok)
	}
	opc := int64(op[0])
	switch op {
	case "+", "-", "*":
		ins := map[string]string{"+": "add", "-": "sub", "*": "mul"}[op]
		if safe {
			return val{a.t, f.def("%s nsw i64 %s, %s", ins, a.v, b.v)}
		}
		r := f.def("call { i64, i1 } @llvm.s%s.with.overflow.i64(i64 %s, i64 %s)", ins, a.v, b.v)
		ov := f.def("extractvalue { i64, i1 } %s, 1", r)
		fault(ov, "@xo_fault_overflow(i64 %s, i64 %d, i64 %s, ptr %s)", a.v, opc, b.v, loc())
		return val{a.t, f.def("extractvalue { i64, i1 } %s, 0", r)}
	case "/", "%":
		k, isConst := constInt(b.v)
		if !safe && !(isConst && k != 0) {
			z := f.def("icmp eq i64 %s, 0", b.v)
			fault(z, "@xo_fault_divzero(i64 %s, i64 %d, ptr %s)", a.v, opc, loc())
		}
		if op == "/" {
			if !safe && !(isConst && k != -1) {
				m1 := f.def("icmp eq i64 %s, -1", b.v)
				mn := f.def("icmp eq i64 %s, %d", a.v, int64(math.MinInt64))
				both := f.def("and i1 %s, %s", m1, mn)
				fault(both, "@xo_fault_overflow(i64 %s, i64 %d, i64 %s, ptr %s)", a.v, opc, b.v, loc())
			}
			return val{a.t, f.def("sdiv i64 %s, %s", a.v, b.v)}
		}
		if isConst && k != -1 {
			return val{a.t, f.def("srem i64 %s, %s", a.v, b.v)}
		}
		// Xo's `x % -1` is 0; LLVM's srem of the minimum by -1 is undefined
		m1 := f.def("icmp eq i64 %s, -1", b.v)
		d := f.def("select i1 %s, i64 1, i64 %s", m1, b.v)
		return val{a.t, f.def("srem i64 %s, %s", a.v, d)}
	case "&", "|", "^":
		ins := map[string]string{"&": "and", "|": "or", "^": "xor"}[op]
		return val{a.t, f.def("%s i64 %s, %s", ins, a.v, b.v)}
	case "<<", ">>":
		bad := f.def("icmp uge i64 %s, 64", b.v)
		fault(bad, "@xo_fault_shift(i64 %s, ptr %s)", b.v, loc())
		if op == ">>" {
			return val{a.t, f.def("ashr i64 %s, %s", a.v, b.v)}
		}
		r := f.def("shl i64 %s, %s", a.v, b.v)
		back := f.def("ashr i64 %s, %s", r, b.v)
		lost := f.def("icmp ne i64 %s, %s", back, a.v)
		fault(lost, "@xo_fault_shl(i64 %s, i64 %s, ptr %s)", a.v, b.v, loc())
		return val{a.t, r}
	}
	g.unsupported(at, "the operator "+op+" on Int")
	return val{a.t, "poison"}
}

func constInt(s string) (int64, bool) {
	k, err := strconv.ParseInt(s, 10, 64)
	return k, err == nil
}

// coalesce is `opt ?? default`: the default is evaluated only for None.
func (f *fn) coalesce(x *ast.BinaryExpr) val {
	g := f.g
	l := f.val(x.X)
	rt := f.nat(x)
	t := g.lower(rt, x)
	slot := f.alloca(t)
	p := f.spill(l)
	tag := f.tagOf(l)
	some, none, end := f.label("some"), f.label("none"), f.label("cend")
	isSome := f.def("icmp eq i64 %s, 1", tag)
	f.condBr(isSome, some, none)
	f.startBlock(some)
	if g.optionElem(rt) != nil && t.key == l.t.key {
		f.emit("store %s %s, ptr %s", t.ll, l.v, slot)
	} else {
		v := f.payload(p, l.t, 1, 0, x)
		f.emit("store %s %s, ptr %s", t.ll, v.v, slot)
	}
	f.br(end)
	f.startBlock(none)
	r := f.valAs(x.Y, rt)
	if !f.term {
		f.emit("store %s %s, ptr %s", t.ll, r.v, slot)
	}
	f.startBlock(end)
	return f.load(t, slot)
}
