package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Map[K, V] and Set[T] on the native backends (std.md 2.4, decision 0072)
// are SSA structs of lists, so value semantics, reference counting, in
// place updates of unique values, and sharing between tasks come from the
// List machinery:
//
//	keys  List[K]    every entry in insertion order (removed ones too)
//	vals  List[V]    their values (Set: V is ())
//	live  List[Bool] false for a removed entry
//	slots List[Int]  open addressing table (a power of two long, or
//	                 empty): 0 empty, else an entry's index + 1
//	n     Int        live entries
//
// A removed entry keeps its slot (lookups skip it), its key, and its
// value until a rebuild (removing writes only the live flags, so removing
// from a shared map copies no keys or values); when entries and removed
// entries would fill three quarters of the table, put rebuilds it from
// the live entries, which drops the removed ones.
// The operations are functions generated per type (xo.map.*), and keys
// hash with generated functions per type (xo.hash[T]). Iteration and
// keys/values/entries/to_list follow insertion order, as on the Go
// backend.

// mapType returns the struct type of Map[k, v] (set: Set[k], v unit).
func (b *builder) mapType(set bool, k, v *Type) *Type {
	m := b.mod
	name := "Map[" + k.Name + ", " + v.Name + "]"
	if set {
		name = "Set[" + k.Name + "]"
	}
	t, created := m.Named(name, KStruct)
	if created {
		t.Coll = true
		t.Fields = []Field{
			{"keys", m.List(k)}, {"vals", m.List(v)}, {"live", m.List(m.Builtin(KBool))},
			{"slots", m.List(m.Builtin(KInt))}, {"n", m.Builtin(KInt)},
		}
	}
	return t
}

func mapKey(t *Type) *Type { return t.Fields[0].Type.Elems[0] }
func mapVal(t *Type) *Type { return t.Fields[1].Type.Elems[0] }

// holdsColl reports whether a value of t holds a Map or Set (`==` on it is
// not lowered: the struct's fields are not its value).
func holdsColl(t *Type, seen map[*Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if t.Coll {
		return true
	}
	switch t.Kind {
	case KStruct:
		for _, f := range t.Fields {
			if holdsColl(f.Type, seen) {
				return true
			}
		}
	case KEnum:
		for _, v := range t.Variants {
			for _, f := range v.Fields {
				if holdsColl(f.Type, seen) {
					return true
				}
			}
		}
	case KTuple, KList, KBox:
		for _, e := range t.Elems {
			if holdsColl(e, seen) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helpers for generated functions

// local is a fresh mutable local of a generated function.
func local(name string) *check.VarObj { return &check.VarObj{Name: "$" + name} }

// ifThen runs then in a block entered when c holds.
func (s *fn) ifThen(c *Value, then func()) {
	yes, no := s.branch(c)
	join := s.newBlock()
	s.cur = yes
	then()
	if !s.isDead() {
		s.jump(join)
	}
	s.cur = no
	s.jump(join)
	s.seal(join)
	s.cur = join
}

// forUpTo runs body for i in 0..n.
func (s *fn) forUpTo(n *Value, body func(i *Value)) {
	m := s.b.mod
	iv := local("i")
	s.writeVar(iv, s.cint(0))
	head, in, exit := s.newBlock(), s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	i := s.read(iv)
	s.cur.Branch(s.add(OpLt, m.Builtin(KBool), nil, i, n), in, exit)
	s.seal(in)
	s.cur = in
	body(i)
	if !s.isDead() {
		s.writeVar(iv, s.add(OpAdd, m.Builtin(KInt), nil, i, s.cint(1)))
		s.jump(head)
	}
	s.seal(head)
	s.seal(exit)
	s.cur = exit
}

// eqValues is `l == r` (structural; Str through str.eq).
func (s *fn) eqValues(l, r *Value) *Value {
	boolT := s.b.mod.Builtin(KBool)
	if l.Type.Kind == KStr {
		return s.call("str.eq", boolT, "", l, r)
	}
	return s.add(OpEq, boolT, nil, l, r)
}

// eqColl returns xo.eqc[T](a, b) -> Bool: `==` on a type that holds a
// Map or Set (std/rt's Eq): a Map or Set equals another with the same
// live entries in any order; structs, tuples, enums, lists, and boxes
// compare their parts, and parts without a Map or Set compare with `==`.
func (b *builder) eqColl(t *Type) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	return b.genFunc("xo.eqc["+t.Name+"]", []*Type{t, t}, []*Type{boolT}, func(s *fn, ps []*Value) {
		l, r := ps[0], ps[1]
		eq := func(x, y *Value) *Value {
			if holdsColl(x.Type, map[*Type]bool{}) {
				return s.call(b.eqColl(x.Type), boolT, "", x, y)
			}
			return s.eqValues(x, y)
		}
		no := func(c *Value) {
			s.ifThen(s.add(OpNot, boolT, nil, c), func() { s.retv(s.bool(false)) })
		}
		switch {
		case t.Coll:
			K, V := mapKey(t), mapVal(t)
			no(s.add(OpEq, boolT, nil, s.add(OpField, intT, 4, l), s.add(OpField, intT, 4, r)))
			keys := s.add(OpField, t.Fields[0].Type, 0, l)
			vals := s.add(OpField, t.Fields[1].Type, 1, l)
			live := s.add(OpField, t.Fields[2].Type, 2, l)
			rvals := s.add(OpField, t.Fields[1].Type, 1, r)
			find := b.mapFind(t, nil)
			s.forUpTo(s.call("list.len", intT, "", keys), func(i *Value) {
				s.ifThen(s.call("list.index", boolT, "", live, i), func() {
					k := s.call("list.index", K, "", keys, i)
					j := s.call(find, intT, "", r, k, s.call(b.hashFunc(K, nil), intT, "", k))
					no(s.add(OpGe, boolT, nil, j, s.cint(0)))
					if V.Kind != KUnit {
						no(eq(s.call("list.index", V, "", vals, i), s.call("list.index", V, "", rvals, j)))
					}
				})
			})
		case t.Kind == KStruct || t.Kind == KTuple:
			for i := 0; i < numFields(t); i++ {
				ft := fieldType(t, i)
				no(eq(s.add(OpField, ft, i, l), s.add(OpField, ft, i, r)))
			}
		case t.Kind == KBox:
			et := t.Elems[0]
			no(eq(s.add(OpUnbox, et, nil, l), s.add(OpUnbox, et, nil, r)))
		case t.Kind == KList:
			et := t.Elems[0]
			n := s.call("list.len", intT, "", l)
			no(s.add(OpEq, boolT, nil, n, s.call("list.len", intT, "", r)))
			s.forUpTo(n, func(i *Value) {
				no(eq(s.call("list.index", et, "", l, i), s.call("list.index", et, "", r, i)))
			})
		case t.Kind == KEnum:
			tag := s.add(OpTag, intT, nil, l)
			no(s.add(OpEq, boolT, nil, tag, s.add(OpTag, intT, nil, r)))
			for vi, vr := range t.Variants {
				if len(vr.Fields) == 0 {
					continue
				}
				s.ifThen(s.add(OpEq, boolT, nil, tag, s.cint(int64(vi))), func() {
					for fi, fl := range vr.Fields {
						no(eq(s.add(OpPayload, fl.Type, PayloadRef{vi, fi}, l), s.add(OpPayload, fl.Type, PayloadRef{vi, fi}, r)))
					}
				})
			}
		}
		s.retv(s.bool(true))
	})
}

func (s *fn) retv(v *Value) {
	s.add(OpReturn, nil, nil, v)
	s.dead()
}

// genFunc builds the function name once: params and results, body
// filling it.
func (b *builder) genFunc(name string, params, results []*Type, body func(s *fn, ps []*Value)) string {
	if b.mod.Func(name) != nil {
		return name
	}
	f := b.mod.NewFunc(name, params, results)
	s := b.newFn(f, nil, nil)
	body(s, f.Params())
	s.finishFn()
	return name
}

// ---------------------------------------------------------------------------
// Hashing

// hashFunc returns the function that hashes a value of t (an Int); ok
// is false for a type that cannot be a key here (Float, functions,
// handles).
func (b *builder) hashFunc(t *Type, at ast.Node) string {
	m := b.mod
	intT := m.Builtin(KInt)
	name := "xo.hash[" + t.Name + "]"
	if m.Func(name) != nil {
		return name
	}
	switch t.Kind {
	case KFloat, KFunc, KHandle, KCap:
		b.unsupported(at, "a Map key or Set element of type `"+t.Name+"`")
	}
	if t.Coll {
		b.unsupported(at, "a Map key or Set element that holds a Map or Set")
	}
	f := m.NewFunc(name, []*Type{t}, []*Type{intT})
	s := b.newFn(f, nil, nil)
	v := f.Params()[0]
	mix := func(h, x *Value) *Value {
		// h * 31 + x, wrapping (no Pos: unchecked; Wrap: overflow is
		// expected, not undefined)
		return s.add(OpAdd, intT, Wrap, s.add(OpMul, intT, Wrap, h, s.cint(31)), x)
	}
	var h *Value
	switch t.Kind {
	case KInt:
		// a multiplicative mix, then the high bits folded down
		x := s.add(OpMul, intT, Wrap, v, s.cint(-7046029254386353131))
		h = s.add(OpXor, intT, nil, x, s.add(OpShr, intT, nil, x, s.cint(29)))
	case KBool:
		hv := local("h")
		s.writeVar(hv, s.cint(2))
		s.ifThen(v, func() { s.writeVar(hv, s.cint(1)) })
		h = s.read(hv)
	case KStr:
		h = s.rt("str_hash", intT, v)
	case KUnit:
		h = s.cint(7)
	case KStruct, KTuple:
		h = s.cint(17)
		for i := 0; i < numFields(t); i++ {
			ft := fieldType(t, i)
			fv := s.add(OpField, ft, i, v)
			h = mix(h, s.call(b.hashFunc(ft, at), intT, "", fv))
		}
	case KBox:
		h = s.call(b.hashFunc(t.Elems[0], at), intT, "", s.add(OpUnbox, t.Elems[0], nil, v))
	case KList:
		et := t.Elems[0]
		hv := local("h")
		s.writeVar(hv, s.cint(19))
		s.forUpTo(s.call("list.len", intT, "", v), func(i *Value) {
			e := s.call("list.index", et, "", v, i)
			s.writeVar(hv, mix(s.read(hv), s.call(b.hashFunc(et, at), intT, "", e)))
		})
		h = s.read(hv)
	case KEnum:
		hv := local("h")
		tag := s.add(OpTag, intT, nil, v)
		s.writeVar(hv, mix(s.cint(23), tag))
		for vi, vr := range t.Variants {
			if len(vr.Fields) == 0 {
				continue
			}
			s.ifThen(s.add(OpEq, m.Builtin(KBool), nil, tag, s.cint(int64(vi))), func() {
				for fi, fl := range vr.Fields {
					p := s.add(OpPayload, fl.Type, PayloadRef{vi, fi}, v)
					s.writeVar(hv, mix(s.read(hv), s.call(b.hashFunc(fl.Type, at), intT, "", p)))
				}
			})
		}
		h = s.read(hv)
	default:
		h = s.cint(0)
	}
	s.retv(h)
	return name
}

// ---------------------------------------------------------------------------
// Operations

// emptyMap is a Map or Set with no entries.
func (s *fn) emptyMap(t *Type) *Value {
	var fs []*Value
	for i := 0; i < 4; i++ {
		fs = append(fs, s.call("list.new", t.Fields[i].Type, ""))
	}
	fs = append(fs, s.cint(0))
	return s.add(OpStruct, t, nil, fs...)
}

// mapFind returns xo.map.find[T](m, k, h) -> Int: the index of k's live
// entry, or -1.
func (b *builder) mapFind(t *Type, at ast.Node) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	K := mapKey(t)
	return b.genFunc("xo.map.find["+t.Name+"]", []*Type{t, K, intT}, []*Type{intT}, func(s *fn, ps []*Value) {
		mp, k, h := ps[0], ps[1], ps[2]
		slots := s.add(OpField, t.Fields[3].Type, 3, mp)
		n := s.call("list.len", intT, "", slots)
		s.ifThen(s.add(OpEq, boolT, nil, n, s.cint(0)), func() { s.retv(s.cint(-1)) })
		keys := s.add(OpField, t.Fields[0].Type, 0, mp)
		live := s.add(OpField, t.Fields[2].Type, 2, mp)
		mask := s.add(OpSub, intT, nil, n, s.cint(1))
		iv := local("i")
		s.writeVar(iv, s.add(OpAnd, intT, nil, h, mask))
		head := s.newBlock()
		s.jump(head)
		s.cur = head
		i := s.read(iv)
		e := s.call("list.index", intT, "", slots, i)
		s.ifThen(s.add(OpEq, boolT, nil, e, s.cint(0)), func() { s.retv(s.cint(-1)) })
		idx := s.add(OpSub, intT, nil, e, s.cint(1))
		s.ifThen(s.call("list.index", boolT, "", live, idx), func() {
			kk := s.call("list.index", K, "", keys, idx)
			s.ifThen(s.eqValues(kk, k), func() { s.retv(idx) })
		})
		s.writeVar(iv, s.add(OpAnd, intT, nil, s.add(OpAdd, intT, nil, i, s.cint(1)), mask))
		s.jump(head)
		s.seal(head)
		s.dead()
		s.add(OpUnreachable, nil, nil)
	})
}

// insertSlot stores entry index pos (+1) in the first empty slot of h's
// probe sequence.
func (s *fn) insertSlot(slots, h, pos *Value) *Value {
	m := s.b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	mask := s.add(OpSub, intT, nil, s.call("list.len", intT, "", slots), s.cint(1))
	iv, sv := local("i"), local("slots")
	s.writeVar(iv, s.add(OpAnd, intT, nil, h, mask))
	s.writeVar(sv, slots)
	head, found := s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	i := s.read(iv)
	full := s.add(OpNe, boolT, nil, s.call("list.index", intT, "", s.read(sv), i), s.cint(0))
	next := s.newBlock()
	s.cur.Branch(full, next, found)
	s.seal(next)
	s.cur = next
	s.writeVar(iv, s.add(OpAnd, intT, nil, s.add(OpAdd, intT, nil, i, s.cint(1)), mask))
	s.jump(head)
	s.seal(head)
	s.seal(found)
	s.cur = found
	return s.call("list.set", slots.Type, "", s.read(sv), s.read(iv), s.add(OpAdd, intT, nil, pos, s.cint(1)))
}

// mapRebuild returns xo.map.rebuild[T](m, want) -> T: the live entries
// in order, in a table with room for want entries.
func (b *builder) mapRebuild(t *Type, at ast.Node) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	K, V := mapKey(t), mapVal(t)
	return b.genFunc("xo.map.rebuild["+t.Name+"]", []*Type{t, intT}, []*Type{t}, func(s *fn, ps []*Value) {
		mp, want := ps[0], ps[1]
		keys := s.add(OpField, t.Fields[0].Type, 0, mp)
		vals := s.add(OpField, t.Fields[1].Type, 1, mp)
		live := s.add(OpField, t.Fields[2].Type, 2, mp)
		nk, nv, nl := local("keys"), local("vals"), local("live")
		s.writeVar(nk, s.call("list.new", keys.Type, ""))
		s.writeVar(nv, s.call("list.new", vals.Type, ""))
		s.writeVar(nl, s.call("list.new", live.Type, ""))
		s.forUpTo(s.call("list.len", intT, "", keys), func(i *Value) {
			s.ifThen(s.call("list.index", boolT, "", live, i), func() {
				s.writeVar(nk, s.call("list.push", keys.Type, "", s.read(nk), s.call("list.index", K, "", keys, i)))
				s.writeVar(nv, s.call("list.push", vals.Type, "", s.read(nv), s.call("list.index", V, "", vals, i)))
				s.writeVar(nl, s.call("list.push", live.Type, "", s.read(nl), s.bool(true)))
			})
		})
		// the table: a power of two, at least 8, at least twice want
		cv := local("cap")
		s.writeVar(cv, s.cint(8))
		head, grow, done := s.newBlock(), s.newBlock(), s.newBlock()
		s.jump(head)
		s.cur = head
		c := s.read(cv)
		s.cur.Branch(s.add(OpLt, boolT, nil, c, s.add(OpMul, intT, nil, want, s.cint(2))), grow, done)
		s.seal(grow)
		s.cur = grow
		s.writeVar(cv, s.add(OpMul, intT, nil, c, s.cint(2)))
		s.jump(head)
		s.seal(head)
		s.seal(done)
		s.cur = done
		sv := local("slots")
		s.writeVar(sv, s.call("list.new", t.Fields[3].Type, ""))
		s.forUpTo(s.read(cv), func(*Value) {
			s.writeVar(sv, s.call("list.push", t.Fields[3].Type, "", s.read(sv), s.cint(0)))
		})
		keys2 := s.read(nk)
		s.forUpTo(s.call("list.len", intT, "", keys2), func(i *Value) {
			k := s.call("list.index", K, "", keys2, i)
			h := s.call(b.hashFunc(K, at), intT, "", k)
			s.writeVar(sv, s.insertSlot(s.read(sv), h, i))
		})
		n := s.call("list.len", intT, "", keys2)
		s.retv(s.add(OpStruct, t, nil, keys2, s.read(nv), s.read(nl), s.read(sv), n))
	})
}

// mapPut returns xo.map.put[T](m, k, v) -> T: k's value replaced (its
// place kept), or a new entry at the end.
func (b *builder) mapPut(t *Type, at ast.Node) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	K, V := mapKey(t), mapVal(t)
	find, rebuild := b.mapFind(t, at), b.mapRebuild(t, at)
	hash := b.hashFunc(K, at)
	return b.genFunc("xo.map.put["+t.Name+"]", []*Type{t, K, V}, []*Type{t}, func(s *fn, ps []*Value) {
		mp, k, v := ps[0], ps[1], ps[2]
		h := s.call(hash, intT, "", k)
		idx := s.call(find, intT, "", mp, k, h)
		s.ifThen(s.add(OpGe, boolT, nil, idx, s.cint(0)), func() {
			vals := s.call("list.set", t.Fields[1].Type, "", s.add(OpField, t.Fields[1].Type, 1, mp), idx, v)
			s.retv(s.add(OpSetField, t, 1, mp, vals))
		})
		mv := local("m")
		s.writeVar(mv, mp)
		total := s.call("list.len", intT, "", s.add(OpField, t.Fields[0].Type, 0, mp))
		tcap := s.call("list.len", intT, "", s.add(OpField, t.Fields[3].Type, 3, mp))
		full := s.add(OpGt, boolT, nil, s.add(OpMul, intT, nil, s.add(OpAdd, intT, nil, total, s.cint(1)), s.cint(4)), s.add(OpMul, intT, nil, tcap, s.cint(3)))
		s.ifThen(full, func() {
			n := s.add(OpField, intT, 4, mp)
			s.writeVar(mv, s.call(rebuild, t, "", mp, s.add(OpAdd, intT, nil, n, s.cint(1))))
		})
		cur := s.read(mv)
		keys := s.add(OpField, t.Fields[0].Type, 0, cur)
		vals := s.add(OpField, t.Fields[1].Type, 1, cur)
		live := s.add(OpField, t.Fields[2].Type, 2, cur)
		slots := s.add(OpField, t.Fields[3].Type, 3, cur)
		n := s.add(OpField, intT, 4, cur)
		pos := s.call("list.len", intT, "", keys)
		keys = s.call("list.push", keys.Type, "", keys, k)
		vals = s.call("list.push", vals.Type, "", vals, v)
		live = s.call("list.push", live.Type, "", live, s.bool(true))
		slots = s.insertSlot(slots, h, pos)
		s.retv(s.add(OpStruct, t, nil, keys, vals, live, slots, s.add(OpAdd, intT, nil, n, s.cint(1))))
	})
}

// mapRemove returns xo.map.remove[T](m, k) -> (T, V?).
func (b *builder) mapRemove(t *Type, ot *Type, at ast.Node) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	K, V := mapKey(t), mapVal(t)
	find := b.mapFind(t, at)
	rt := m.Tuple(t, ot)
	return b.genFunc("xo.map.remove["+t.Name+"]", []*Type{t, K}, []*Type{rt}, func(s *fn, ps []*Value) {
		mp, k := ps[0], ps[1]
		idx := s.call(find, intT, "", mp, k, s.call(b.hashFunc(K, at), intT, "", k))
		s.ifThen(s.add(OpLt, boolT, nil, idx, s.cint(0)), func() {
			s.retv(s.add(OpTuple, rt, nil, mp, s.add(OpVariant, ot, ot.VariantIndex("None"))))
		})
		old := s.call("list.index", V, "", s.add(OpField, t.Fields[1].Type, 1, mp), idx)
		// only the entry's live flag changes: its key and value stay (dead)
		// until a rebuild drops them, so removing from a shared map copies
		// the flags, not the keys and values with a count update of each
		// (Mutex.with keeps the old map while a closure may still fail)
		keys := s.add(OpField, t.Fields[0].Type, 0, mp)
		vals := s.add(OpField, t.Fields[1].Type, 1, mp)
		live := s.call("list.set", t.Fields[2].Type, "", s.add(OpField, t.Fields[2].Type, 2, mp), idx, s.bool(false))
		n := s.add(OpSub, intT, nil, s.add(OpField, intT, 4, mp), s.cint(1))
		slots := s.add(OpField, t.Fields[3].Type, 3, mp)
		nm := s.add(OpStruct, t, nil, keys, vals, live, slots, n)
		s.retv(s.add(OpTuple, rt, nil, nm, s.add(OpVariant, ot, ot.VariantIndex("Some"), old)))
	})
}

// mapList returns the function that lists the live entries' keys (what
// 0), values (1), or (key, value) tuples (2) in order.
func (b *builder) mapList(t *Type, what int) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	K, V := mapKey(t), mapVal(t)
	et := []*Type{K, V, m.Tuple(K, V)}[what]
	lt := m.List(et)
	name := []string{"xo.map.keys[", "xo.map.values[", "xo.map.entries["}[what] + t.Name + "]"
	return b.genFunc(name, []*Type{t}, []*Type{lt}, func(s *fn, ps []*Value) {
		mp := ps[0]
		keys := s.add(OpField, t.Fields[0].Type, 0, mp)
		vals := s.add(OpField, t.Fields[1].Type, 1, mp)
		live := s.add(OpField, t.Fields[2].Type, 2, mp)
		out := local("out")
		// sized to the live entries: no regrowth while copying
		s.writeVar(out, s.call("list.new", lt, "", s.add(OpField, intT, 4, mp)))
		s.forUpTo(s.call("list.len", intT, "", keys), func(i *Value) {
			s.ifThen(s.call("list.index", boolT, "", live, i), func() {
				var e *Value
				switch what {
				case 0:
					e = s.call("list.index", K, "", keys, i)
				case 1:
					e = s.call("list.index", V, "", vals, i)
				default:
					e = s.add(OpTuple, et, nil, s.call("list.index", K, "", keys, i), s.call("list.index", V, "", vals, i))
				}
				s.writeVar(out, s.call("list.push", lt, "", s.read(out), e))
			})
		})
		s.retv(s.read(out))
	})
}

// ---------------------------------------------------------------------------
// Lowering

// mapLit lowers `{k: v, ...}` and `#{x, ...}`.
func (s *fn) mapLit(t *Type, keys, vals []ast.Expr, at ast.Node) *Value {
	if !t.Coll {
		s.b.unsupported(at, "this literal")
		return s.undef(t)
	}
	put := s.b.mapPut(t, at)
	v := s.emptyMap(t)
	for i, k := range keys {
		kv := s.valueAs(k, mapKey(t))
		var vv *Value
		if vals != nil {
			vv = s.valueAs(vals[i], mapVal(t))
		} else {
			vv = s.unit()
		}
		v = s.call(put, t, "", v, kv, vv)
	}
	return v
}

// mapCall lowers a method of Map or Set; ok is false for one it does not
// lower.
func (s *fn) mapCall(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	nt := s.natOf(x)
	args := argExprs(fo.Params, x)
	key := fo.Recv.Name + "." + fo.Name
	if fo.RecvVar {
		p, ok := s.place(ci.Recv)
		if !ok || !p.t.Coll {
			return nil, false
		}
		t := p.t
		switch key {
		case "Map.put", "Set.add":
			k := s.valueAs(args[0], mapKey(t))
			var v *Value
			if key == "Map.put" {
				v = s.valueAs(args[1], mapVal(t))
			} else {
				v = s.unit()
			}
			p.set(s.call(b.mapPut(t, x), t, "", p.get(), k, v))
			return s.unit(), true
		case "Map.remove", "Set.remove":
			ot := nt
			if key == "Set.remove" {
				ot = b.typ(b.preludeType("Option", &check.Tuple{}), x)
			}
			k := s.valueAs(args[0], mapKey(t))
			r := s.call(b.mapRemove(t, ot, x), m.Tuple(t, ot), "", p.get(), k)
			p.set(s.add(OpField, t, 0, r))
			o := s.add(OpField, ot, 1, r)
			if key == "Set.remove" {
				return s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, o), s.cint(int64(ot.VariantIndex("Some")))), true
			}
			return o, true
		case "Map.update":
			return s.mapUpdate(x, p, args), true
		}
		return nil, false
	}
	mv := s.expr(ci.Recv)
	t := mv.Type
	if !t.Coll {
		return nil, false
	}
	find := func(k *Value) *Value {
		return s.call(b.mapFind(t, x), intT, "", mv, k, s.call(b.hashFunc(mapKey(t), x), intT, "", k))
	}
	switch key {
	case "Map.len", "Set.len":
		return s.add(OpField, intT, 4, mv), true
	case "Map.is_empty", "Set.is_empty":
		return s.add(OpEq, boolT, nil, s.add(OpField, intT, 4, mv), s.cint(0)), true
	case "Map.contains", "Set.contains":
		return s.add(OpGe, boolT, nil, find(s.valueAs(args[0], mapKey(t))), s.cint(0)), true
	case "Map.get":
		idx := find(s.valueAs(args[0], mapKey(t)))
		join := s.newBlock()
		res := join.AddParam(nt)
		yes, no := s.branch(s.add(OpGe, boolT, nil, idx, s.cint(0)))
		s.cur = yes
		v := s.call("list.index", mapVal(t), "", s.add(OpField, t.Fields[1].Type, 1, mv), idx)
		s.jump(join, s.variant(nt, "Some", []*Value{v}, x))
		s.cur = no
		s.jump(join, s.variant(nt, "None", nil, x))
		s.seal(join)
		s.cur = join
		return res, true
	case "Map.get_or_fault":
		k := s.valueAs(args[0], mapKey(t))
		reason := s.valueAs(args[1], m.Builtin(KStr))
		idx := find(k)
		bad, ok := s.branch(s.add(OpLt, boolT, nil, idx, s.cint(0)))
		s.cur = bad
		var shown *Value
		switch mapKey(t).Kind {
		case KStr:
			shown = s.call("list.index", m.Builtin(KStr), "", s.rt("str_quote", m.List(m.Builtin(KStr)), k), s.cint(0))
		case KInt:
			shown = s.call("str.of_int", m.Builtin(KStr), "", k)
		default:
			b.unsupported(x, "`get_or_fault` on a Map whose keys are not Str or Int")
			shown = s.add(OpConst, m.Builtin(KStr), "?")
		}
		msg := s.call("str.concat", m.Builtin(KStr), "", s.add(OpConst, m.Builtin(KStr), "key "), shown)
		msg = s.call("str.concat", m.Builtin(KStr), "", msg, s.add(OpConst, m.Builtin(KStr), " not found in Map of length "))
		msg = s.call("str.concat_int", m.Builtin(KStr), "", msg, s.add(OpField, intT, 4, mv))
		msg = s.call("str.concat", m.Builtin(KStr), "", msg, s.add(OpConst, m.Builtin(KStr), ": "))
		msg = s.call("str.concat", m.Builtin(KStr), "", msg, reason)
		s.fault(msg, b.posLine(x))
		s.cur = ok
		return s.call("list.index", mapVal(t), "", s.add(OpField, t.Fields[1].Type, 1, mv), idx), true
	case "Map.keys", "Set.to_list":
		return s.call(b.mapList(t, 0), m.List(mapKey(t)), "", mv), true
	case "Map.values":
		return s.call(b.mapList(t, 1), m.List(mapVal(t)), "", mv), true
	case "Map.entries":
		return s.call(b.mapList(t, 2), m.List(m.Tuple(mapKey(t), mapVal(t))), "", mv), true
	}
	b.unsupported(x, "the method `"+fo.DisplayName()+"`")
	return s.undef(nt), true
}

// mapUpdate lowers `m.update(k, init, f)`: f gets k's value (or init) as
// a `var` parameter; its new value is stored when f succeeds, and f's
// result is the call's.
func (s *fn) mapUpdate(x *ast.CallExpr, p place, args []ast.Expr) *Value {
	b := s.b
	m := b.mod
	t := p.t
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	nt := s.natOf(x)
	V := mapVal(t)
	fv, packed := s.withClosure(args[2], V)
	if fv == nil {
		return s.undef(nt)
	}
	if !packed {
		b.unsupported(x, "`update` with a closure whose parameter is not `var`")
		return s.undef(nt)
	}
	mp := p.get()
	k := s.valueAs(args[0], mapKey(t))
	init := s.valueAs(args[1], V)
	idx := s.call(b.mapFind(t, x), intT, "", mp, k, s.call(b.hashFunc(mapKey(t), x), intT, "", k))
	cur := local("v")
	s.writeVar(cur, init)
	s.ifThen(s.add(OpGe, boolT, nil, idx, s.cint(0)), func() {
		s.writeVar(cur, s.call("list.index", V, "", s.add(OpField, t.Fields[1].Type, 1, mp), idx))
	})
	r := s.add(OpCallValue, fv.Type.Ret, nil, fv, s.read(cur))
	s.check("")
	res := s.add(OpField, r.Type.Elems[0], 0, r)
	nv := s.add(OpField, V, 1, r)
	store := func() { p.set(s.call(b.mapPut(t, x), t, "", mp, k, nv)) }
	rt := res.Type
	if rt.Kind == KEnum && rt.VariantIndex("Ok") >= 0 && rt.VariantIndex("Err") >= 0 && failErr(s.withErr(args[2])) != nil {
		s.ifThen(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, res), s.cint(int64(rt.VariantIndex("Ok")))), store)
		return res
	}
	store()
	return res
}

// forMap lowers `for (k, v) in m` and `for x in set`: the live entries in
// insertion order, of the value the map had when the loop started.
func (s *fn) forMap(x *ast.ForStmt, label string, mv *Value) {
	b := s.b
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	t := mv.Type
	keys := s.add(OpField, t.Fields[0].Type, 0, mv)
	vals := s.add(OpField, t.Fields[1].Type, 1, mv)
	live := s.add(OpField, t.Fields[2].Type, 2, mv)
	set := len(t.Name) > 4 && t.Name[:4] == "Set["
	n := s.call("list.len", intT, "", keys)
	idx := local("i")
	s.writeVar(idx, s.cint(0))
	head, test, body, latch, exit := s.newBlock(), s.newBlock(), s.newBlock(), s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	i := s.read(idx)
	s.cur.Branch(s.add(OpLt, boolT, nil, i, n), test, exit)
	s.seal(test)
	s.cur = test
	s.cur.Branch(s.call("list.index", boolT, "", live, i), body, latch)
	s.seal(body)
	s.cur = body
	var elem *Value
	k := s.call("list.index", mapKey(t), "", keys, i)
	if set {
		elem = k
	} else {
		elem = s.add(OpTuple, m.Tuple(mapKey(t), mapVal(t)), nil, k, s.call("list.index", mapVal(t), "", vals, i))
	}
	fail := s.newBlock()
	s.pattern(elem, x.Pattern, fail)
	cont := s.cur
	s.cur = fail
	s.seal(fail)
	s.add(OpUnreachable, nil, nil)
	s.cur = cont
	s.loops = append(s.loops, &loopCtx{label: label, brk: exit, cont: latch, scopes: len(s.scopes), exits: len(s.exits)})
	s.block(x.Body)
	s.loops = s.loops[:len(s.loops)-1]
	s.jump(latch)
	s.seal(latch)
	s.cur = latch
	s.writeVar(idx, s.add(OpAdd, intT, nil, s.read(idx), s.cint(1)))
	s.jump(head)
	s.seal(head)
	s.seal(exit)
	s.cur = exit
}
