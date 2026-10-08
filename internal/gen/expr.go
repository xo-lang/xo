package gen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

const assignOp = token.Assign

func compoundOp(k token.Kind) string {
	switch k {
	case token.PlusAssign:
		return "+"
	case token.MinusAssign:
		return "-"
	case token.StarAssign:
		return "*"
	case token.SlashAssign:
		return "/"
	case token.PercentAssign:
		return "%"
	}
	return "?"
}

// val is the Go code of e converted to its type in context.
func (f *fnGen) val(e ast.Expr) string {
	code := f.expr(e)
	nat := f.nat(e)
	want := f.info.TypeOf(e)
	if want == nil {
		return code
	}
	return f.coerce(code, nat, want)
}

// valAs is the Go code of e converted to type want.
func (f *fnGen) valAs(e ast.Expr, want check.Type) string {
	return f.coerce(f.expr(e), f.nat(e), want)
}

func trivialCode(s string) bool {
	if s == "" || s == "tk" || s == "true" || s == "false" {
		return true
	}
	if strings.HasSuffix(s, "Unit{}") && !strings.ContainsAny(s, "()") {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' && !strings.Contains(s[1:len(s)-1], "\"") {
		return true
	}
	return false
}

// operands generates n operand expressions in evaluation order. When an
// operand needs statements, the operands before it are spilled into
// temporaries first so they are still evaluated before it.
func (f *fnGen) operands(n int, gen func(i int) string) []string {
	return f.operandsKeep(n, gen, nil)
}

// operandsKeep is operands where keep(i) marks operands that must not be
// spilled: places (a `var` receiver, a `&place` argument), whose mutation
// must reach the original.
func (f *fnGen) operandsKeep(n int, gen func(i int) string, keep func(i int) bool) []string {
	codes := make([]string, n)
	for i := 0; i < n; i++ {
		var code string
		stmts := f.capture(func() { code = gen(i) })
		if stmts != "" {
			for j := 0; j < i; j++ {
				if keep != nil && keep(j) {
					continue
				}
				if !trivialCode(codes[j]) {
					t := f.tmp("a")
					f.line("%s := %s", t, codes[j])
					codes[j] = t
				}
			}
			f.out.WriteString(stmts)
		}
		codes[i] = code
	}
	return codes
}

// spill stores code in a temporary unless it is trivial.
func (f *fnGen) spill(code string) string {
	if trivialCode(code) || strings.HasPrefix(code, "x_") && !strings.ContainsAny(code, ".()[") {
		return code
	}
	t := f.tmp("t")
	f.line("%s := %s", t, code)
	return t
}

// ---------------------------------------------------------------------------
// Conversions (core 2.10)

// coerce converts code from type `from` to type `to`: Some wrapping,
// interface conversion, function adapters, and Never.
func (f *fnGen) coerce(code string, from, to check.Type) string {
	if from == nil || to == nil {
		return code
	}
	ft, tt := f.goType(from), f.goType(to)
	if check.IsNever(from) && !check.IsNever(to) {
		return fmt.Sprintf("%sAbsurd[%s](%s)", f.rt(), tt, code)
	}
	if ft == tt {
		return code
	}
	if el := f.g.optionElem(to); el != nil && f.g.optionElem(from) == nil {
		return fmt.Sprintf("%sSome[%s](%s)", f.rt(), f.goType(el), f.coerce(code, from, el))
	}
	if f.g.isInterface(to) || tt == "any" {
		return f.toIface(code, from, to)
	}
	if ff, ok := check.Resolve(from).(*check.Func); ok {
		if tf, ok := check.Resolve(to).(*check.Func); ok {
			return f.adaptFunc(code, ff, tf)
		}
	}
	if _, _, ok := f.g.resultParts(from); ok {
		if tt2, te2, ok := f.g.resultParts(to); ok {
			return f.coerceResult(code, from, tt2, te2)
		}
	}
	return code
}

// toIface converts a value to an interface type.
func (f *fnGen) toIface(code string, from, to check.Type) string {
	tt := f.goType(to)
	if tt == "any" {
		return "any(" + code + ")"
	}
	if f.g.isInterface(from) && f.goType(from) != tt {
		return fmt.Sprintf("%sIface[%s](%s)", f.rt(), tt, code)
	}
	n := f.g.named(to)
	if n != nil && n.Obj.Module.IsPrelude {
		return fmt.Sprintf("%sIface[%s](%s)", f.rt(), tt, code)
	}
	if _, ok := check.Resolve(from).(*check.ParamType); ok {
		return fmt.Sprintf("%sIface[%s](%s)", f.rt(), tt, code)
	}
	if check.IsNever(from) {
		return fmt.Sprintf("%sAbsurd[%s](%s)", f.rt(), tt, code)
	}
	if adapter := f.pg.ifaceAdapter(from, to); adapter != "" {
		return adapter + "{V: " + code + "}"
	}
	return tt + "(" + code + ")"
}

// coerceErr converts an error value at a `?` site (core 2.6): the same
// type, a Display type to Error, or wrapping in a `from` variant.
func (f *fnGen) coerceErr(code string, from, to check.Type) string {
	if to == nil || from == nil {
		return code
	}
	ft, tt := f.goType(from), f.goType(to)
	if ft == tt {
		return code
	}
	if check.IsNever(from) {
		return fmt.Sprintf("%sAbsurd[%s](%s)", f.rt(), tt, code)
	}
	if f.g.isPrelude(to, "Error") || f.g.isPrelude(to, "Display") {
		return fmt.Sprintf("%sIface[%s](%s)", f.rt(), tt, code)
	}
	if v := f.info.FromVariant(to, from); v != nil {
		return f.variantValue(v, check.Resolve(to).(*check.Named), []string{code})
	}
	return f.coerce(code, from, to)
}

// coerceResult converts a Result to Result[toT, toE].
func (f *fnGen) coerceResult(code string, from, toT, toE check.Type) string {
	ft, fe, _ := f.g.resultParts(from)
	if toE == nil {
		toE = f.neverType()
	}
	if f.goType(ft) == f.goType(toT) && f.goType(fe) == f.goType(toE) {
		return code
	}
	r := f.spill(code)
	return fmt.Sprintf("func() %s { if %s.Ok { return %sOk[%s, %s](%s) }; return %sErrTr[%s](%s, %s.Tr) }()",
		f.pg.resultOf(toT, toE), r, f.rt(), f.goType(toT), f.goType(toE), f.coerce(r+".V", ft, toT),
		f.rt(), f.goType(toT), f.coerceErr(r+".E", fe, toE), r)
}

func (f *fnGen) neverType() check.Type {
	return &check.Named{Obj: f.g.prelude.Scope["Never"].(*check.TypeObj)}
}

// adaptFunc wraps a function value so it has the Go form of type to
// (subsumption, core 6.2: a function that cannot fail where a failing one
// is expected, or an error that converts).
func (f *fnGen) adaptFunc(code string, from, to *check.Func) string {
	fv := f.spill(code)
	var ps, args []string
	for i, p := range to.Params {
		pt := f.goType(p.T)
		if p.Var {
			pt = "*" + pt
		}
		ps = append(ps, fmt.Sprintf("a%d %s", i, pt))
		args = append(args, fmt.Sprintf("a%d", i))
	}
	fromE := from.Err
	if fromE == nil {
		fromE = f.neverType()
	}
	toE := to.Err
	if toE == nil {
		toE = f.neverType()
	}
	res := f.pg.resultOf(to.Result, to.Err)
	call := fmt.Sprintf("%s(tk%s)", fv, prefixComma(args))
	return fmt.Sprintf("func(tk *%sTask%s) %s { r := %s; if r.Ok { return %sOk[%s, %s](%s) }; return %sErrTr[%s](%s, r.Tr) }",
		f.rt(), prefixComma(ps), res, call, f.rt(), f.goType(to.Result), f.goType(toE), f.coerce("r.V", from.Result, to.Result),
		f.rt(), f.goType(to.Result), f.coerceErr("r.E", fromE, toE))
}

func prefixComma(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return ", " + strings.Join(xs, ", ")
}

// ---------------------------------------------------------------------------
// Expressions

// expr generates e and returns Go code of its natural type. It may emit
// statements first.
func (f *fnGen) expr(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return f.expr(x.X)
	case *ast.IntLit:
		return f.intLit(x.Value, f.nat(x))
	case *ast.FloatLit:
		t := f.goType(f.nat(x))
		if t != "float32" {
			t = "float64"
		}
		return fmt.Sprintf("%s(%s)", t, x.Raw)
	case *ast.DurationLit:
		return fmt.Sprintf("%sDuration(%s)", f.rt(), x.Nanos)
	case *ast.RuneLit:
		return fmt.Sprintf("%sRune(%d)", f.rt(), x.Value)
	case *ast.BytesLit:
		return fmt.Sprintf("%sBytes(%s)", f.rt(), strconv.Quote(x.Value))
	case *ast.StringLit:
		return f.stringLit(x)
	case *ast.UnitExpr:
		return f.rt() + "Unit{}"
	case *ast.HoleExpr:
		return fmt.Sprintf("%sHole[%s](%q)", f.rt(), f.goType(f.typeOf(x)), f.g.loc(x))
	case *ast.Ident:
		return f.ident(x)
	case *ast.TupleExpr:
		t := check.Resolve(f.nat(x)).(*check.Tuple)
		codes := f.operands(len(x.Elems), func(i int) string { return f.valAs(x.Elems[i], t.Elems[i]) })
		var fs []string
		for i, c := range codes {
			fs = append(fs, fmt.Sprintf("F%d: %s", i, c))
		}
		return fmt.Sprintf("%s{%s}", f.goType(t), strings.Join(fs, ", "))
	case *ast.ListLit:
		return f.collLit("List", x.Elems, f.nat(x))
	case *ast.SetLit:
		return f.collLit("Set", x.Elems, f.nat(x))
	case *ast.MapLit:
		return f.mapLit(x)
	case *ast.StructLit:
		return f.structLit(x)
	case *ast.WithExpr:
		return f.withExpr(x)
	case *ast.UnaryExpr:
		return f.unary(x)
	case *ast.BinaryExpr:
		return f.binary(x)
	case *ast.IsExpr:
		v := f.spill(f.val(x.X))
		t := f.info.PatType(x.Pattern)
		if t == nil {
			t = f.typeOf(x.X)
		}
		m := f.pattern(x.Pattern, v, t)
		return m.condition()
	case *ast.RangeExpr:
		t := f.nat(x)
		n := f.g.named(t)
		et := f.goType(n.Args[0])
		codes := f.operands(2, func(i int) string {
			if i == 0 {
				return f.val(x.Lo)
			}
			return f.val(x.Hi)
		})
		return fmt.Sprintf("%sRange[%s]{Lo: %s, Hi: %s, Incl: %v}", f.rt(), et, codes[0], codes[1], x.Inclusive)
	case *ast.SelectorExpr:
		return f.selector(x)
	case *ast.CallExpr:
		return f.call(x)
	case *ast.TypeArgsExpr:
		return f.typeArgsValue(x)
	case *ast.IndexExpr:
		return f.index(x)
	case *ast.TryExpr:
		return f.try(x)
	case *ast.FnLit:
		return f.fnLit(x)
	case *ast.IfExpr, *ast.MatchExpr, *ast.Block, *ast.ScopeExpr, *ast.SelectExpr:
		t := f.typeOf(e)
		v := f.tmp("v")
		f.line("var %s %s", v, f.goType(t))
		f.into(e, dest{kind: dAssign, name: v, typ: t})
		return v
	case *ast.WhileExpr:
		f.loopWhile(nil, nil, x.Body, x)
		return f.rt() + "Unit{}"
	case *ast.BadExpr:
		f.g.errorf(x, "bad expression")
		return "nil"
	}
	f.g.errorf(e, "unsupported expression %T", e)
	return "nil"
}

func (f *fnGen) intLit(v string, t check.Type) string {
	gt := f.goType(t)
	switch gt {
	case "int64", "int8", "int16", "int32", "uint8", "uint16", "uint32", "uint64", "float64", "float32":
		return fmt.Sprintf("%s(%s)", gt, v)
	}
	return fmt.Sprintf("int64(%s)", v)
}

func (f *fnGen) stringLit(x *ast.StringLit) string {
	parts := append([]ast.StringPart(nil), x.Parts...)
	if len(parts) == 0 {
		return `""`
	}
	codes := f.operands(len(parts), func(i int) string {
		p := parts[i]
		if p.Expr == nil {
			return strconv.Quote(p.Text)
		}
		return f.display(f.val(p.Expr), f.typeOf(p.Expr))
	})
	if len(codes) == 1 {
		return codes[0]
	}
	return "(" + strings.Join(codes, " + ") + ")"
}

// display is the Display text of a value (core 1.6 interpolation).
func (f *fnGen) display(code string, t check.Type) string {
	t = f.pg.monoType(t)
	if f.g.isPrelude(t, "Str") {
		return code
	}
	if f.g.isPrelude(t, "Int") {
		return f.rt() + "IntStr(" + code + ")"
	}
	return fmt.Sprintf("%sDisplay(tk, %s)", f.rt(), code)
}

func (f *fnGen) collLit(kind string, elems []ast.Expr, t check.Type) string {
	n := f.g.named(t)
	et := n.Args[0]
	if len(elems) == 0 {
		return fmt.Sprintf("%s%s[%s]{}", f.rt(), kind, f.goType(et))
	}
	codes := f.operands(len(elems), func(i int) string { return f.valAs(elems[i], et) })
	return fmt.Sprintf("%s%sOf[%s](%s)", f.rt(), kind, f.goType(et), strings.Join(codes, ", "))
}

func (f *fnGen) mapLit(x *ast.MapLit) string {
	n := f.g.named(f.nat(x))
	kt, vt := n.Args[0], n.Args[1]
	if len(x.Entries) == 0 {
		return fmt.Sprintf("%sMap[%s, %s]{}", f.rt(), f.goType(kt), f.goType(vt))
	}
	codes := f.operands(2*len(x.Entries), func(i int) string {
		en := x.Entries[i/2]
		if i%2 == 0 {
			return f.valAs(en.Key, kt)
		}
		return f.valAs(en.Value, vt)
	})
	tup := fmt.Sprintf("%sTuple2[%s, %s]", f.rt(), f.goType(kt), f.goType(vt))
	var es []string
	for i := 0; i < len(codes); i += 2 {
		es = append(es, fmt.Sprintf("%s{F0: %s, F1: %s}", tup, codes[i], codes[i+1]))
	}
	return fmt.Sprintf("%sMapOf[%s, %s](%s)", f.rt(), f.goType(kt), f.goType(vt), strings.Join(es, ", "))
}

func (f *fnGen) structLit(x *ast.StructLit) string {
	t := f.nat(x)
	n := f.g.named(t)
	if n == nil {
		f.g.errorf(x, "struct literal of unknown type")
		return "nil"
	}
	obj := n.Obj
	s := check.NewSubst(obj.TParams, n.Args)
	codes := f.operands(len(x.Fields), func(i int) string {
		fi := x.Fields[i]
		var ft check.Type
		for _, fo := range obj.Fields {
			if fo.Name == fi.Name.Name {
				ft = s.Apply(fo.Type)
			}
		}
		return f.valAs(fi.Value, ft)
	})
	given := map[string]string{}
	for i, fi := range x.Fields {
		given[fi.Name.Name] = codes[i]
	}
	gt := f.goType(t)
	var parts []string
	for _, fo := range obj.Fields {
		boxed := f.g.isBoxedField(obj, fo)
		box := func(c string) string {
			if boxed {
				return f.rt() + "BoxOf(" + c + ")"
			}
			return c
		}
		if c, ok := given[fo.Name]; ok {
			parts = append(parts, fmt.Sprintf("%s: %s", goField(fo.Name), box(c)))
			continue
		}
		if fo.Default != nil {
			parts = append(parts, fmt.Sprintf("%s: %s", goField(fo.Name), box(f.fieldDefault(obj, fo, n))))
		}
	}
	lit := fmt.Sprintf("%s{%s}", gt, strings.Join(parts, ", "))
	if obj.Invariant != nil {
		v := f.tmp("s")
		f.line("%s := %s", v, lit)
		f.line("%s.XoInvariant(tk, %q)", v, f.g.loc(x))
		return v
	}
	return lit
}

// fieldDefault is the default value of a struct field.
func (f *fnGen) fieldDefault(obj *check.TypeObj, fo *check.FieldObj, n *check.Named) string {
	if obj.Module.IsStd {
		return f.stdDefault(fo.Default, fo.Type)
	}
	targs := ""
	if args := typeArgs(n); len(args) > 0 {
		targs = "[" + f.pg.typeList(args) + "]"
	}
	return fmt.Sprintf("%s%s_D_%s%s(tk)", f.pg.pkgRef(obj.Module), obj.Name, fo.Name, targs)
}

func (f *fnGen) withExpr(x *ast.WithExpr) string {
	t := f.nat(x)
	n := f.g.named(t)
	base := f.val(x.X)
	v := f.tmp("w")
	f.line("%s := %s", v, base)
	s := check.NewSubst(n.Obj.TParams, n.Args)
	for _, fi := range x.Fields {
		var ft check.Type
		for _, fo := range n.Obj.Fields {
			if fo.Name == fi.Name.Name {
				ft = s.Apply(fo.Type)
			}
		}
		c := f.valAs(fi.Value, ft)
		if f.g.boxedFieldNamed(t, fi.Name.Name) {
			c = f.rt() + "BoxOf(" + c + ")"
		}
		f.line("%s.%s = %s", v, goField(fi.Name.Name), c)
	}
	if n.Obj.Invariant != nil {
		f.line("%s.XoInvariant(tk, %q)", v, f.g.loc(x))
	}
	return v
}

// ---------------------------------------------------------------------------
// Identifiers and selectors

func (f *fnGen) ident(x *ast.Ident) string {
	if v := f.info.VarOf(x); v != nil {
		return f.varName(v)
	}
	if vo := f.info.Ctor(x); vo != nil {
		return f.ctorValue(vo, f.nat(x), nil)
	}
	switch o := f.info.Ref(x).(type) {
	case *check.FuncObj:
		return f.funcValue(o, f.typeOf(x), nil)
	case *check.ConstObj:
		return f.constRef(o)
	}
	switch x.Name {
	case "true", "false":
		return x.Name
	case "target":
		return f.rt() + "CurrentTarget()"
	case "None":
		return fmt.Sprintf("%sNone[%s]()", f.rt(), f.goType(f.g.optionElem(f.nat(x))))
	}
	f.g.errorf(x, "unresolved name `%s` in code generation", x.Name)
	return "nil"
}

func (f *fnGen) constRef(o *check.ConstObj) string {
	if o.Module.IsStd {
		return f.rt() + stdPrefix(o.Module) + o.Name
	}
	return f.pg.pkgRef(o.Module) + "C_" + o.Name
}

func (f *fnGen) selector(x *ast.SelectorExpr) string {
	if vo := f.info.Ctor(x); vo != nil {
		return f.ctorValue(vo, f.nat(x), nil)
	}
	if ref := f.info.Ref(x); ref != nil {
		switch o := ref.(type) {
		case *check.ConstObj:
			return f.constRef(o)
		case *check.FuncObj:
			if o.Const {
				return f.stdCall(o, nil, "", nil)
			}
			if o.Recv != nil && !o.Static && f.info.TypeOf(x.X) != nil {
				return f.methodValue(x, o)
			}
			return f.funcValue(o, f.typeOf(x), nil)
		}
	}
	bt := f.typeOf(x.X)
	if bt == nil {
		f.g.errorf(x, "cannot generate selector `%s`", x.Sel.Name)
		return "nil"
	}
	if n := f.g.named(bt); n != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Task" && x.Sel.Name == "await" {
		h := f.val(x.X)
		code := fmt.Sprintf("%sAwait(tk, %s)", f.rt(), h)
		if check.IsNever(n.Args[1]) {
			return code + ".V"
		}
		return code
	}
	base := f.val(x.X)
	if isDigits(x.Sel.Name) {
		return base + ".F" + x.Sel.Name
	}
	if f.g.boxedFieldNamed(bt, x.Sel.Name) {
		return base + "." + goField(x.Sel.Name) + ".Get()"
	}
	return base + "." + goField(x.Sel.Name)
}

// ---------------------------------------------------------------------------
// Operators

func (f *fnGen) unary(x *ast.UnaryExpr) string {
	t := f.nat(x)
	switch x.Op {
	case token.Bang:
		return "!(" + f.val(x.X) + ")"
	case token.Tilde:
		return "^(" + f.val(x.X) + ")"
	case token.Minus:
		if lit, ok := x.X.(*ast.IntLit); ok {
			return f.intLit("-"+lit.Value, t)
		}
		v := f.val(x.X)
		switch {
		case f.g.isFloat(t):
			return "(-" + v + ")"
		case f.g.isUnsigned(t):
			return fmt.Sprintf("%sNegU(%s, %q)", f.rt(), v, f.g.loc(x))
		}
		return fmt.Sprintf("%sNegS(%s, %q)", f.rt(), v, f.g.loc(x))
	}
	f.g.errorf(x, "unsupported unary operator")
	return "nil"
}

func opText(k token.Kind) string {
	switch k {
	case token.Plus:
		return "+"
	case token.Minus:
		return "-"
	case token.Star:
		return "*"
	case token.Slash:
		return "/"
	case token.Percent:
		return "%"
	case token.Amp:
		return "&"
	case token.Pipe:
		return "|"
	case token.Caret:
		return "^"
	case token.Shl:
		return "<<"
	case token.Shr:
		return ">>"
	case token.Eq:
		return "=="
	case token.NotEq:
		return "!="
	case token.Lt:
		return "<"
	case token.LtEq:
		return "<="
	case token.Gt:
		return ">"
	case token.GtEq:
		return ">="
	case token.AndAnd:
		return "&&"
	case token.OrOr:
		return "||"
	}
	return "?"
}

func (f *fnGen) binary(x *ast.BinaryExpr) string {
	if f.isExpectCompare(x) {
		return f.expectCompare(x)
	}
	switch x.Op {
	case token.AndAnd, token.OrOr:
		l := f.val(x.X)
		var r string
		stmts := f.capture(func() { r = f.val(x.Y) })
		if stmts == "" {
			return "(" + l + " " + opText(x.Op) + " " + r + ")"
		}
		v := f.tmp("b")
		f.line("%s := %s", v, l)
		if x.Op == token.AndAnd {
			f.line("if %s {", v)
		} else {
			f.line("if !%s {", v)
		}
		f.ind++
		f.out.WriteString(indent(stmts, 1))
		f.line("%s = %s", v, r)
		f.ind--
		f.line("}")
		return v
	case token.Coalesce:
		return f.coalesce(x)
	}
	xt, yt := f.nat(x.X), f.nat(x.Y)
	rt := f.nat(x)
	op := opText(x.Op)
	switch x.Op {
	case token.Eq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
		common := xt
		if f.goType(xt) != f.goType(yt) {
			if f.g.optionElem(yt) != nil && f.g.optionElem(xt) == nil || f.g.isInterface(yt) {
				common = yt
			}
		}
		codes := f.operands(2, func(i int) string {
			if i == 0 {
				return f.valAs(x.X, common)
			}
			return f.valAs(x.Y, common)
		})
		return f.compare(op, codes[0], codes[1], common)
	}
	codes := f.operands(2, func(i int) string {
		if i == 0 {
			return f.val(x.X)
		}
		return f.val(x.Y)
	})
	return f.binop(op, codes[0], codes[1], xt, yt, rt, x)
}

// indent adds n tabs before every line of s.
func indent(s string, n int) string {
	if s == "" {
		return s
	}
	pre := strings.Repeat("\t", n)
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "//line ") {
			continue
		}
		lines[i] = pre + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// compare generates a comparison of two values of type t.
func (f *fnGen) compare(op, a, b string, t check.Type) string {
	// inside a specialized copy a type parameter compares as its concrete
	// type (mono.go)
	t = f.pg.monoType(t)
	if f.pg.monoOn() {
		eq := op == "==" || op == "!="
		switch {
		case f.g.isFloat(t) && !eq:
			return fmt.Sprintf("(%sFloatCmp(float64(%s), float64(%s)) %s 0)", f.rt(), a, b, op)
		case eq && f.pg.hasTypedEq(t, false):
			// (a): a may be a composite literal, ambiguous in an if header
			if op == "!=" {
				return "!(" + a + ").XoEqT(" + b + ")"
			}
			return "(" + a + ").XoEqT(" + b + ")"
		case !eq && f.pg.hasTypedEq(t, true):
			return "((" + a + ").XoCmpT(" + b + ") " + op + " 0)"
		}
	}
	if f.g.isScalar(t) && !f.g.isPrelude(t, "Bool") && !f.g.isFloat(t) {
		return "(" + a + " " + op + " " + b + ")"
	}
	if f.g.isPrelude(t, "Bool") && (op == "==" || op == "!=") {
		return "(" + a + " " + op + " " + b + ")"
	}
	if f.g.isFloat(t) && (op == "==" || op == "!=") {
		return "(" + a + " " + op + " " + b + ")"
	}
	switch op {
	case "==":
		return fmt.Sprintf("%sEq(%s, %s)", f.rt(), a, b)
	case "!=":
		return fmt.Sprintf("!%sEq(%s, %s)", f.rt(), a, b)
	}
	return fmt.Sprintf("(%sCmp(%s, %s) %s 0)", f.rt(), a, b, op)
}

// binop generates an arithmetic or bitwise operation (core 2.1): checked
// for integers, Duration and Time rules, Str/Bytes/List concatenation.
func (f *fnGen) binop(op, a, b string, xt, yt, rt check.Type, at ast.Node) string {
	r := f.rt()
	loc := strconv.Quote(f.g.loc(at))
	xd, yd := f.g.isPrelude(xt, "Duration"), f.g.isPrelude(yt, "Duration")
	xtime, ytime := f.g.isPrelude(xt, "Time"), f.g.isPrelude(yt, "Time")
	switch {
	case xtime && yd:
		if op == "+" {
			return fmt.Sprintf("%sTimeAdd(%s, %s, %s)", r, a, b, loc)
		}
		return fmt.Sprintf("%sTimeSub(%s, %s, %s)", r, a, b, loc)
	case xtime && ytime:
		return fmt.Sprintf("%sTimeDiff(%s, %s, %s)", r, a, b, loc)
	case xd && f.g.isPrelude(yt, "Int"):
		if op == "*" {
			return fmt.Sprintf("%sDurMulInt(%s, %s, %s)", r, a, b, loc)
		}
		return fmt.Sprintf("%sDurDivInt(%s, %s, %s)", r, a, b, loc)
	case yd && f.g.isPrelude(xt, "Int"):
		return fmt.Sprintf("%sIntMulDur(%s, %s, %s)", r, a, b, loc)
	case xd && yd && op == "/":
		return fmt.Sprintf("%sDurDivDur(%s, %s, %s)", r, a, b, loc)
	}
	if f.g.isPrelude(xt, "Str") || f.g.isPrelude(xt, "Bytes") {
		return "(" + a + " + " + b + ")"
	}
	if f.g.isPrelude(xt, "List") {
		return a + ".Concat(" + b + ")"
	}
	if f.g.isFloat(xt) {
		switch op {
		case "%":
			return fmt.Sprintf("%sFMod(%s, %s)", r, a, b)
		case "/":
			// a call, not a Go constant expression: 0.0 / 0.0 is NaN at run
			// time but a compile error as a Go constant
			return fmt.Sprintf("%sFDiv(%s, %s)", r, a, b)
		}
		return "(" + a + " " + op + " " + b + ")"
	}
	signed := f.g.isSigned(xt)
	suffix := "U"
	if signed {
		suffix = "S"
	}
	if f.pg.monoOn() && signed && f.goType(xt) == "int64" && f.goType(yt) == "int64" {
		// typed, inlinable Int arithmetic (std/rt/mono.go)
		switch op {
		case "+", "-", "*", "/", "%":
			name := map[string]string{"+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Rem"}[op]
			return fmt.Sprintf("%s%sI(%s, %s, %s)", r, name, a, b, loc)
		}
	}
	switch op {
	case "+":
		return fmt.Sprintf("%sAdd%s(%s, %s, %s)", r, suffix, a, b, loc)
	case "-":
		return fmt.Sprintf("%sSub%s(%s, %s, %s)", r, suffix, a, b, loc)
	case "*":
		return fmt.Sprintf("%sMul%s(%s, %s, %s)", r, suffix, a, b, loc)
	case "/":
		return fmt.Sprintf("%sDiv%s(%s, %s, %s)", r, suffix, a, b, loc)
	case "%":
		return fmt.Sprintf("%sRem%s(%s, %s, %s)", r, suffix, a, b, loc)
	case "<<":
		return fmt.Sprintf("%sShl(%s, %s, %s)", r, a, b, loc)
	case ">>":
		return fmt.Sprintf("%sShr(%s, %s, %s)", r, a, b, loc)
	case "&", "|", "^":
		return "(" + a + " " + op + " " + b + ")"
	}
	f.g.errorf(at, "unsupported operator %s", op)
	return "nil"
}

// coalesce generates `opt ?? default`: the default is evaluated only when
// the optional is None.
func (f *fnGen) coalesce(x *ast.BinaryExpr) string {
	lt := f.nat(x.X)
	l := f.spill(f.valAs(x.X, lt))
	rt := f.nat(x)
	var r string
	stmts := f.capture(func() { r = f.valAs(x.Y, rt) })
	v := f.tmp("c")
	f.line("var %s %s", v, f.goType(rt))
	f.line("if %s.Some {", l)
	if f.g.optionElem(rt) != nil {
		f.line("\t%s = %s", v, l)
	} else {
		f.line("\t%s = %s.V", v, l)
	}
	f.line("} else {")
	f.ind++
	f.out.WriteString(indent(stmts, 0))
	f.line("%s = %s", v, r)
	f.ind--
	f.line("}")
	return v
}

// ---------------------------------------------------------------------------
// Indexing (core 3.6)

func (f *fnGen) index(x *ast.IndexExpr) string {
	bt := f.typeOf(x.X)
	n := f.g.named(bt)
	if n == nil {
		f.g.errorf(x, "cannot index")
		return "nil"
	}
	loc := strconv.Quote(f.g.loc(x))
	if rng, ok := x.Index.(*ast.RangeExpr); ok {
		var lo, hi string
		codes := f.operands(3, func(i int) string {
			switch i {
			case 0:
				return f.val(x.X)
			case 1:
				if rng.Lo == nil {
					return "0"
				}
				return f.val(rng.Lo)
			}
			if rng.Hi == nil {
				return "0"
			}
			h := f.val(rng.Hi)
			if rng.Inclusive {
				return fmt.Sprintf("%sAddS(%s, 1, %s)", f.rt(), h, loc)
			}
			return h
		})
		base, lo, hi := codes[0], codes[1], codes[2]
		args := fmt.Sprintf("%s, %s, %v, %v, %s", lo, hi, rng.Lo != nil, rng.Hi != nil, loc)
		switch n.Obj.Name {
		case "Str":
			return fmt.Sprintf("%sStrSlice(%s, %s)", f.rt(), base, args)
		case "Bytes":
			return fmt.Sprintf("%sBytesSlice(%s, %s)", f.rt(), base, args)
		}
		return fmt.Sprintf("%s.SliceRange(%s)", base, args)
	}
	codes := f.operands(2, func(i int) string {
		if i == 0 {
			return f.val(x.X)
		}
		return f.val(x.Index)
	})
	switch n.Obj.Name {
	case "Bytes":
		return fmt.Sprintf("%sBytesAt(%s, %s, %s)", f.rt(), codes[0], codes[1], loc)
	case "List":
		if f.pg.monoOn() {
			return fmt.Sprintf("%s.AtM(%s, %s)", codes[0], codes[1], loc)
		}
	}
	return fmt.Sprintf("%s.At(%s, %s)", codes[0], codes[1], loc)
}

// ---------------------------------------------------------------------------
// `?` (core 2.5, 2.6, 11.1)

func (f *fnGen) try(x *ast.TryExpr) string {
	xt := f.nat(x.X)
	v := f.tmp("r")
	f.line("%s := %s", v, f.val(x.X))
	if tt, et, ok := f.g.resultParts(xt); ok {
		_ = tt
		f.line("if !%s.Ok {", v)
		f.ind++
		f.propagate(v+".E", v+".Tr", et, x)
		f.ind--
		f.line("}")
		return v + ".V"
	}
	f.line("if !%s.Some {", v)
	f.ind++
	f.emitReturnValue(f.okValue(fmt.Sprintf("%sNone[%s]()", f.rt(), f.goType(f.g.optionElem(f.fr.resT)))))
	f.ind--
	f.line("}")
	return v + ".V"
}

// propagate returns an error from the current frame, converted to its
// error type, with a frame appended (core 11.1).
func (f *fnGen) propagate(errCode, trCode string, et check.Type, at ast.Node) {
	fr := f.fr
	conv := f.coerceErr(errCode, et, fr.errT)
	label := f.frameLabel()
	sp := at.Span()
	loc := fmt.Sprintf("%s:%d", f.g.linePath(sp.File), sp.Start.Line)
	tr := fmt.Sprintf("%s.Frame(%s, %q)", trCode, label, loc)
	if !fr.isResult {
		f.line("panic(%sAsFault(%q))", f.rt(), "error propagated out of a function that cannot fail")
		return
	}
	f.emitReturnValue(fmt.Sprintf("%sErrTr[%s](%s, %s)", f.rt(), f.goType(fr.resT), conv, tr))
}

// frameLabel is the code of the current frame's label (core 11.1):
// function name and the Debug text of its parameters. Capabilities are
// left out; they carry no information.
func (f *fnGen) frameLabel() string {
	var args []string
	for _, p := range f.fr.params {
		if f.g.isCapability(p.t) {
			continue
		}
		args = append(args, strconv.Quote(p.xo), p.code)
	}
	return fmt.Sprintf("%sFrameLabel(%q%s)", f.rt(), f.fr.name, prefixComma(args))
}

// ---------------------------------------------------------------------------
// Closures (core 3.3)

func (f *fnGen) fnLit(x *ast.FnLit) string {
	ft, ok := check.Resolve(f.nat(x)).(*check.Func)
	if !ok {
		f.g.errorf(x, "closure without a function type")
		return "nil"
	}
	// captured mutable bindings are copied: closures capture by value
	saved := map[*check.VarObj]string{}
	for _, v := range f.freeMutable(x) {
		cur := f.varName(v)
		c := f.tmp("cap")
		f.line("%s := %s", c, cur)
		saved[v] = cur
		f.names[v] = c
	}
	params := f.info.ClosureParams(x)
	var ps []string
	outer := f.fr
	fr := &frame{parent: outer, name: outer.name + ".closure", resT: ft.Result, errT: ft.Err, isResult: true, isTest: outer.isTest}
	fr.params = outer.params
	fr.goRes = f.pg.resultOf(ft.Result, ft.Err)
	for i, pv := range params {
		name := f.declName(pv)
		pt := f.goType(ft.Params[i].T)
		if ft.Params[i].Var {
			pt = "*" + pt
			f.names[pv] = "(*" + name + ")"
		}
		ps = append(ps, name+" "+pt)
	}
	f.fr = fr
	savedLine := f.lastLine
	body := f.capture(func() {
		f.ind++
		f.at(x.Body)
		for _, pv := range params {
			f.line("_ = %s", f.varName(pv))
		}
		f.topBlock(x.Body)
		f.ind--
	})
	f.fr = outer
	f.lastLine = savedLine
	for v, n := range saved {
		f.names[v] = n
	}
	var b strings.Builder
	fmt.Fprintf(&b, "func(tk *%sTask", f.rt())
	for _, p := range ps {
		b.WriteString(", " + p)
	}
	fmt.Fprintf(&b, ") %s {\n%s", fr.goRes, body)
	for i := 0; i < f.ind; i++ {
		b.WriteByte('\t')
	}
	b.WriteString("}")
	return b.String()
}

// freeMutable returns the mutable bindings declared outside closure x and
// used inside it.
func (f *fnGen) freeMutable(x *ast.FnLit) []*check.VarObj {
	declared := map[*check.VarObj]bool{}
	for _, v := range f.info.ClosureParams(x) {
		declared[v] = true
	}
	var out []*check.VarObj
	seen := map[*check.VarObj]bool{}
	inspectNode(x.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			v := f.info.VarOf(id)
			if v == nil || seen[v] {
				return true
			}
			if _, known := f.names[v]; !known {
				return true // declared inside the closure
			}
			seen[v] = true
			if v.Mutable && !declared[v] {
				out = append(out, v)
			}
		}
		return true
	})
	return out
}
