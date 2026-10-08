package gen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// callKind tells how a generated call can be used.
type callKind int

const (
	ckValue callKind = iota // an expression that is not a Go call
	ckCall                  // a Go call expression (valid as a statement)
	ckVoid                  // a Go call that returns nothing
)

// call generates a call in value position.
func (f *fnGen) call(x *ast.CallExpr) string {
	code, kind := f.callCore(x)
	if kind == ckVoid {
		f.line("%s", code)
		return f.rt() + "Unit{}"
	}
	return code
}

// callStmt generates a call in statement position. stmt is true when code
// is a valid Go statement.
func (f *fnGen) callStmt(x *ast.CallExpr) (string, bool) {
	code, kind := f.callCore(x)
	return code, kind != ckValue
}

func (f *fnGen) callCore(x *ast.CallExpr) (string, callKind) {
	// expect(cond) in tests
	if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "expect" && f.info.VarOf(id) == nil && f.info.Ref(id) == nil && f.info.CallOf(x) == nil {
		cond := f.val(x.Args[0].Value)
		return fmt.Sprintf("%sExpect(tk, %s, %q, %q)", f.rt(), cond, f.g.source(x.Args[0].Value), f.g.loc(x)), ckVoid
	}
	if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "old" && f.oldVals != nil {
		if v, ok := f.oldVals[x]; ok {
			return v, ckValue
		}
	}
	if vo := f.info.Ctor(x); vo != nil {
		return f.ctorCall(vo, x), ckValue
	}
	if vo := f.info.Ctor(x.Fun); vo != nil {
		return f.ctorCall(vo, x), ckValue
	}
	if f.isRetCtor(x) {
		// Ok(x)/Err(x) outside return position use the call's type
		return f.retCtorValue(x), ckValue
	}
	if obj := f.info.TypeCall(x); obj != nil {
		return f.typeCall(obj, x), ckValue
	}
	ci := f.info.CallOf(x)
	if ci == nil {
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "min", "max", "clamp":
				return f.ordCall(sel, x), ckValue
			}
		}
		f.g.errorf(x, "unresolved call in code generation")
		return "nil", ckValue
	}
	if ci.Fn == nil {
		return f.valueCall(x)
	}
	return f.declCall(x, ci)
}

// retCtorValue is `Ok(x)` or `Err(x)` typed by the checker as the Result
// of the enclosing function.
func (f *fnGen) retCtorValue(x *ast.CallExpr) string {
	id := x.Fun.(*ast.Ident)
	t := f.typeOf(x)
	rt, et, ok := f.g.resultParts(t)
	if !ok {
		rt, et = f.fr.resT, f.fr.errT
	}
	arg := x.Args[0].Value
	if id.Name == "Err" {
		return fmt.Sprintf("%sErr[%s, %s](%s)", f.rt(), f.goType(rt), f.errGo(et), f.valAs(arg, et))
	}
	return fmt.Sprintf("%sOk[%s, %s](%s)", f.rt(), f.goType(rt), f.errGo(et), f.valAs(arg, rt))
}

// ---------------------------------------------------------------------------
// Calls of function values

func (f *fnGen) valueCall(x *ast.CallExpr) (string, callKind) {
	saved := f.writebacks
	f.writebacks = nil
	code, kind := f.valueCall1(x)
	wbs := f.writebacks
	f.writebacks = saved
	return f.withWritebacks(code, kind, wbs)
}

func (f *fnGen) valueCall1(x *ast.CallExpr) (string, callKind) {
	if code, ok := f.litCall(x); ok {
		if strings.HasPrefix(code, f.rt()+"Ok[") {
			return code, ckCall
		}
		return code, ckValue
	}
	ft, ok := check.Resolve(f.exprType(x.Fun)).(*check.Func)
	if !ok {
		f.g.errorf(x, "call of a non function value")
		return "nil", ckValue
	}
	codes := f.operandsKeep(len(x.Args)+1, func(i int) string {
		if i == 0 {
			return f.val(x.Fun)
		}
		p := ft.Params[i-1]
		if p.Var {
			return "&" + f.place(x.Args[i-1].Value)
		}
		return f.valAs(x.Args[i-1].Value, p.T)
	}, func(i int) bool { return i > 0 && ft.Params[i-1].Var })
	call := fmt.Sprintf("%s(tk%s)", codes[0], prefixComma(codes[1:]))
	if ft.Err == nil {
		return call + ".V", ckValue
	}
	return call, ckCall
}

// exprType is the type of e, falling back to what the checker did not
// record: the type of a local binding, a struct field, or a tuple field
// that is called.
func (f *fnGen) exprType(e ast.Expr) check.Type {
	if t := f.typeOf(e); t != nil {
		return t
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return f.exprType(x.X)
	case *ast.Ident:
		if v := f.info.VarOf(x); v != nil {
			return check.Deep(v.Type)
		}
	case *ast.SelectorExpr:
		bt := f.exprType(x.X)
		if tt, ok := check.Resolve(bt).(*check.Tuple); ok && isDigits(x.Sel.Name) {
			i := 0
			fmt.Sscan(x.Sel.Name, &i)
			if i < len(tt.Elems) {
				return tt.Elems[i]
			}
		}
		if n := f.g.named(bt); n != nil {
			for _, fo := range n.Obj.Fields {
				if fo.Name == x.Sel.Name {
					return check.NewSubst(n.Obj.TParams, n.Args).Apply(fo.Type)
				}
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Calls of declared functions and methods

// argExprs matches call arguments to parameters: positional first, then
// named. Missing ones are nil (their default is used).
func argExprs(params []*check.ParamInfo, call *ast.CallExpr) []ast.Expr {
	out := make([]ast.Expr, len(params))
	pos := 0
	for _, a := range call.Args {
		if a.Name == nil {
			if pos < len(out) {
				out[pos] = a.Value
			}
			pos++
			continue
		}
		for i, p := range params {
			if p.Name == a.Name.Name {
				out[i] = a.Value
			}
		}
	}
	return out
}

func (f *fnGen) declCall(x *ast.CallExpr, ci *check.Call) (string, callKind) {
	saved := f.writebacks
	f.writebacks = nil
	code, kind := f.declCall1(x, ci)
	wbs := f.writebacks
	f.writebacks = saved
	return f.withWritebacks(code, kind, wbs)
}

// withWritebacks runs a call that mutated copies of places, then stores
// the copies back.
func (f *fnGen) withWritebacks(code string, kind callKind, wbs []func()) (string, callKind) {
	if len(wbs) == 0 {
		return code, kind
	}
	res := ""
	if kind == ckVoid {
		f.line("%s", code)
	} else {
		res = f.tmp("res")
		f.line("%s := %s", res, code)
	}
	for _, wb := range wbs {
		wb()
	}
	if kind == ckVoid {
		return f.rt() + "Unit{}", ckValue
	}
	return res, ckValue
}

func (f *fnGen) declCall1(x *ast.CallExpr, ci *check.Call) (string, callKind) {
	fo := ci.Fn
	sub := ci.Subst
	if fo.Module.IsStd {
		if code, kind, ok := f.specialStd(x, ci); ok {
			return code, kind
		}
		if code, ok := f.hofInline(x, ci); ok {
			return code, ckValue
		}
	}
	// a constraint method on a type parameter inside a specialized copy
	// calls the concrete method (mono.go)
	callee, calleeSub := fo, sub
	if m, ms := f.devirt(fo, ci.Recv); m != nil {
		callee, calleeSub = m, ms
	}
	params := fo.Params
	exprs := argExprs(params, x)
	// evaluation order: receiver, then arguments as written
	order := []int{}
	if ci.Recv != nil {
		order = append(order, -1)
	}
	seen := map[ast.Expr]int{}
	for i, e := range exprs {
		if e != nil {
			seen[e] = i
		}
	}
	for _, a := range x.Args {
		if i, ok := seen[a.Value]; ok {
			order = append(order, i)
		}
	}
	recvCode := ""
	argCodes := make([]string, len(params))
	codes := f.operandsKeep(len(order), func(k int) string {
		i := order[k]
		if i < 0 {
			return f.recvCode(ci.Recv, callee)
		}
		p := params[i]
		if p.Var {
			return "&" + f.place(exprs[i])
		}
		pt := p.Type
		if sub != nil {
			pt = sub.Apply(pt)
		}
		return f.valAs(exprs[i], pt)
	}, func(k int) bool {
		i := order[k]
		if i < 0 {
			return fo.RecvVar
		}
		return params[i].Var
	})
	for k, i := range order {
		if i < 0 {
			recvCode = codes[k]
		} else {
			argCodes[i] = codes[k]
		}
	}
	for i, p := range params {
		if exprs[i] == nil {
			argCodes[i] = f.paramDefault(fo, i, p, sub)
		}
	}
	if lits := f.litArgs(callee, calleeSub, x); lits != nil {
		if name, ok := f.specForLits(callee, calleeSub, lits); ok {
			return f.emitSpecCall(callee, calleeSub, name, recvCode, argCodes)
		}
	}
	return f.emitCall(callee, calleeSub, recvCode, argCodes, x)
}

// emitSpecCall calls the specialized copy name of fo.
func (f *fnGen) emitSpecCall(fo *check.FuncObj, sub *check.Subst, name, recv string, args []string) (string, callKind) {
	var call string
	if fo.Recv == nil {
		call = fmt.Sprintf("%s(tk%s)", name, prefixComma(args))
	} else {
		if fo.RecvVar {
			recv = "&" + recv
		}
		call = fmt.Sprintf("%s(tk, %s%s)", name, recv, prefixComma(args))
	}
	return f.callResult(fo, sub, call)
}

// recvCode generates the receiver of a method call: an addressable place
// for a `var` receiver.
func (f *fnGen) recvCode(e ast.Expr, fo *check.FuncObj) string {
	if fo.RecvVar {
		return f.place(e)
	}
	t := f.typeOf(e)
	code := f.val(e)
	if pt, ok := check.Resolve(t).(*check.ParamType); ok && fo.Iface {
		c := pt.P.Constraint
		if c != nil {
			return fmt.Sprintf("%sIface[%s](%s)", f.rt(), f.goType(c), code)
		}
	}
	return code
}

// paramDefault is the default value of parameter i of fo.
func (f *fnGen) paramDefault(fo *check.FuncObj, i int, p *check.ParamInfo, sub *check.Subst) string {
	if p.Default == nil {
		return "MISSING_ARG"
	}
	pt := p.Type
	if sub != nil {
		pt = sub.Apply(pt)
	}
	if fo.Module.IsStd {
		return f.stdDefault(p.Default, pt)
	}
	targs := f.targs(fo.TParams, sub)
	return fmt.Sprintf("%s%s_D_%d%s(tk)", f.pg.pkgRef(fo.Module), funcGoName(fo), i, targs)
}

// targs is `[T1, T2]` for the non effect parameters under sub.
func (f *fnGen) targs(ps []*check.TParam, sub *check.Subst) string {
	ps = typeParams(ps)
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		t := sub.Type(p)
		if t == nil {
			parts[i] = f.rt() + "Unit"
			continue
		}
		parts[i] = f.goType(t)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// emitCall builds the Go call of a declared function or method with the
// receiver and arguments already generated.
func (f *fnGen) emitCall(fo *check.FuncObj, sub *check.Subst, recv string, args []string, at ast.Node) (string, callKind) {
	if spec, ok := f.specFor(fo, sub); ok {
		return f.emitSpecCall(fo, sub, spec, recv, args)
	}
	var call string
	switch {
	case fo.Iface && methodIsGoMethod(fo):
		call = fmt.Sprintf("%s.%s(tk%s)", recv, goMethod(fo.Name), prefixComma(args))
	case fo.Module.IsStd:
		call = f.stdCall(fo, sub, recv, args)
	case fo.Recv == nil:
		call = fmt.Sprintf("%sF_%s%s(tk%s)", f.pg.pkgRef(fo.Module), fo.Name, f.targs(fo.TParams, sub), prefixComma(args))
	case methodIsGoMethod(fo):
		call = fmt.Sprintf("%s.%s(tk%s)", recv, goMethod(fo.Name), prefixComma(args))
	default:
		if fo.RecvVar {
			recv = "&" + recv
		}
		call = fmt.Sprintf("%s%s%s(tk, %s%s)", f.pg.pkgRef(fo.Module), funcGoName(fo), f.targs(fo.TParams, sub), recv, prefixComma(args))
	}
	return f.callResult(fo, sub, call)
}

// callResult classifies a generated call of fo: void, a Result whose
// error type became Never (`.V` taken), or a call.
func (f *fnGen) callResult(fo *check.FuncObj, sub *check.Subst, call string) (string, callKind) {
	if returnsVoid(fo) {
		return call, ckVoid
	}
	if returnsResult(fo) {
		et := fo.Err
		if sub != nil {
			et = sub.Apply(et)
		}
		if check.IsNever(check.Deep(et)) {
			return call + ".V", ckValue
		}
	}
	return call, ckCall
}

// stdCall names a std function or method in the runtime (see the calling
// convention in std/rt/core.go).
func (f *fnGen) stdCall(fo *check.FuncObj, sub *check.Subst, recv string, args []string) string {
	r := f.rt()
	prefix := stdPrefix(fo.Module)
	switch {
	case fo.Recv == nil:
		return fmt.Sprintf("%s%s%s%s(tk%s)", r, prefix, fo.Name, f.targs(fo.TParams, sub), prefixComma(args))
	case fo.Static:
		return fmt.Sprintf("%s%s%s_%s%s(tk%s)", r, stdPrefix(fo.Recv.Module), fo.Recv.Name, fo.Name, f.targs(fo.TParams, sub), prefixComma(args))
	case methodIsGoMethod(fo):
		return fmt.Sprintf("%s.%s(tk%s)", recv, goMethod(fo.Name), prefixComma(args))
	}
	if fo.RecvVar {
		recv = "&" + recv
	}
	tps := fo.TParams
	if fo.Iface {
		tps = append(append([]*check.TParam{}, fo.Recv.TParams...), fo.TParams...)
	}
	return fmt.Sprintf("%s%s%s_%s%s(tk, %s%s)", r, stdPrefix(fo.Recv.Module), fo.Recv.Name, fo.Name, f.targs(tps, sub), recv, prefixComma(args))
}

// ---------------------------------------------------------------------------
// Function values of declared functions (core 3.3, 6.2)

// funcValue wraps a declared function as a function value of type t.
func (f *fnGen) funcValue(fo *check.FuncObj, t check.Type, recv *string) string {
	ft, ok := check.Resolve(t).(*check.Func)
	if !ok {
		f.g.errorf(nil, "function value `%s` without a function type", fo.Name)
		return "nil"
	}
	sub := f.instSubst(fo, ft)
	var ps, args []string
	for i, p := range ft.Params {
		pt := f.goType(p.T)
		if p.Var {
			pt = "*" + pt
		}
		ps = append(ps, fmt.Sprintf("a%d %s", i, pt))
		args = append(args, fmt.Sprintf("a%d", i))
	}
	rv := ""
	if recv != nil {
		rv = *recv
	}
	saved := f.out
	var b strings.Builder
	f.out = &b
	call, kind := f.emitCall(fo, sub, rv, args, nil)
	f.out = saved
	res := f.pg.resultOf(ft.Result, ft.Err)
	var body string
	switch {
	case kind == ckVoid:
		body = fmt.Sprintf("%s; return %sOk[%sUnit, %s](%sUnit{})", call, f.rt(), f.rt(), f.errGo(ft.Err), f.rt())
	case returnsResult(fo) && !strings.HasSuffix(call, ".V"):
		body = "return " + call
	default:
		body = fmt.Sprintf("return %sOk[%s, %s](%s)", f.rt(), f.goType(ft.Result), f.errGo(ft.Err), call)
	}
	return fmt.Sprintf("func(tk *%sTask%s) %s { %s }", f.rt(), prefixComma(ps), res, body)
}

// methodValue is `x.method` used as a value: the receiver is evaluated
// now and captured.
func (f *fnGen) methodValue(x *ast.SelectorExpr, fo *check.FuncObj) string {
	rv := f.spill(f.recvCode(x.X, fo))
	return f.funcValue(fo, f.typeOf(x), &rv)
}

// instSubst recovers the type arguments of a generic function from its
// instantiated type.
func (f *fnGen) instSubst(fo *check.FuncObj, ft *check.Func) *check.Subst {
	if len(fo.TParams) == 0 {
		return nil
	}
	m := map[*check.TParam]check.Type{}
	for i, p := range fo.Params {
		if i < len(ft.Params) {
			matchTypes(p.Type, ft.Params[i].T, m)
		}
	}
	matchTypes(fo.Result, ft.Result, m)
	if fo.Err != nil && ft.Err != nil {
		matchTypes(fo.Err, ft.Err, m)
	}
	var args []check.Type
	for _, p := range fo.TParams {
		t := m[p]
		if t == nil && fo.Err != nil {
			if pt, ok := fo.Err.(*check.ParamType); ok && pt.P == p {
				t = f.neverType()
			}
		}
		if t == nil {
			t = check.Type(&check.Tuple{})
		}
		args = append(args, t)
	}
	return check.NewSubst(fo.TParams, args)
}

// matchTypes binds the type parameters in pat to the parts of conc.
func matchTypes(pat, conc check.Type, m map[*check.TParam]check.Type) {
	conc = check.Resolve(conc)
	switch p := pat.(type) {
	case *check.ParamType:
		if _, ok := m[p.P]; !ok && conc != nil {
			m[p.P] = conc
		}
	case *check.Named:
		c, ok := conc.(*check.Named)
		if !ok || c.Obj != p.Obj {
			return
		}
		for i := range p.Args {
			if i < len(c.Args) {
				matchTypes(p.Args[i], c.Args[i], m)
			}
		}
	case *check.Tuple:
		c, ok := conc.(*check.Tuple)
		if !ok {
			return
		}
		for i := range p.Elems {
			if i < len(c.Elems) {
				matchTypes(p.Elems[i], c.Elems[i], m)
			}
		}
	case *check.Func:
		c, ok := conc.(*check.Func)
		if !ok {
			return
		}
		for i := range p.Params {
			if i < len(c.Params) {
				matchTypes(p.Params[i].T, c.Params[i].T, m)
			}
		}
		matchTypes(p.Result, c.Result, m)
		if p.Err != nil && c.Err != nil {
			matchTypes(p.Err, c.Err, m)
		}
	}
}

// typeArgsValue is `f[T]` used as a value.
func (f *fnGen) typeArgsValue(x *ast.TypeArgsExpr) string {
	var fo *check.FuncObj
	switch g := x.X.(type) {
	case *ast.Ident:
		fo, _ = f.info.Ref(g).(*check.FuncObj)
	case *ast.SelectorExpr:
		fo, _ = f.info.Ref(g).(*check.FuncObj)
	}
	if fo == nil {
		f.g.errorf(x, "unsupported generic function value")
		return "nil"
	}
	return f.funcValue(fo, f.typeOf(x), nil)
}

// ---------------------------------------------------------------------------
// Constructors

// ctorCall constructs a variant with arguments.
func (f *fnGen) ctorCall(v *check.VariantObj, x *ast.CallExpr) string {
	t := f.nat(x)
	n := f.g.named(t)
	if n == nil || n.Obj != v.Enum {
		if el := f.g.optionElem(t); el != nil {
			n = f.g.named(el)
		}
	}
	var s *check.Subst
	if n != nil {
		s = check.NewSubst(v.Enum.TParams, n.Args)
	}
	fields := make([]ast.Expr, len(v.Fields))
	pos := 0
	for _, a := range x.Args {
		if a.Name == nil {
			if pos < len(fields) {
				fields[pos] = a.Value
			}
			pos++
			continue
		}
		for i, vf := range v.Fields {
			if vf.Name == a.Name.Name {
				fields[i] = a.Value
			}
		}
	}
	codes := f.operands(len(fields), func(i int) string {
		if fields[i] == nil {
			return "MISSING_FIELD"
		}
		ft := v.Fields[i].Type
		if s != nil {
			ft = s.Apply(ft)
		}
		return f.valAs(fields[i], ft)
	})
	return f.variantValue(v, n, codes)
}

// ctorValue is a unit variant used as a value.
func (f *fnGen) ctorValue(v *check.VariantObj, t check.Type, args []string) string {
	n := f.g.named(t)
	if n != nil && n.Obj != v.Enum {
		if el := f.g.optionElem(t); el != nil {
			n = f.g.named(el)
		}
	}
	return f.variantValue(v, n, args)
}

// variantValue builds an enum value from field codes.
func (f *fnGen) variantValue(v *check.VariantObj, n *check.Named, codes []string) string {
	r := f.rt()
	if v.Enum.Module.IsPrelude {
		switch v.Enum.Name {
		case "Option":
			if v.Name == "None" {
				return fmt.Sprintf("%sNone[%s]()", r, f.goType(n.Args[0]))
			}
			return fmt.Sprintf("%sSome[%s](%s)", r, f.goType(n.Args[0]), codes[0])
		case "Result":
			if v.Name == "Ok" {
				return fmt.Sprintf("%sOk[%s, %s](%s)", r, f.goType(n.Args[0]), f.goType(n.Args[1]), codes[0])
			}
			return fmt.Sprintf("%sErr[%s, %s](%s)", r, f.goType(n.Args[0]), f.goType(n.Args[1]), codes[0])
		}
	}
	et := f.goType(n)
	if len(v.Fields) == 0 {
		return fmt.Sprintf("%s{Tag: %d}", et, v.Index+1)
	}
	var fs []string
	for i, c := range codes {
		fs = append(fs, fmt.Sprintf("%s: %s", vfieldName(v.Fields[i], i), c))
	}
	return fmt.Sprintf("%s{Tag: %d, P: %s{%s}}", et, v.Index+1, f.payloadType(v, n), strings.Join(fs, ", "))
}

// typeCall is a numeric conversion `Int32(x)` (core 2.1) or a newtype
// construction `Email(s)` (core 2.8).
func (f *fnGen) typeCall(obj *check.TypeObj, x *ast.CallExpr) string {
	arg := x.Args[0].Value
	if obj.Kind == check.KNewtype {
		t := f.nat(x)
		n := f.g.named(t)
		s := check.NewSubst(obj.TParams, n.Args)
		return fmt.Sprintf("%s{F0: %s}", f.goType(t), f.valAs(arg, s.Apply(obj.Target)))
	}
	to := f.goType(f.nat(x))
	at := f.nat(arg)
	from := f.goType(at)
	v := f.val(arg)
	if from == to {
		return v
	}
	loc := strconv.Quote(f.g.loc(x))
	toFloat := to == "float64" || to == "float32"
	fromFloat := from == "float64" || from == "float32"
	switch {
	case toFloat:
		return fmt.Sprintf("%s(%s)", to, v)
	case fromFloat:
		return fmt.Sprintf("%sFloatToInt[%s](%s, %s)", f.rt(), to, v, loc)
	}
	return fmt.Sprintf("%sConv[%s, %s](%s, %s)", f.rt(), to, from, v, loc)
}

// ordCall is `a.min(b)`, `a.max(b)`, `a.clamp(lo, hi)` on an Ord type.
func (f *fnGen) ordCall(sel *ast.SelectorExpr, x *ast.CallExpr) string {
	t := f.typeOf(x)
	n := len(x.Args) + 1
	codes := f.operands(n, func(i int) string {
		if i == 0 {
			return f.valAs(sel.X, t)
		}
		return f.valAs(x.Args[i-1].Value, t)
	})
	switch sel.Sel.Name {
	case "min":
		return fmt.Sprintf("%sMin(%s, %s)", f.rt(), codes[0], codes[1])
	case "max":
		return fmt.Sprintf("%sMax(%s, %s)", f.rt(), codes[0], codes[1])
	}
	return fmt.Sprintf("%sClamp(%s, %s, %s)", f.rt(), codes[0], codes[1], codes[2])
}

// ---------------------------------------------------------------------------
// expect (core 9.2)

func (f *fnGen) isExpectCompare(x *ast.BinaryExpr) bool {
	call, ok := x.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "expect" && f.info.VarOf(id) == nil && f.info.Ref(id) == nil && f.info.CallOf(call) == nil && len(call.Args) == 1
}

// expectCompare is `expect(a) op b`: the comparison, and both values in
// the failure message.
func (f *fnGen) expectCompare(x *ast.BinaryExpr) string {
	a := x.X.(*ast.CallExpr).Args[0].Value
	at, bt := f.nat(a), f.nat(x.Y)
	common := at
	if f.goType(at) != f.goType(bt) {
		if f.g.optionElem(bt) != nil && f.g.optionElem(at) == nil || f.g.isInterface(bt) {
			common = bt
		}
	}
	codes := f.operands(2, func(i int) string {
		if i == 0 {
			return f.spill(f.valAs(a, common))
		}
		return f.spill(f.valAs(x.Y, common))
	})
	op := opText(x.Op)
	cmp := f.compare(op, codes[0], codes[1], common)
	src := f.g.source(x)
	return fmt.Sprintf("%sExpectCmp(tk, %s, %s, %s, %q, %q, %q)", f.rt(), cmp, codes[0], codes[1], op, src, f.g.loc(x))
}

var _ = token.Assign
