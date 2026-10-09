package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Rows into structs on the native backends (`derive SqlRow`, std.md 10.4,
// decision 0083): the intrinsic sql_decode[T, R] of std/native/rtsql.xo is
// lowered to a function generated per struct T,
//
//	xo.sql.dec[T](r: R) -> Result[T, sql.SqlErr]
//
// which reads each field, in declaration order, through the accessor of
// the implementation's Row type R for the field's type (`int`, `str`,
// `time_opt`, ...) and returns the first failure, as std/rt's SqlRowField
// does for the Go backend.

// sqlIntrinsic lowers sql_decode; ok is false for any other intrinsic.
func (s *fn) sqlIntrinsic(x *ast.CallExpr, ci *check.Call, es []ast.Expr) (*Value, bool) {
	fo := ci.Fn
	if fo.Name != "sql_decode" || fo.Module == nil || fo.Module.Path != "native/rtsql" {
		return nil, false
	}
	tps := typeParams(fo)
	if len(tps) != 2 || ci.Subst == nil || len(es) != 1 {
		return nil, false
	}
	ct := check.Deep(s.apply(ci.Subst.Type(tps[0])))
	rowCT := check.Deep(s.apply(ci.Subst.Type(tps[1])))
	nt := s.natOf(x)
	row := s.valueAs(es[0], s.b.typ(rowCT, x))
	name := s.b.sqlDecode(ct, rowCT, nt, x)
	return s.call(name, nt, "", row), true
}

// sqlAccessor is the name of the Row accessor that reads a field of type
// ct, or "" for a type no column holds.
func sqlAccessor(ct check.Type) string {
	for _, n := range []string{"Int", "Float", "Str", "Bool", "Time"} {
		if isPreludeNamed(ct, n) {
			return lowerName(n)
		}
	}
	if isPreludeNamed(ct, "Option") {
		if n, ok := check.Resolve(check.Deep(ct)).(*check.Named); ok && len(n.Args) == 1 {
			if a := sqlAccessor(n.Args[0]); a != "" && !isPreludeNamed(n.Args[0], "Option") {
				return a + "_opt"
			}
		}
	}
	return ""
}

// sqlDecode returns xo.sql.dec[ct], whose result type is rt
// (Result[T, sql.SqlErr] lowered).
func (b *builder) sqlDecode(ct, rowCT check.Type, rt *Type, at ast.Node) string {
	m := b.mod
	st, intT, boolT := m.Builtin(KStr), m.Builtin(KInt), m.Builtin(KBool)
	rowT := b.typ(rowCT, at)
	return b.genFunc("xo.sql.dec["+ct.String()+"]", []*Type{rowT}, []*Type{rt}, func(s *fn, ps []*Value) {
		row := ps[0]
		errI, okI := rt.VariantIndex("Err"), rt.VariantIndex("Ok")
		if rt.Kind != KEnum || errI < 0 || okI < 0 {
			b.unsupported(at, "decoding a row into `"+ct.String()+"`")
			s.retv(s.undef(rt))
			return
		}
		et := rt.Variants[errI].Fields[0].Type
		t := rt.Variants[okI].Fields[0].Type
		n, ok := check.Resolve(ct).(*check.Named)
		if !ok || n.Obj.Kind != check.KStruct || t.Kind != KStruct {
			b.unsupported(at, "decoding a row into `"+ct.String()+"`")
			s.retv(s.undef(rt))
			return
		}
		var vals []*Value
		for _, f := range n.Obj.Fields {
			col := s.add(OpConst, st, f.Name)
			acc := sqlAccessor(f.Type)
			md := s.methodOf(rowCT, acc)
			if acc == "" || md == nil {
				e := s.variant(et, "Mismatch", []*Value{col, s.add(OpConst, st, "the field type cannot be read from a column")}, at)
				s.retv(s.add(OpVariant, rt, errI, e))
				return
			}
			callee := b.declare(md, nil, at)
			crt := callee.Results[0]
			r := s.call(callee.Name, crt, "", row, col)
			s.check(callee.Name)
			ce, co := crt.VariantIndex("Err"), crt.VariantIndex("Ok")
			if crt.Kind != KEnum || ce < 0 || co < 0 {
				b.unsupported(at, "the accessor `"+acc+"` of `"+rowCT.String()+"`")
				s.retv(s.undef(rt))
				return
			}
			s.ifThen(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, r), s.cint(int64(ce))), func() {
				e := s.add(OpPayload, crt.Variants[ce].Fields[0].Type, PayloadRef{ce, 0}, r)
				s.retv(s.add(OpVariant, rt, errI, e))
			})
			vals = append(vals, s.add(OpPayload, crt.Variants[co].Fields[0].Type, PayloadRef{co, 0}, r))
		}
		s.retv(s.add(OpVariant, rt, okI, s.add(OpStruct, t, nil, vals...)))
	})
}
