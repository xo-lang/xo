package ssa

import (
	"math"
	"strconv"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// Sized integers, Rune, byte strings, and slicing (decision 0081).
//
// A sized integer (Int8 to UInt64) and a Rune are KInt types held in the
// 64 bit word, sign or zero extended (a UInt64 above the Int range is a
// negative word). Operations that cannot leave the type's range are the
// Int operations: moves, `&`, `|`, `^`, and the order of narrow values.
// The others are calls of the C runtime, which checks the range and
// faults with std/rt's messages: arithmetic and shifts (rt.int_op), and
// conversions that can fail (rt.int_conv, rt.rune_of). A UInt64 is
// ordered by flipping the sign bit of both operands. Widening conversions
// are OpIntConv, which is no code.

// intOpCodes are the operation codes of rt.int_op.
var intOpCodes = map[token.Kind]int64{
	token.Plus: 0, token.Minus: 1, token.Star: 2, token.Slash: 3, token.Percent: 4, token.Shl: 5, token.Shr: 6,
}

// intOpNeg is the code of unary minus in rt.int_op.
const intOpNeg = 7

// sizeArgs are the type operands of the runtime calls on sized
// integers: the bits and whether the type is unsigned.
func (s *fn) sizeArgs(t *Type) []*Value {
	u := int64(0)
	if t.Unsigned {
		u = 1
	}
	bits := int64(t.Bits)
	if bits == 0 {
		bits = 64
	}
	return []*Value{s.cint(bits), s.cint(u)}
}

// sizedArith is `l op r` on a sized integer type.
func (s *fn) sizedArith(op token.Kind, l, r *Value, at ast.Node) *Value {
	t := l.Type
	switch op {
	case token.Amp:
		return s.add(OpAnd, t, nil, l, r)
	case token.Pipe:
		return s.add(OpOr, t, nil, l, r)
	case token.Caret:
		return s.add(OpXor, t, nil, l, r)
	}
	code, ok := intOpCodes[op]
	if !ok {
		s.b.unsupported(at, "this operator")
		return s.undef(t)
	}
	if s.sizedInline() {
		return s.sizedOp(code, l, r, at)
	}
	args := append([]*Value{s.cint(code), l, r}, s.sizeArgs(t)...)
	return s.rt("int_op", t, append(args, s.posStr(at))...)
}

// sizedNeg is `-v` on a sized integer type.
func (s *fn) sizedNeg(v *Value, at ast.Node) *Value {
	if s.sizedInline() {
		return s.sizedOp(intOpNeg, v, s.add(OpConst, v.Type, int64(0)), at)
	}
	args := append([]*Value{s.cint(intOpNeg), v, s.cint(0)}, s.sizeArgs(v.Type)...)
	return s.rt("int_op", v.Type, append(args, s.posStr(at))...)
}

// sizedInline reports whether sized integer operations are lowered
// inline, their faults in Xo (std/native/llrt/ints.xo, decisions 0193 and
// 0195): with the native prelude, unless XO_LLRT_C names ints (then
// rt.int_op, rt.int_conv, and rt.rune_of, the C, as before the port).
func (s *fn) sizedInline() bool {
	return s.b.preludeImpl("llrt_int_op_fault") != nil && !check.LLRTUsesC("ints")
}

// llrtCall calls the native prelude's llrt_<name> (a function of
// std/native/llrt) with args: its result (unit when it returns Never;
// the caller ends the block).
func (s *fn) llrtCall(name string, at ast.Node, args ...*Value) *Value {
	b := s.b
	f := b.declare(b.preludeImpl("llrt_"+name), nil, at)
	t := b.mod.Builtin(KUnit)
	if len(f.Results) > 0 {
		t = f.Results[0]
	}
	return s.call(f.Name, t, "", args...)
}

// constWord is the value of v when it is an integer constant (through
// conversions).
func constWord(v *Value) (int64, bool) {
	for v.Op == OpIntConv {
		v = v.Args[0]
	}
	if v.Op != OpConst {
		return 0, false
	}
	k, ok := v.Aux.(int64)
	return k, ok
}

// word is the integer v as an Int (the same word).
func (s *fn) word(v *Value) *Value {
	if it := s.b.mod.Builtin(KInt); v.Type != it {
		return s.add(OpIntConv, it, nil, v)
	}
	return v
}

// ult is `x < y` on two Int words as UInt64s.
func (s *fn) ult(x, y *Value) *Value {
	intT := s.b.mod.Builtin(KInt)
	fx := s.add(OpXor, intT, nil, x, s.cint(math.MinInt64))
	fy := s.add(OpXor, intT, nil, y, s.cint(math.MinInt64))
	return s.add(OpLt, s.b.mod.Builtin(KBool), nil, fx, fy)
}

// outOf reports whether the Int word r is not a value of the narrow
// integer type t: r >> bits is not 0 (unsigned), or r + 2^(bits-1) >>
// bits is not 0 (signed).
func (s *fn) outOf(r *Value, t *Type) *Value {
	intT, boolT := s.b.mod.Builtin(KInt), s.b.mod.Builtin(KBool)
	if !t.Unsigned {
		r = s.add(OpAdd, intT, nil, r, s.cint(int64(1)<<(t.Bits-1)))
	}
	return s.add(OpNe, boolT, nil, s.add(OpShr, intT, nil, r, s.cint(int64(t.Bits))), s.cint(0))
}

// sizedFailIf faults when bad holds, with the fault of `l op r` (code as
// intOpCodes, or intOpNeg) on l's type at at (ints.op_fault).
func (s *fn) sizedFailIf(bad *Value, code int64, l, r *Value, at ast.Node) {
	s.ifThen(bad, func() {
		args := append([]*Value{s.cint(code), s.word(l), s.word(r)}, s.sizeArgs(l.Type)...)
		s.llrtCall("int_op_fault", at, append(args, s.posStr(at))...)
		s.add(OpUnreachable, nil, nil)
		s.dead()
	})
}

// sizedOp is `l op r` (code as intOpCodes; intOpNeg: -l) on a sized
// integer type, inline: the word operation and the range check that
// xo_int_op made (std/rt's AddS, AddU, ..., Shl, Shr, NegS, NegU). A
// narrow type's exact result fits the word (operands below 2^32, shift
// amounts below 32), except an unsigned 32 bit product, which wraps and
// is then out of range. A UInt64 checks the carry, the borrow, the high
// word of the product, and the shift back; its division is ints.xo's.
func (s *fn) sizedOp(code int64, l, r *Value, at ast.Node) *Value {
	m := s.b.mod
	t := l.Type
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	u64 := t.U64()
	a, b := s.word(l), s.word(r)
	kb, bconst := constWord(r)
	wrap := func(o Op) *Value { return s.add(o, intT, Wrap, a, b) }
	cmp0 := func(o Op, v *Value) *Value { return s.add(o, boolT, nil, v, s.cint(0)) }
	var res *Value
	switch code {
	case 0, 1: // + -
		o := OpAdd
		if code == 1 {
			o = OpSub
		}
		if u64 {
			res = wrap(o)
			if code == 0 {
				s.sizedFailIf(s.ult(res, a), code, l, r, at)
			} else {
				s.sizedFailIf(s.ult(a, b), code, l, r, at)
			}
			break
		}
		res = s.add(o, intT, nil, a, b)
		s.sizedFailIf(s.outOf(res, t), code, l, r, at)
	case 2: // *
		if u64 {
			s.sizedFailIf(cmp0(OpNe, s.rt("u64_mul_hi", intT, a, b)), code, l, r, at)
			res = wrap(OpMul)
			break
		}
		if t.Unsigned {
			// a UInt32 product can pass 2^63: it wraps, and is out of range
			res = wrap(OpMul)
		} else {
			res = s.add(OpMul, intT, nil, a, b)
		}
		s.sizedFailIf(s.outOf(res, t), code, l, r, at)
	case 3, 4: // / %
		if !bconst || kb == 0 {
			s.sizedFailIf(cmp0(OpEq, b), code, l, r, at)
		}
		if u64 {
			name := "u64_div"
			if code == 4 {
				name = "u64_rem"
			}
			res = s.llrtCall(name, at, a, b)
			break
		}
		if code == 4 {
			res = s.add(OpRem, intT, nil, a, b)
			break
		}
		res = s.add(OpDiv, intT, nil, a, b)
		if !t.Unsigned && (!bconst || kb == -1) {
			// the minimum over -1
			s.sizedFailIf(s.outOf(res, t), code, l, r, at)
		}
	case 5, 6: // << >>
		bits := int64(t.Bits)
		if u64 {
			bits = 64
		}
		if !bconst || kb < 0 || kb >= bits {
			bad := s.add(OpOr, boolT, nil, cmp0(OpLt, b), s.add(OpGe, boolT, nil, b, s.cint(bits)))
			s.sizedFailIf(bad, code, l, r, at)
		}
		if code == 6 {
			if u64 {
				res = s.rt("int_wrap", intT, s.cint(4), a, b)
			} else {
				// a narrow unsigned value is not negative
				res = s.add(OpShr, intT, nil, a, b)
			}
			break
		}
		res = s.add(OpShl, intT, nil, a, b)
		if u64 {
			back := s.rt("int_wrap", intT, s.cint(4), res, b)
			s.sizedFailIf(s.add(OpNe, boolT, nil, back, a), code, l, r, at)
			break
		}
		s.sizedFailIf(s.outOf(res, t), code, l, r, at)
	case intOpNeg:
		if t.Unsigned {
			// only 0 negates
			s.sizedFailIf(cmp0(OpNe, a), code, l, r, at)
			res = s.cint(0)
			break
		}
		res = s.add(OpSub, intT, nil, s.cint(0), a)
		s.sizedFailIf(s.outOf(res, t), code, l, r, at)
	}
	return s.add(OpIntConv, t, nil, res)
}

// inRange is whether the Int word v is a value of the integer type to
// (`Int32.try`, xo_int_in_range): a UInt64 is not negative.
func (s *fn) inRange(v *Value, to *Type) *Value {
	boolT := s.b.mod.Builtin(KBool)
	if to.U64() {
		return s.add(OpGe, boolT, nil, v, s.cint(0))
	}
	return s.add(OpNot, boolT, nil, s.outOf(v, to))
}

// runeValid is whether the Int word v is a Unicode scalar value
// (`Rune.try`, xo_rune_valid): 0 to 0x10FFFF but not 0xD800 to 0xDFFF
// (v >> 11 is 27).
func (s *fn) runeValid(v *Value) *Value {
	intT, boolT := s.b.mod.Builtin(KInt), s.b.mod.Builtin(KBool)
	neg := s.add(OpLt, boolT, nil, v, s.cint(0))
	big := s.add(OpGt, boolT, nil, v, s.cint(0x10FFFF))
	sur := s.add(OpEq, boolT, nil, s.add(OpShr, intT, nil, v, s.cint(11)), s.cint(27))
	return s.add(OpNot, boolT, nil, s.add(OpOr, boolT, nil, s.add(OpOr, boolT, nil, neg, big), sur))
}

// sizedNot is `~v` on a sized integer type: an unsigned narrow value
// keeps its high bits clear.
func (s *fn) sizedNot(v *Value) *Value {
	t := v.Type
	if t.Unsigned && t.Bits < 64 {
		return s.add(OpXor, t, nil, v, s.add(OpConst, t, int64(1)<<t.Bits-1))
	}
	return s.add(OpNot, t, nil, v)
}

// posStr is the `file:line:col` of n as a Str constant.
func (s *fn) posStr(n ast.Node) *Value {
	return s.add(OpConst, s.b.mod.Builtin(KStr), s.b.pos(n))
}

// compare is `l op r` on two values of one scalar type (Int, a sized
// integer, Float, Bool): a UInt64 is ordered as unsigned.
func (s *fn) compare(o Op, l, r *Value) *Value {
	boolT := s.b.mod.Builtin(KBool)
	if l.Type.U64() && o != OpEq && o != OpNe {
		flip := s.add(OpConst, l.Type, int64(math.MinInt64))
		l = s.add(OpXor, l.Type, nil, l, flip)
		r = s.add(OpXor, l.Type, nil, r, flip)
	}
	return s.add(o, boolT, nil, l, r)
}

// intRange is the range of the integer type t as (lo, hi) for the
// widening test; a UInt64 is [0, 2^64), reported with hi = -1.
func intRange(t *Type) (lo, hi int64, u64 bool) {
	switch {
	case t.Bits == 0:
		return math.MinInt64, math.MaxInt64, false
	case t.U64():
		return 0, -1, true
	case t.Unsigned:
		return 0, int64(1)<<t.Bits - 1, false
	}
	return -(int64(1) << (t.Bits - 1)), int64(1)<<(t.Bits-1) - 1, false
}

// widens reports whether every value of from is a value of to.
func widens(from, to *Type) bool {
	flo, fhi, fu := intRange(from)
	tlo, thi, tu := intRange(to)
	switch {
	case tu:
		return flo >= 0
	case fu:
		return false
	}
	return flo >= tlo && fhi <= thi
}

// intConv converts the integer v to the integer type to (`T(x)`,
// core 2.1): no code when it widens, else a checked runtime call. A Rune
// result must be a Unicode scalar value.
func (s *fn) intConv(v *Value, to *Type, at ast.Node) *Value {
	if v.Type == to {
		return v
	}
	if to.Name == "Rune" {
		if from := v.Type; from.Unsigned && from.Bits == 8 {
			// UInt8 is always a scalar value
			return s.add(OpIntConv, to, nil, v)
		}
		if s.sizedInline() {
			// a UInt64 at 2^63 or above is a negative word: not valid
			w := s.word(v)
			s.convFailIf(s.add(OpNot, s.b.mod.Builtin(KBool), nil, s.runeValid(w)), v, nil, at)
			return s.add(OpIntConv, to, nil, w)
		}
		args := append([]*Value{v}, s.sizeArgs(v.Type)...)
		return s.rt("rune_of", to, append(args, s.posStr(at))...)
	}
	if widens(v.Type, to) {
		return s.add(OpIntConv, to, nil, v)
	}
	if s.sizedInline() {
		w := s.word(v)
		boolT := s.b.mod.Builtin(KBool)
		var bad *Value
		switch {
		case to.U64(), to.Bits == 0:
			// a UInt64 from a signed type, or an Int from a UInt64
			bad = s.add(OpLt, boolT, nil, w, s.cint(0))
		default:
			bad = s.outOf(w, to)
			if v.Type.U64() && !to.Unsigned {
				// a UInt64 at 2^63 or above (a negative word) fits no
				// signed type
				bad = s.add(OpOr, boolT, nil, s.add(OpLt, boolT, nil, w, s.cint(0)), bad)
			}
		}
		s.convFailIf(bad, v, to, at)
		return s.add(OpIntConv, to, nil, w)
	}
	args := append(append([]*Value{v}, s.sizeArgs(v.Type)...), s.sizeArgs(to)...)
	return s.rt("int_conv", to, append(args, s.posStr(at))...)
}

// convFailIf faults when bad holds, with the fault of converting v to
// the integer type to (nil: Rune) at at (ints.conv_fault).
func (s *fn) convFailIf(bad, v *Value, to *Type, at ast.Node) {
	s.ifThen(bad, func() {
		args := append([]*Value{s.word(v)}, s.sizeArgs(v.Type)...)
		if to == nil {
			args = append(args, s.cint(0), s.cint(0))
		} else {
			args = append(args, s.sizeArgs(to)...)
		}
		s.llrtCall("int_conv_fault", at, append(args, s.posStr(at))...)
		s.add(OpUnreachable, nil, nil)
		s.dead()
	})
}

// intLit is the constant of an integer literal (decimal digits) of type t.
func (s *fn) intLit(x *ast.IntLit, t *Type) *Value {
	if t.U64() {
		u, err := strconv.ParseUint(x.Value, 10, 64)
		if err != nil {
			s.b.unsupported(x, "this UInt64 literal")
		}
		return s.add(OpConst, t, int64(u))
	}
	n, err := strconv.ParseInt(x.Value, 10, 64)
	if err != nil {
		s.b.unsupported(x, "this Int literal")
	}
	return s.add(OpConst, t, n)
}

// slice lowers `x[lo..hi]` (and `..=`, and the open forms) on Str, Bytes,
// and List (core 3.6): a new value, faulting out of range as std/rt.
func (s *fn) slice(x *ast.IndexExpr, r *ast.RangeExpr) *Value {
	b := s.b
	m := b.mod
	intT := m.Builtin(KInt)
	base := s.expr(x.X)
	var lo, hi *Value
	if r.Lo != nil {
		lo = s.valueAs(r.Lo, intT)
	} else {
		lo = s.cint(0)
	}
	if r.Hi != nil {
		hi = s.valueAs(r.Hi, intT)
		if r.Inclusive {
			hi = s.add(OpAdd, intT, nil, hi, s.cint(1))
			hi.Pos = b.pos(x)
		}
	}
	length := func() *Value {
		if base.Type.Kind == KList {
			return s.call("list.len", intT, "", base)
		}
		return s.call("Str.len", intT, "", base)
	}
	if hi == nil {
		hi = length()
	}
	switch base.Type.Kind {
	case KStr:
		name := "str_slice_at"
		if isPreludeNamed(s.natCheck(x.X), "Bytes") {
			name = "bytes_slice_at"
		}
		return s.rt(name, base.Type, base, lo, hi, s.posStr(x))
	case KList:
		return s.call(b.listSlice(base.Type), base.Type, "", base, lo, hi, s.posStr(x))
	}
	b.unsupported(x, "slicing a value of type `"+base.Type.Name+"`")
	return s.undef(base.Type)
}

// listSlice returns xo.slice[List[T]](l, lo, hi, at): the elements lo to
// hi of l in a new list, faulting at at when the range is not in l.
func (b *builder) listSlice(lt *Type) string {
	m := b.mod
	intT, boolT, strT := m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr)
	return b.genFunc("xo.slice["+lt.Name+"]", []*Type{lt, intT, intT, strT}, []*Type{lt}, func(s *fn, ps []*Value) {
		l, lo, hi, at := ps[0], ps[1], ps[2], ps[3]
		n := s.call("list.len", intT, "", l)
		bad := s.add(OpOr, boolT, nil, s.add(OpLt, boolT, nil, lo, s.cint(0)),
			s.add(OpOr, boolT, nil, s.add(OpGt, boolT, nil, hi, n), s.add(OpGt, boolT, nil, lo, hi)))
		s.ifThen(bad, func() {
			msg := s.call("str.concat_int", strT, "", s.add(OpConst, strT, "slice "), lo)
			msg = s.call("str.concat", strT, "", msg, s.add(OpConst, strT, ".."))
			msg = s.call("str.concat_int", strT, "", msg, hi)
			msg = s.call("str.concat", strT, "", msg, s.add(OpConst, strT, " out of range for List of length "))
			msg = s.call("str.concat_int", strT, "", msg, n)
			s.rt("fault_at", nil, msg, at)
			s.add(OpUnreachable, nil, nil)
			s.dead()
		})
		out := local("out")
		s.writeVar(out, s.call("list.new", lt, ""))
		s.forUpTo(s.add(OpSub, intT, nil, hi, lo), func(i *Value) {
			e := s.call("list.index", lt.Elems[0], "", l, s.add(OpAdd, intT, nil, lo, i))
			s.writeVar(out, s.call("list.push", lt, "", s.read(out), e))
		})
		s.retv(s.read(out))
	})
}

// maxConsts is the number of constant slots of the runtime
// (std/llrt/xo_bytes.c).
const maxConsts = 4096

// constValue reads the constant c as a value of type t. A constant of a
// heap type other than Str (a List, Map, or struct literal) is built once
// per run, by an entry function the runtime calls on first use, and kept
// (immortal) in a runtime slot, as the Go backend keeps it in a package
// variable; others are built where they are read.
func (s *fn) constValue(c *check.ConstObj, t *Type) *Value {
	b := s.b
	if !t.Heap() || t.Kind == KStr {
		return s.valueAs(c.Decl.Value, t)
	}
	m := b.mod
	intT := m.Builtin(KInt)
	lt := m.List(t)
	if b.consts == nil {
		b.consts = map[*check.ConstObj]int{}
	}
	id, ok := b.consts[c]
	name := "xo.const[" + c.Module.Name + "." + c.Name + "]"
	if !ok {
		id = len(b.consts)
		b.consts[c] = id
		if id >= maxConsts {
			b.unsupported(c.Decl, "more than "+strconv.Itoa(maxConsts)+" constants of collection types")
		}
		f := m.NewFunc(name, nil, []*Type{lt})
		f.Entry = true
		cs := b.newFn(f, nil, nil)
		v := cs.valueAs(c.Decl.Value, t)
		l := cs.call("list.push", lt, "", cs.call("list.new", lt, ""), v)
		// readers on any thread share it
		cs.add(OpShare, nil, nil, l)
		cs.retv(l)
		cs.finishFn()
	}
	l := s.rt("const_once", lt, s.add(OpFuncAddr, intT, name), s.add(OpFuncAddr, intT, b.dropList(t)), s.cint(int64(id)))
	return s.call("list.index", t, "", l, s.cint(0))
}

// shiftCount is the shift amount e of a shift of a value of type t: any
// integer type, as the word of t (the shift checks its range).
func (s *fn) shiftCount(e ast.Expr, t *Type) *Value {
	r := s.expr(e)
	if r.Type != t && r.Type.Kind == KInt && t.Kind == KInt {
		return s.add(OpIntConv, t, nil, r)
	}
	return s.coerce(r, t, e)
}

// sizedMinMax is `a.min(b)` or `a.max(b)` on a sized integer type: the
// receiver on a tie, ordered as the type is (a UInt64 unsigned).
func (s *fn) sizedMinMax(name string, a, b *Value) *Value {
	// min: b < a ? b : a; max: a < b ? b : a
	var c *Value
	if name == "min" {
		c = s.compare(OpLt, b, a)
	} else {
		c = s.compare(OpLt, a, b)
	}
	join := s.newBlock()
	res := join.AddParam(a.Type)
	yes, no := s.branch(c)
	s.cur = yes
	s.jump(join, b)
	s.cur = no
	s.jump(join, a)
	s.seal(join)
	s.cur = join
	return res
}
