package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Generated values on the native backends (decision 0120): the inputs of
// property tests (`test "x" for v: T`, core 9.2) and testing.gen[T]
// (std.md 8.5). A generator is the C runtime's xo_gen (an Int handle,
// std/llrt/xo_testing.c): Go's PCG and a record of every choice. The
// functions here make a value of a checker type from it with the same
// choices, in the same order, as std/rt's Gen (gen.go) and the generated
// XoGenAny methods (internal/gen json.go), so a seed gives the Go
// backend's values and a recorded choice list replays to the same value
// (shrinking, cmd/xo nativetest.go). Scalars come from the runtime.

// sizedGenKind is xo_gen_sized's kind of each sized integer.
var sizedGenKind = map[string]int64{"Int32": 1, "Int16": 2, "Int8": 3, "UInt64": 4, "UInt32": 5, "UInt16": 6, "UInt8": 7}

// genValueFunc returns xo.gen[ct](g: Int) -> T: a value of checker type ct
// (lowered to T) from generator g.
func (b *builder) genValueFunc(ct check.Type, at ast.Node) string {
	ct = check.Deep(ct)
	t := b.typ(ct, at)
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	return b.genFunc("xo.gen["+b.typeName(ct)+"]", []*Type{intT}, []*Type{t}, func(s *fn, ps []*Value) {
		g := ps[0]
		sub := func(xt check.Type, want *Type) *Value {
			xt = check.Deep(xt)
			v := s.call(b.genValueFunc(xt, at), b.typ(xt, at), "", g)
			return s.coerce(v, want, at)
		}
		enter := func() { s.rt("gen_enter", nil, g) }
		leave := func() { s.rt("gen_leave", nil, g) }
		intn := func(n int64) *Value { return s.rt("gen_intn", intT, g, s.cint(n)) }
		eqInt := func(v *Value, k int64) *Value { return s.add(OpEq, boolT, nil, v, s.cint(k)) }
		if _, et, ok := resultParts(ct); ok && check.IsNever(et) {
			b.unsupported(at, "generated values of `"+ct.String()+"`")
			s.retv(s.undef(t))
			return
		}
		switch {
		case isPreludeNamed(ct, "Int", "Int64"):
			s.retv(s.rt("gen_int", t, g))
			return
		case isPreludeNamed(ct, "Int32", "Int16", "Int8", "UInt64", "UInt32", "UInt16", "UInt8"):
			n := check.Resolve(ct).(*check.Named)
			s.retv(s.rt("gen_sized", t, g, s.cint(sizedGenKind[n.Obj.Name])))
			return
		case isPreludeNamed(ct, "Float"):
			s.retv(s.rt("gen_float", t, g))
			return
		case isPreludeNamed(ct, "Float32"):
			s.retv(s.rt("f32_round", t, s.rt("gen_float", m.Builtin(KFloat), g)))
			return
		case isPreludeNamed(ct, "Bool"):
			s.retv(eqInt(intn(2), 1))
			return
		case isPreludeNamed(ct, "Str", "Bytes"):
			s.retv(s.rt("gen_str", t, g))
			return
		case isPreludeNamed(ct, "Rune"):
			s.retv(s.rt("gen_rune", t, g))
			return
		case isPreludeNamed(ct, "Duration"):
			s.retv(s.rt("gen_duration", t, g))
			return
		case isPreludeNamed(ct, "Time"):
			s.retv(s.rt("gen_time", t, g))
			return
		case isPreludeNamed(ct, "Secret") && t.Kind == KStruct && len(t.Fields) == 1:
			n := check.Resolve(ct).(*check.Named)
			s.retv(s.add(OpStruct, t, nil, sub(n.Args[0], t.Fields[0].Type)))
			return
		case isPreludeNamed(ct, "List") && t.Kind == KList:
			n := check.Resolve(ct).(*check.Named)
			enter()
			cnt := s.rt("gen_size", intT, g)
			xs := local("xs")
			s.writeVar(xs, s.call("list.new", t, ""))
			s.forUpTo(cnt, func(*Value) {
				s.writeVar(xs, s.call("list.push", t, "", s.read(xs), sub(n.Args[0], t.Elems[0])))
			})
			leave()
			s.retv(s.read(xs))
			return
		case isPreludeNamed(ct, "Map", "Set") && t.Coll:
			// a Set is a generated List made a set (std/rt): the same
			// choices as entries put one by one
			n := check.Resolve(ct).(*check.Named)
			put := b.mapPut(t, at)
			enter()
			cnt := s.rt("gen_size", intT, g)
			mv := local("m")
			s.writeVar(mv, s.emptyMap(t))
			s.forUpTo(cnt, func(*Value) {
				k := sub(n.Args[0], mapKey(t))
				v := s.unit()
				if len(n.Args) == 2 {
					v = sub(n.Args[1], mapVal(t))
				}
				s.writeVar(mv, s.call(put, t, "", s.read(mv), k, v))
			})
			leave()
			s.retv(s.read(mv))
			return
		case isPreludeNamed(ct, "Option") && t.Kind == KEnum:
			n := check.Resolve(ct).(*check.Named)
			none := s.add(OpVariant, t, t.VariantIndex("None"))
			s.ifThen(s.rt("gen_deep", boolT, g), func() { s.retv(none) })
			s.ifThen(eqInt(intn(4), 0), func() { s.retv(none) })
			enter()
			si := t.VariantIndex("Some")
			v := s.add(OpVariant, t, si, sub(n.Args[0], t.Variants[si].Fields[0].Type))
			leave()
			s.retv(v)
			return
		case isPreludeNamed(ct, "Result") && t.Kind == KEnum:
			n := check.Resolve(ct).(*check.Named)
			oi, ei := t.VariantIndex("Ok"), t.VariantIndex("Err")
			enter()
			s.ifThen(eqInt(intn(3), 0), func() {
				v := s.variant(t, "Err", []*Value{sub(n.Args[1], t.Variants[ei].Fields[0].Type)}, at)
				leave()
				s.retv(v)
			})
			v := s.add(OpVariant, t, oi, sub(n.Args[0], t.Variants[oi].Fields[0].Type))
			leave()
			s.retv(v)
			return
		}
		switch r := check.Resolve(ct).(type) {
		case *check.Tuple:
			if len(r.Elems) == 0 {
				s.retv(s.unit())
				return
			}
			if t.Kind == KTuple && len(t.Elems) == len(r.Elems) {
				var es []*Value
				for i, et := range r.Elems {
					es = append(es, sub(et, t.Elems[i]))
				}
				s.retv(s.add(OpTuple, t, nil, es...))
				return
			}
		case *check.Named:
			obj := r.Obj
			if obj.Module != nil && obj.Module.IsPrelude {
				break
			}
			sb := check.NewSubst(obj.TParams, r.Args)
			switch {
			case obj.Kind == check.KNewtype && t.Kind == KStruct && len(t.Fields) == 1:
				s.retv(s.add(OpStruct, t, nil, sub(sb.Apply(obj.Target), t.Fields[0].Type)))
				return
			case obj.Kind == check.KStruct && t.Kind == KStruct && !obj.Opaque:
				// every field; a struct with an invariant tries again,
				// up to 1000 times (std/rt's XoGenAny)
				var inv string
				if obj.Invariant != nil {
					inv = b.invariantFunc(r, t, at)
				}
				enter()
				s.forUpTo(s.cint(1001), func(try *Value) {
					fs := make([]*Value, len(t.Fields))
					for _, f := range obj.Fields {
						fi := t.FieldIndex(f.Name)
						if fi < 0 {
							b.unsupported(at, "generated values of `"+ct.String()+"`")
							return
						}
						fs[fi] = sub(sb.Apply(f.Type), t.Fields[fi].Type)
					}
					for i := range fs {
						if fs[i] == nil {
							b.unsupported(at, "generated values of `"+ct.String()+"`")
							return
						}
					}
					v := s.add(OpStruct, t, nil, fs...)
					ok := s.bool(true)
					if inv != "" {
						ok = s.add(OpOr, boolT, nil, s.call(inv, boolT, "", v), s.add(OpGe, boolT, nil, try, s.cint(1000)))
					}
					s.ifThen(ok, func() {
						leave()
						s.retv(v)
					})
				})
				s.add(OpUnreachable, nil, nil) // the last try returns
				s.dead()
				return
			case obj.Kind == check.KEnum && t.Kind == KEnum:
				// a random variant; a unit variant once the value is deep
				var units []int
				for i, vo := range obj.Variants {
					if len(vo.Fields) == 0 {
						units = append(units, i)
					}
				}
				enter()
				iv := local("i")
				s.writeVar(iv, intn(int64(len(obj.Variants))))
				if len(units) > 0 {
					s.ifThen(s.rt("gen_deep", boolT, g), func() {
						k := intn(int64(len(units)))
						s.writeVar(iv, s.cint(int64(units[0])))
						for j := 1; j < len(units); j++ {
							j := j
							s.ifThen(eqInt(k, int64(j)), func() { s.writeVar(iv, s.cint(int64(units[j]))) })
						}
					})
				}
				i := s.read(iv)
				for oi, vo := range obj.Variants {
					vi := t.VariantIndex(vo.Name)
					if vi < 0 {
						b.unsupported(at, "generated values of `"+ct.String()+"`")
						s.retv(s.undef(t))
						return
					}
					vo := vo
					s.ifThen(eqInt(i, int64(oi)), func() {
						var args []*Value
						for fi, f := range vo.Fields {
							args = append(args, sub(sb.Apply(f.Type), t.Variants[vi].Fields[fi].Type))
						}
						v := s.add(OpVariant, t, vi, args...)
						leave()
						s.retv(v)
					})
				}
				s.add(OpUnreachable, nil, nil) // i is one of the variants
				s.dead()
				return
			}
		}
		b.unsupported(at, "generated values of `"+ct.String()+"`")
		s.retv(s.undef(t))
	})
}
