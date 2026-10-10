package check

import (
	"strconv"

	"github.com/xo-lang/xo/internal/ast"
)

// mvLit is the closure literal standing for a method value.
type mvLit struct {
	lit  *ast.FnLit
	recv *VarObj
}

// MethodValueLit returns a closure literal equivalent to the method value
// x (`recv.method` used as a value, core 3.2): `fn(p0, p1, ...) {
// r.method(p0, p1, ...) }`, where r is the returned binding, which the
// caller sets to the receiver's value where x is evaluated (the receiver
// is evaluated once, when the value is made). The literal and its parts
// are recorded as if they had been checked, so a code generator lowers it
// as any closure (decision 0109). The same literal is returned for the
// same x. nil when x is not a method value.
func (i *Info) MethodValueLit(x *ast.SelectorExpr) (*ast.FnLit, *VarObj) {
	c := i.c
	if m := c.mvLits[x]; m != nil {
		return m.lit, m.recv
	}
	fo, ok := c.rec.ref[x].(*FuncObj)
	if !ok || fo.Recv == nil || fo.Static || fo.RecvVar {
		return nil, nil
	}
	ft, ok := Resolve(i.TypeOf(x)).(*Func)
	if !ok {
		if nt := i.NaturalType(x); nt != nil {
			ft, ok = Resolve(nt).(*Func)
		}
	}
	rt := i.NaturalType(x.X)
	if rt == nil {
		rt = i.TypeOf(x.X)
	}
	if !ok || rt == nil {
		return nil, nil
	}
	sp := x.Sp
	record := func(e ast.Expr, t Type) {
		c.exprType[e] = t
		c.rec.natType[e] = t
	}
	recv := &VarObj{Name: "xo.recv", Type: rt, Span: sp, Used: true}
	rid := &ast.Ident{Base: ast.Base{Sp: x.X.Span()}, Name: "xo.recv"}
	c.identObj[rid] = recv
	record(rid, rt)
	sel := &ast.SelectorExpr{Base: ast.Base{Sp: sp}, X: rid, Sel: x.Sel}
	call := &ast.CallExpr{Base: ast.Base{Sp: sp}, Fun: sel}
	lit := &ast.FnLit{Base: ast.Base{Sp: sp}}
	var params []*VarObj
	for k, p := range ft.Params {
		name := "xo.p" + strconv.Itoa(k)
		pv := &VarObj{Name: name, Type: p.T, Span: sp, Used: true}
		params = append(params, pv)
		id := &ast.Ident{Base: ast.Base{Sp: sp}, Name: name}
		lit.Params = append(lit.Params, &ast.Param{Base: ast.Base{Sp: sp}, Name: id})
		aid := &ast.Ident{Base: ast.Base{Sp: sp}, Name: name}
		c.identObj[id] = pv
		c.identObj[aid] = pv
		record(aid, p.T)
		call.Args = append(call.Args, &ast.Arg{Base: ast.Base{Sp: sp}, Value: aid})
	}
	res := ft.Result
	if res == nil {
		res = &Tuple{}
	}
	record(call, c.callResult(res, ft.Err))
	c.callInfo[call] = &callInfo{fn: fo, recvExpr: rid, sm: c.rec.subst[x]}
	lit.Body = &ast.Block{Base: ast.Base{Sp: sp}, Stmts: []ast.Stmt{&ast.ExprStmt{Base: ast.Base{Sp: sp}, X: call}}}
	record(lit, ft)
	c.rec.closureVar[lit] = params
	if c.mvLits == nil {
		c.mvLits = map[*ast.SelectorExpr]*mvLit{}
	}
	c.mvLits[x] = &mvLit{lit: lit, recv: recv}
	return lit, recv
}
