package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// JSON codecs on the native backends (std.md 5, decision 0074): the
// intrinsics json_enc and json_dec of std/native/rtjson.xo are lowered to
// functions generated per checker type, which produce what std/rt's
// JsonEnc and the XoJsonEnc and XoJsonDec methods internal/gen generates
// produce (internal/gen/json.go):
//
//	xo.json.enc[T](acc: Str, v: T) -> Str
//	xo.json.dec[T](src: Str, tape: List[Int], i: Int) -> Result[T, json.DecodeErr]
//
// A decoder reads the value at tape node i (std/native/llrt/jsontext.xo); its
// error's path is relative to that value (`.name`, `[3]`), and each
// caller puts its own part in front, so the path is only built when
// decoding fails.

// Tape node kinds (std/native/llrt/jsontext.xo).
const (
	jkNull = iota
	jkFalse
	jkTrue
	jkNum
	jkStr
	jkArr
	jkObj
)

// genericIntrinsicJSON lowers json_enc and json_dec.
func (s *fn) jsonIntrinsic(x *ast.CallExpr, ci *check.Call, es []ast.Expr) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	m := b.mod
	st, intT := m.Builtin(KStr), m.Builtin(KInt)
	tps := typeParams(fo)
	if len(tps) != 1 || ci.Subst == nil {
		return nil, false
	}
	ct := check.Deep(s.apply(ci.Subst.Type(tps[0])))
	switch fo.Name {
	case "json_enc":
		acc := s.valueAs(es[0], st)
		v := s.valueAs(es[1], b.typ(ct, x))
		return s.call(b.jsonEnc(ct, x), st, "", acc, v), true
	case "json_dec":
		src := s.valueAs(es[0], st)
		tape := s.valueAs(es[1], m.List(intT))
		i := s.valueAs(es[2], intT)
		name := b.jsonDec(ct, x)
		return s.call(name, b.mod.Func(name).Results[0], "", src, tape, i), true
	}
	return nil, false
}

// decodeErrType is the checker type json.DecodeErr.
func (b *builder) decodeErrType() check.Type {
	for _, md := range b.info.AllModules() {
		if md.Path == "std/json" {
			if obj, ok := md.Scope["DecodeErr"].(*check.TypeObj); ok {
				return &check.Named{Obj: obj}
			}
		}
	}
	return nil
}

// isJSONPatch reports json.Patch[T].
func isJSONPatch(ct check.Type) bool {
	n, ok := check.Resolve(check.Deep(ct)).(*check.Named)
	return ok && n.Obj.Module != nil && n.Obj.Module.Path == "std/json" && n.Obj.Name == "Patch"
}

// jsonEnc returns xo.json.enc[ct].
func (b *builder) jsonEnc(ct check.Type, at ast.Node) string {
	m := b.mod
	st, intT, boolT := m.Builtin(KStr), m.Builtin(KInt), m.Builtin(KBool)
	t := b.typ(ct, at)
	return b.genFunc("xo.json.enc["+b.typeName(ct)+"]", []*Type{st, t}, []*Type{st}, func(s *fn, ps []*Value) {
		acc, v := ps[0], ps[1]
		lit := func(a *Value, text string) *Value {
			return s.call("str.concat", st, "", a, s.add(OpConst, st, text))
		}
		sub := func(a, x *Value, xt check.Type) *Value {
			x = s.unbox(x)
			xt = check.Deep(xt)
			return s.call(b.jsonEnc(xt, at), st, "", a, x)
		}
		switch {
		case isPreludeNamed(ct, "Duration"):
			s.retv(lit(s.llrt("append_duration", st, at, lit(acc, `"`), v), `"`))
			return
		case isPreludeNamed(ct, "Time"):
			s.retv(lit(s.llrt("append_time", st, at, lit(acc, `"`), v), `"`))
			return
		case isPreludeNamed(ct, "Bytes"):
			s.retv(s.jsonRT("rs_append_b64_quoted", st, acc, v))
			return
		}
		switch t.Kind {
		case KStr:
			s.retv(s.jsonRT("rs_append_json_str", st, acc, v))
			return
		case KInt:
			s.retv(s.numText(acc, v))
			return
		case KFloat:
			s.retv(s.jsonRT("rs_append_json_float", st, acc, v))
			return
		case KBool:
			s.retv(s.call("str.concat", st, "", acc, s.call("str.of_bool", st, "", v)))
			return
		case KUnit:
			s.retv(lit(acc, "null"))
			return
		case KBox:
			s.retv(sub(acc, v, ct))
			return
		}
		if tu, isTup := check.Resolve(ct).(*check.Tuple); isTup && t.Kind == KTuple && len(tu.Elems) == len(t.Elems) {
			// an array, one element per position (decision 0088)
			a := lit(acc, "[")
			for k, et := range tu.Elems {
				if k > 0 {
					a = lit(a, ",")
				}
				a = sub(a, s.add(OpField, t.Elems[k], k, v), et)
			}
			s.retv(lit(a, "]"))
			return
		}
		n, ok := check.Resolve(ct).(*check.Named)
		if !ok {
			b.unsupported(at, "JSON for `"+ct.String()+"`")
			s.retv(acc)
			return
		}
		obj := n.Obj
		if obj.Module != nil && obj.Module.IsPrelude {
			switch obj.Name {
			case "Option":
				some := t.VariantIndex("Some")
				av := local("acc")
				s.writeVar(av, acc)
				join := func() {}
				_ = join
				isSome := s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, v), s.cint(int64(some)))
				yes, no := s.branch(isSome)
				s.cur = yes
				s.retv(sub(acc, s.add(OpPayload, t.Variants[some].Fields[0].Type, PayloadRef{some, 0}, v), n.Args[0]))
				s.cur = no
				s.retv(lit(acc, "null"))
				return
			case "List":
				et := t.Elems[0]
				av := local("acc")
				s.writeVar(av, lit(acc, "["))
				s.forUpTo(s.call("list.len", intT, "", v), func(i *Value) {
					s.ifThen(s.add(OpGt, boolT, nil, i, s.cint(0)), func() { s.writeVar(av, lit(s.read(av), ",")) })
					s.writeVar(av, sub(s.read(av), s.call("list.index", et, "", v, i), n.Args[0]))
				})
				s.retv(lit(s.read(av), "]"))
				return
			case "Map", "Set":
				K, V := mapKey(t), mapVal(t)
				if obj.Name == "Map" && K.Kind != KStr {
					b.unsupported(at, "JSON for a Map whose keys are not Str")
				}
				keys := s.add(OpField, t.Fields[0].Type, 0, v)
				vals := s.add(OpField, t.Fields[1].Type, 1, v)
				live := s.add(OpField, t.Fields[2].Type, 2, v)
				av, first := local("acc"), local("first")
				open, close := "{", "}"
				if obj.Name == "Set" {
					open, close = "[", "]"
				}
				s.writeVar(av, lit(acc, open))
				s.writeVar(first, s.bool(true))
				s.forUpTo(s.call("list.len", intT, "", keys), func(i *Value) {
					s.ifThen(s.call("list.index", boolT, "", live, i), func() {
						s.ifThen(s.add(OpNot, boolT, nil, s.read(first)), func() { s.writeVar(av, lit(s.read(av), ",")) })
						s.writeVar(first, s.bool(false))
						k := s.call("list.index", K, "", keys, i)
						var a *Value
						if obj.Name == "Map" {
							a = lit(s.jsonRT("rs_append_json_str", st, s.read(av), k), ":")
							a = sub(a, s.call("list.index", V, "", vals, i), n.Args[1])
						} else {
							a = sub(s.read(av), k, n.Args[0])
						}
						s.writeVar(av, a)
					})
				})
				s.retv(lit(s.read(av), close))
				return
			}
		}
		sb := check.NewSubst(obj.TParams, n.Args)
		if isJSONPatch(ct) {
			vi := t.VariantIndex("Value")
			yes, no := s.branch(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, v), s.cint(int64(vi))))
			s.cur = yes
			s.retv(sub(acc, s.add(OpPayload, t.Variants[vi].Fields[0].Type, PayloadRef{vi, 0}, v), n.Args[0]))
			s.cur = no
			s.retv(lit(acc, "null"))
			return
		}
		switch {
		case t.Kind == KStruct && obj.Kind == check.KStruct && !obj.Opaque:
			// {"f":v,...}; an Absent Patch field is left out
			av, first := local("acc"), local("first")
			s.writeVar(av, lit(acc, "{"))
			s.writeVar(first, s.bool(true))
			for _, f := range obj.Fields {
				fi := t.FieldIndex(f.Name)
				if fi < 0 {
					continue
				}
				ft := sb.Apply(f.Type)
				fv := s.unbox(s.add(OpField, t.Fields[fi].Type, fi, v))
				write := func() {
					s.ifThen(s.add(OpNot, boolT, nil, s.read(first)), func() { s.writeVar(av, lit(s.read(av), ",")) })
					s.writeVar(first, s.bool(false))
					s.writeVar(av, sub(lit(s.read(av), `"`+f.Name+`":`), fv, ft))
				}
				if isJSONPatch(ft) {
					absent := fv.Type.VariantIndex("Absent")
					s.ifThen(s.add(OpNe, boolT, nil, s.add(OpTag, intT, nil, fv), s.cint(int64(absent))), write)
					continue
				}
				write()
			}
			s.retv(lit(s.read(av), "}"))
			return
		case t.Kind == KEnum && obj.Kind == check.KEnum:
			tag := s.add(OpTag, intT, nil, v)
			for _, vo := range obj.Variants {
				vi := t.VariantIndex(vo.Name)
				if vi < 0 {
					continue
				}
				s.ifThen(s.add(OpEq, boolT, nil, tag, s.cint(int64(vi))), func() {
					if len(vo.Fields) == 0 {
						s.retv(lit(acc, `"`+vo.Name+`"`))
						return
					}
					a := lit(acc, `{"`+vo.Name+`":`)
					if len(vo.Fields) == 1 && (vo.Fields[0].From || vo.Fields[0].Name == "") {
						ft := t.Variants[vi].Fields[0].Type
						a = sub(a, s.add(OpPayload, ft, PayloadRef{vi, 0}, v), sb.Apply(vo.Fields[0].Type))
						s.retv(lit(a, "}"))
						return
					}
					a = lit(a, "{")
					for fi, f := range vo.Fields {
						if fi > 0 {
							a = lit(a, ",")
						}
						ft := t.Variants[vi].Fields[fi].Type
						a = sub(lit(a, `"`+f.Name+`":`), s.add(OpPayload, ft, PayloadRef{vi, fi}, v), sb.Apply(f.Type))
					}
					s.retv(lit(a, "}}"))
				})
			}
			s.retv(acc)
			return
		}
		b.unsupported(at, "JSON for `"+ct.String()+"`")
		s.retv(acc)
	})
}

// jsonDec returns xo.json.dec[ct].
func (b *builder) jsonDec(ct check.Type, at ast.Node) string {
	m := b.mod
	st, intT, boolT := m.Builtin(KStr), m.Builtin(KInt), m.Builtin(KBool)
	t := b.typ(ct, at)
	det := b.decodeErrType()
	if det == nil {
		b.unsupported(at, "JSON decoding without std/json")
		return "?"
	}
	rt := b.typ(b.resultOf(ct, det), at)
	listInt := m.List(intT)
	name := "xo.json.dec[" + b.typeName(ct) + "]"
	return b.genFunc(name, []*Type{st, listInt, intT}, []*Type{rt}, func(s *fn, ps []*Value) {
		src, tape, i := ps[0], ps[1], ps[2]
		errI := rt.VariantIndex("Err")
		dt := rt.Variants[errI].Fields[0].Type
		okI := rt.VariantIndex("Ok")
		str := func(text string) *Value { return s.add(OpConst, st, text) }
		mkErr := func(path, msg *Value) *Value {
			return s.add(OpVariant, rt, errI, s.add(OpStruct, dt, nil, path, msg))
		}
		fail := func(msg string) { s.retv(mkErr(str(""), str(msg))) }
		ok := func(v *Value) { s.retv(s.add(OpVariant, rt, okI, s.coerce(v, t, at))) }
		at3 := func(j *Value, k int64) *Value {
			return s.call("list.index", intT, "", tape, s.add(OpAdd, intT, nil, j, s.cint(k)))
		}
		kind := s.call("list.index", intT, "", tape, i)
		isKind := func(k int64) *Value { return s.add(OpEq, boolT, nil, kind, s.cint(k)) }
		need := func(k int64, msg string) {
			s.ifThen(s.add(OpNot, boolT, nil, isKind(k)), func() { fail(msg) })
		}
		// child decodes the value at node j as xt; on an error it returns
		// from this decoder with seg in front of the error's path
		child := func(xt check.Type, j, seg *Value) *Value {
			xt = check.Deep(xt)
			cn := b.jsonDec(xt, at)
			cf := b.mod.Func(cn)
			if cf == nil {
				return s.undef(b.typ(xt, at))
			}
			crt := cf.Results[0]
			r := s.call(cn, crt, "", src, tape, j)
			ce := crt.VariantIndex("Err")
			cdt := crt.Variants[ce].Fields[0].Type
			s.ifThen(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, r), s.cint(int64(ce))), func() {
				e := s.add(OpPayload, cdt, PayloadRef{ce, 0}, r)
				p := s.call("str.concat", st, "", seg, s.add(OpField, st, 0, e))
				s.retv(mkErr(p, s.add(OpField, st, 1, e)))
			})
			co := crt.VariantIndex("Ok")
			return s.add(OpPayload, crt.Variants[co].Fields[0].Type, PayloadRef{co, 0}, r)
		}
		next := func(j *Value) *Value { return s.jsonRT("json_next", intT, tape, j) }
		jstr := func(j *Value) *Value { return s.jsonRT("json_str", st, src, tape, j) }
		switch {
		case isPreludeNamed(ct, "Duration"):
			need(jkStr, "expected duration string")
			txt := jstr(i)
			s.ifThen(s.add(OpNot, boolT, nil, s.llrt("duration_ok", boolT, at, txt)), func() {
				fail(`expected duration string such as "1.5s"`)
			})
			ok(s.llrt("duration", intT, at, txt))
			return
		case isPreludeNamed(ct, "Time"):
			need(jkStr, "expected RFC 3339 time string")
			txt := jstr(i)
			s.ifThen(s.add(OpNot, boolT, nil, s.llrt("time_ok", boolT, at, txt)), func() {
				fail("expected RFC 3339 time string")
			})
			ok(s.llrt("time", intT, at, txt))
			return
		case isPreludeNamed(ct, "Bytes"):
			need(jkStr, "expected base64 string")
			txt := jstr(i)
			s.ifThen(s.add(OpNot, boolT, nil, s.jsonRT("b64_ok", boolT, txt)), func() { fail("expected base64 string") })
			ok(s.jsonRT("b64_decode", st, txt))
			return
		}
		switch t.Kind {
		case KStr:
			need(jkStr, "expected string")
			ok(jstr(i))
			return
		case KInt:
			need(jkNum, "expected integer")
			s.ifThen(s.add(OpNot, boolT, nil, s.jsonRT("json_int_ok", boolT, src, tape, i)), func() { fail("expected integer") })
			ok(s.jsonRT("json_int", intT, src, tape, i))
			return
		case KFloat:
			need(jkNum, "expected number")
			s.ifThen(s.add(OpNot, boolT, nil, s.jsonRT("json_float_ok", boolT, src, tape, i)), func() { fail("expected number") })
			ok(s.jsonRT("json_float", m.Builtin(KFloat), src, tape, i))
			return
		case KBool:
			s.ifThen(isKind(jkTrue), func() { ok(s.bool(true)) })
			s.ifThen(isKind(jkFalse), func() { ok(s.bool(false)) })
			fail("expected boolean")
			return
		case KUnit:
			need(jkNull, "expected null")
			ok(s.unit())
			return
		}
		if tu, isTup := check.Resolve(ct).(*check.Tuple); isTup && t.Kind == KTuple && len(tu.Elems) == len(t.Elems) {
			// an array of exactly as many elements as positions
			// (decision 0088)
			need(jkArr, "expected array")
			cnt := at3(i, 1)
			s.ifThen(s.add(OpNe, boolT, nil, cnt, s.cint(int64(len(tu.Elems)))), func() {
				msg := s.call("str.concat_int", st, "", str("expected array of "+itoa(len(tu.Elems))+" elements, got "), cnt)
				s.retv(mkErr(str(""), msg))
			})
			j := s.add(OpAdd, intT, nil, i, s.cint(3))
			var vals []*Value
			for k, et := range tu.Elems {
				if k > 0 {
					j = next(j)
				}
				e := child(et, j, str("["+itoa(k)+"]"))
				vals = append(vals, s.coerce(e, t.Elems[k], at))
			}
			ok(s.add(OpTuple, t, nil, vals...))
			return
		}
		n, isNamed := check.Resolve(ct).(*check.Named)
		if !isNamed {
			b.unsupported(at, "JSON for `"+ct.String()+"`")
			fail("unsupported")
			return
		}
		obj := n.Obj
		if obj.Module != nil && obj.Module.IsPrelude {
			switch obj.Name {
			case "Option":
				s.ifThen(isKind(jkNull), func() { ok(s.add(OpVariant, t, t.VariantIndex("None"))) })
				v := child(n.Args[0], i, str(""))
				ok(s.add(OpVariant, t, t.VariantIndex("Some"), v))
				return
			case "List", "Set":
				need(jkArr, "expected array")
				lt := t
				et := b.typ(n.Args[0], at)
				if obj.Name == "Set" {
					lt = m.List(et)
				}
				lv, jv := local("l"), local("j")
				s.writeVar(lv, s.call("list.new", lt, ""))
				s.writeVar(jv, s.add(OpAdd, intT, nil, i, s.cint(3)))
				s.forUpTo(at3(i, 1), func(k *Value) {
					j := s.read(jv)
					seg := s.call("str.concat", st, "", s.call("str.concat_int", st, "", str("["), k), str("]"))
					e := child(n.Args[0], j, seg)
					s.writeVar(lv, s.call("list.push", lt, "", s.read(lv), s.coerce(e, et, at)))
					s.writeVar(jv, next(j))
				})
				if obj.Name == "List" {
					ok(s.read(lv))
					return
				}
				// a Set: its elements in order
				sv := local("set")
				s.writeVar(sv, s.emptyMap(t))
				l := s.read(lv)
				s.forUpTo(s.call("list.len", intT, "", l), func(k *Value) {
					e := s.call("list.index", et, "", l, k)
					s.writeVar(sv, s.call(b.mapPut(t, at), t, "", s.read(sv), e, s.unit()))
				})
				ok(s.read(sv))
				return
			case "Map":
				need(jkObj, "expected object")
				K := mapKey(t)
				if K.Kind != KStr {
					fail("only Map[Str, V] decodes from JSON")
					return
				}
				mv := local("map")
				s.writeVar(mv, s.emptyMap(t))
				mem := s.jsonRT("json_members", listInt, src, tape, i)
				s.forUpTo(s.call("list.len", intT, "", mem), func(k *Value) {
					s.ifThen(s.add(OpEq, boolT, nil, s.add(OpRem, intT, nil, k, s.cint(2)), s.cint(0)), func() {
						key := jstr(s.call("list.index", intT, "", mem, k))
						vn := s.call("list.index", intT, "", mem, s.add(OpAdd, intT, nil, k, s.cint(1)))
						val := child(n.Args[1], vn, s.call("str.concat", st, "", str("."), key))
						s.writeVar(mv, s.call(b.mapPut(t, at), t, "", s.read(mv), key, s.coerce(val, mapVal(t), at)))
					})
				})
				ok(s.read(mv))
				return
			}
		}
		sb := check.NewSubst(obj.TParams, n.Args)
		if isJSONPatch(ct) {
			s.ifThen(isKind(jkNull), func() { ok(s.add(OpVariant, t, t.VariantIndex("Null"))) })
			vi := t.VariantIndex("Value")
			v := child(n.Args[0], i, str(""))
			ok(s.add(OpVariant, t, vi, s.coerce(v, t.Variants[vi].Fields[0].Type, at)))
			return
		}
		// fields reads the fields of an object at node o into SSA values
		// in the order of fts (checker fields), with seg in front of
		// their paths; defaults apply when withDefaults
		type fieldSpec struct {
			name string
			ct   check.Type
			st   *Type
			dflt ast.Expr
		}
		fields := func(o *Value, seg string, fs []fieldSpec, withDefaults bool) []*Value {
			var out []*Value
			for _, f := range fs {
				j := s.jsonRT("json_find", intT, src, tape, o, str(f.name))
				join := s.newBlock()
				res := join.AddParam(f.st)
				found, missing := s.branch(s.add(OpGe, boolT, nil, j, s.cint(0)))
				s.cur = found
				v := child(f.ct, j, str(seg+"."+f.name))
				s.jump(join, s.coerce(v, f.st, at))
				s.cur = missing
				switch {
				case withDefaults && f.dflt != nil:
					s.jump(join, s.valueAs(f.dflt, f.st))
				case isJSONPatch(f.ct):
					pt := s.unboxType(f.st)
					s.jump(join, s.coerce(s.add(OpVariant, pt, pt.VariantIndex("Absent")), f.st, at))
				case isOption(f.ct):
					pt := s.unboxType(f.st)
					s.jump(join, s.coerce(s.add(OpVariant, pt, pt.VariantIndex("None")), f.st, at))
				default:
					s.retv(mkErr(str(seg), str("missing field `"+f.name+"`")))
				}
				s.seal(join)
				s.cur = join
				out = append(out, res)
			}
			return out
		}
		switch {
		case t.Kind == KStruct && obj.Kind == check.KStruct && !obj.Opaque:
			need(jkObj, "expected object")
			var fs []fieldSpec
			for _, f := range obj.Fields {
				fi := t.FieldIndex(f.Name)
				if fi < 0 {
					continue
				}
				fs = append(fs, fieldSpec{f.Name, sb.Apply(f.Type), t.Fields[fi].Type, f.Default})
			}
			vals := fields(i, "", fs, !obj.Module.IsStd)
			// the struct in SSA field order
			args := make([]*Value, len(t.Fields))
			for k, f := range fs {
				args[t.FieldIndex(f.name)] = vals[k]
			}
			for k := range args {
				if args[k] == nil {
					args[k] = s.zero(t.Fields[k].Type, map[*Type]bool{})
				}
			}
			ok(s.add(OpStruct, t, nil, args...))
			return
		case t.Kind == KEnum && obj.Kind == check.KEnum:
			// the variant name and its payload (std/rt JsonVariant)
			nameV, payV, hasV := local("name"), local("pay"), local("has")
			s.writeVar(nameV, str(""))
			s.writeVar(payV, s.cint(-1))
			s.writeVar(hasV, s.bool(false))
			strB, notStr := s.branch(isKind(jkStr))
			join := s.newBlock()
			s.cur = strB
			s.writeVar(nameV, jstr(i))
			s.jump(join)
			s.cur = notStr
			s.ifThen(s.add(OpNot, boolT, nil, isKind(jkObj)), func() {
				fail("expected a variant name or an object with one variant key")
			})
			s.ifThen(s.add(OpNe, boolT, nil, at3(i, 1), s.cint(1)), func() {
				fail("expected an object with exactly one variant key")
			})
			kn := s.add(OpAdd, intT, nil, i, s.cint(3))
			s.writeVar(nameV, jstr(kn))
			s.writeVar(payV, s.add(OpAdd, intT, nil, kn, s.cint(3)))
			s.writeVar(hasV, s.bool(true))
			s.jump(join)
			s.seal(join)
			s.cur = join
			nm, pay, has := s.read(nameV), s.read(payV), s.read(hasV)
			for _, vo := range obj.Variants {
				vi := t.VariantIndex(vo.Name)
				if vi < 0 {
					continue
				}
				s.ifThen(s.call("str.eq", boolT, "", nm, str(vo.Name)), func() {
					if len(vo.Fields) == 0 {
						ok(s.add(OpVariant, t, vi))
						return
					}
					s.ifThen(s.add(OpNot, boolT, nil, has), func() { fail("variant `" + vo.Name + "` needs fields") })
					vfs := t.Variants[vi].Fields
					if len(vo.Fields) == 1 && (vo.Fields[0].From || vo.Fields[0].Name == "") {
						v := child(sb.Apply(vo.Fields[0].Type), pay, str("."+vo.Name))
						ok(s.add(OpVariant, t, vi, s.coerce(v, vfs[0].Type, at)))
						return
					}
					s.ifThen(s.add(OpNe, boolT, nil, s.call("list.index", intT, "", tape, pay), s.cint(jkObj)), func() {
						s.retv(mkErr(str("."+vo.Name), str("expected object")))
					})
					var fs []fieldSpec
					for fi, f := range vo.Fields {
						fs = append(fs, fieldSpec{f.Name, sb.Apply(f.Type), vfs[fi].Type, nil})
					}
					vals := fields(pay, "."+vo.Name, fs, false)
					ok(s.add(OpVariant, t, vi, vals...))
				})
			}
			s.retv(mkErr(str(""), s.call("str.concat", st, "", s.call("str.concat", st, "", str("unknown variant `"), nm), str("`"))))
			return
		}
		b.unsupported(at, "JSON for `"+ct.String()+"`")
		fail("unsupported")
	})
}

// isOption reports a prelude Option type.
func isOption(ct check.Type) bool {
	_, ok := optionElem(ct)
	return ok
}

// unboxType is t, or the type in a Box.
func (s *fn) unboxType(t *Type) *Type {
	if t.Kind == KBox {
		return t.Elems[0]
	}
	return t
}
