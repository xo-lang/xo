package rt

import (
	"hash/maphash"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Equaler is implemented by types with structural equality (Eq, std.md 1).
// o has the same dynamic type unless the values came through an interface.
type Equaler interface{ XoEq(o any) bool }

// Hasher is implemented by hashable types.
type Hasher interface{ XoHash() uint64 }

// Comparer is implemented by ordered types (`derive Ord`).
type Comparer interface{ XoCmp(o any) int }

// Debugger is implemented by every generated type (Debug, core 2.9).
type Debugger interface{ XoDebug(b *strings.Builder) }

// Eq is structural equality (`==`, core 2.9). Common scalar types are
// compared without boxing.
func Eq[T any](a, b T) bool {
	switch x := any(a).(type) {
	case int64:
		return x == any(b).(int64)
	case string:
		return x == any(b).(string)
	case bool:
		return x == any(b).(bool)
	case Duration:
		return x == any(b).(Duration)
	case Time:
		return x == any(b).(Time)
	case Rune:
		return x == any(b).(Rune)
	case Bytes:
		return x == any(b).(Bytes)
	}
	return EqAny(any(a), any(b))
}

// EqAny compares two values of the same Xo type.
func EqAny(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case int64:
		y, ok := b.(int64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case int32:
		y, ok := b.(int32)
		return ok && x == y
	case Rune:
		y, ok := b.(Rune)
		return ok && x == y
	case Bytes:
		y, ok := b.(Bytes)
		return ok && x == y
	case Duration:
		y, ok := b.(Duration)
		return ok && x == y
	case Time:
		y, ok := b.(Time)
		return ok && x == y
	case int8:
		y, ok := b.(int8)
		return ok && x == y
	case int16:
		y, ok := b.(int16)
		return ok && x == y
	case uint8:
		y, ok := b.(uint8)
		return ok && x == y
	case uint16:
		y, ok := b.(uint16)
		return ok && x == y
	case uint32:
		y, ok := b.(uint32)
		return ok && x == y
	case uint64:
		y, ok := b.(uint64)
		return ok && x == y
	case float32:
		y, ok := b.(float32)
		return ok && x == y
	case Unit:
		_, ok := b.(Unit)
		return ok
	case Equaler:
		return x.XoEq(b)
	}
	return reflect.DeepEqual(a, b)
}

var hashSeed = maphash.MakeSeed()

func mix(h, x uint64) uint64 {
	h ^= x + 0x9e3779b97f4a7c15 + (h << 6) + (h >> 2)
	return h
}

func mixInt(x uint64) uint64 {
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

// Hash hashes a value consistently with Eq. Common scalar types are
// hashed without boxing.
func Hash[T any](v T) uint64 {
	switch x := any(v).(type) {
	case int64:
		return mixInt(uint64(x))
	case string:
		return maphash.String(hashSeed, x)
	case Duration:
		return mixInt(uint64(x)) ^ 0x64
	case Time:
		return mixInt(uint64(x)) ^ 0x74
	case Rune:
		return mixInt(uint64(x)) ^ 0x72
	}
	return HashAny(any(v))
}

// HashAny hashes a value of any Xo type.
func HashAny(v any) uint64 {
	switch x := v.(type) {
	case int64:
		return mixInt(uint64(x))
	case string:
		return maphash.String(hashSeed, x)
	case bool:
		if x {
			return 1
		}
		return 2
	case float64:
		if x == 0 {
			return 3
		}
		return mixInt(math.Float64bits(x))
	case int32:
		return mixInt(uint64(x))
	case Rune:
		return mixInt(uint64(x)) ^ 0x72
	case Bytes:
		return maphash.String(hashSeed, string(x)) ^ 0x62
	case Duration:
		return mixInt(uint64(x)) ^ 0x64
	case Time:
		return mixInt(uint64(x)) ^ 0x74
	case int8:
		return mixInt(uint64(x))
	case int16:
		return mixInt(uint64(x))
	case uint8:
		return mixInt(uint64(x))
	case uint16:
		return mixInt(uint64(x))
	case uint32:
		return mixInt(uint64(x))
	case uint64:
		return mixInt(x)
	case float32:
		return mixInt(uint64(math.Float32bits(x)))
	case Unit:
		return 7
	case Hasher:
		return x.XoHash()
	}
	return maphash.String(hashSeed, Debug(v))
}

// Cmp orders two values of the same Ord type (std.md 1). Common scalar
// types are compared without boxing.
func Cmp[T any](a, b T) int {
	switch x := any(a).(type) {
	case int64:
		return cmp3(x, any(b).(int64))
	case string:
		return strings.Compare(x, any(b).(string))
	case Duration:
		return cmp3(x, any(b).(Duration))
	case Time:
		return cmp3(x, any(b).(Time))
	}
	return CmpAny(any(a), any(b))
}

// CmpAny orders two values of the same Xo type.
func CmpAny(a, b any) int {
	switch x := a.(type) {
	case int64:
		return cmp3(x, b.(int64))
	case string:
		return strings.Compare(x, b.(string))
	case float64:
		return FloatCmp(x, b.(float64))
	case bool:
		y := b.(bool)
		switch {
		case x == y:
			return 0
		case !x:
			return -1
		}
		return 1
	case int32:
		return cmp3(x, b.(int32))
	case Rune:
		return cmp3(x, b.(Rune))
	case Bytes:
		return strings.Compare(string(x), string(b.(Bytes)))
	case Duration:
		return cmp3(x, b.(Duration))
	case Time:
		return cmp3(x, b.(Time))
	case int8:
		return cmp3(x, b.(int8))
	case int16:
		return cmp3(x, b.(int16))
	case uint8:
		return cmp3(x, b.(uint8))
	case uint16:
		return cmp3(x, b.(uint16))
	case uint32:
		return cmp3(x, b.(uint32))
	case uint64:
		return cmp3(x, b.(uint64))
	case float32:
		return FloatCmp(float64(x), float64(b.(float32)))
	case Unit:
		return 0
	case Comparer:
		return x.XoCmp(b)
	}
	return strings.Compare(Debug(a), Debug(b))
}

func cmp3[T Integer](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Min is `a.min(b)` on any Ord type.
func Min[T any](a, b T) T {
	if Cmp(b, a) < 0 {
		return b
	}
	return a
}

// Max is `a.max(b)`.
func Max[T any](a, b T) T {
	if Cmp(b, a) > 0 {
		return b
	}
	return a
}

// Clamp is `a.clamp(lo, hi)`.
func Clamp[T any](a, lo, hi T) T { return Min(Max(a, lo), hi) }

// Debug is the structural text of a value (core 2.9, 11.4): `[1, 2]`,
// `User{id: 1, name: "a"}`, `NotFound(id: 42)`.
func Debug(v any) string {
	var b strings.Builder
	debugTo(&b, v)
	return b.String()
}

// DebugT is Debug for a statically typed value.
func DebugT[T any](v T) string { return Debug(any(v)) }

func debugTo(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("<nil>")
	case string:
		b.WriteString(quote(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case float64:
		b.WriteString(FloatStr(x))
	case float32:
		b.WriteString(FloatStr(float64(x)))
	case Rune:
		b.WriteString(quoteRune(int32(x)))
	case Bytes:
		b.WriteString("b" + quote(string(x)))
	case Duration:
		b.WriteString(x.String())
	case Time:
		b.WriteString(x.String())
	case int8, int16, int32, uint8, uint16, uint32, uint64:
		b.WriteString(Display(nil, x))
	case Unit:
		b.WriteString("()")
	case Debugger:
		x.XoDebug(b)
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Func {
			b.WriteString("<fn>")
			return
		}
		b.WriteString(reflect.TypeOf(v).String())
	}
}

// Display is the user facing text of a value, used by `${x}` (core 1.6).
func Display(tk *Task, v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return FloatStr(x)
	case float32:
		return FloatStr(float64(x))
	case Rune:
		return string(rune(x))
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case Duration:
		return x.String()
	case Time:
		return x.String()
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case IDisplay:
		return x.M_display(tk)
	}
	return Debug(v)
}

// DisplayT is Display for a statically typed value.
func DisplayT[T any](tk *Task, v T) string { return Display(tk, any(v)) }

// FloatStr formats a float: the shortest text that reads back the same
// value, always with a decimal point or exponent (`1.0`, `0.1`, `1e+21`).
func FloatStr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eIN") {
		s += ".0"
	}
	return s
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case 0:
			b.WriteString(`\0`)
		case utf8.RuneError:
			b.WriteString(`\u{FFFD}`)
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteString(`\x` + strconv.FormatUint(uint64(r)>>4, 16) + strconv.FormatUint(uint64(r)&15, 16))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func quoteRune(r int32) string {
	switch r {
	case '\'':
		return `'\''`
	case '\\':
		return `'\\'`
	case '\n':
		return `'\n'`
	case '\t':
		return `'\t'`
	}
	return "'" + string(r) + "'"
}

func tupleDebug(b *strings.Builder, xs ...any) {
	b.WriteByte('(')
	for i, x := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		debugTo(b, x)
	}
	b.WriteByte(')')
}

func tupleEq(xs, ys []any) bool {
	for i := range xs {
		if !EqAny(xs[i], ys[i]) {
			return false
		}
	}
	return true
}

func tupleHash(xs ...any) uint64 {
	h := uint64(0x7475706c)
	for _, x := range xs {
		h = mix(h, HashAny(x))
	}
	return h
}

func tupleCmp(xs, ys []any) int {
	for i := range xs {
		if c := CmpAny(xs[i], ys[i]); c != 0 {
			return c
		}
	}
	return 0
}

// XoDebug implements Debug.
func (t Tuple2[A, B]) XoDebug(b *strings.Builder) { tupleDebug(b, t.F0, t.F1) }

// XoEq implements Eq.
func (t Tuple2[A, B]) XoEq(o any) bool {
	p, ok := o.(Tuple2[A, B])
	return ok && Eq(t.F0, p.F0) && Eq(t.F1, p.F1)
}

// XoHash implements Hash.
func (t Tuple2[A, B]) XoHash() uint64 { return tupleHash(t.F0, t.F1) }

// XoCmp implements Ord.
func (t Tuple2[A, B]) XoCmp(o any) int {
	p := o.(Tuple2[A, B])
	return tupleCmp([]any{t.F0, t.F1}, []any{p.F0, p.F1})
}

// XoDebug implements Debug.
func (t Tuple3[A, B, C]) XoDebug(b *strings.Builder) { tupleDebug(b, t.F0, t.F1, t.F2) }

// XoEq implements Eq.
func (t Tuple3[A, B, C]) XoEq(o any) bool {
	p, ok := o.(Tuple3[A, B, C])
	return ok && tupleEq([]any{t.F0, t.F1, t.F2}, []any{p.F0, p.F1, p.F2})
}

// XoHash implements Hash.
func (t Tuple3[A, B, C]) XoHash() uint64 { return tupleHash(t.F0, t.F1, t.F2) }

// XoCmp implements Ord.
func (t Tuple3[A, B, C]) XoCmp(o any) int {
	p := o.(Tuple3[A, B, C])
	return tupleCmp([]any{t.F0, t.F1, t.F2}, []any{p.F0, p.F1, p.F2})
}

// XoDebug implements Debug.
func (t Tuple4[A, B, C, D]) XoDebug(b *strings.Builder) { tupleDebug(b, t.F0, t.F1, t.F2, t.F3) }

// XoEq implements Eq.
func (t Tuple4[A, B, C, D]) XoEq(o any) bool {
	p, ok := o.(Tuple4[A, B, C, D])
	return ok && tupleEq([]any{t.F0, t.F1, t.F2, t.F3}, []any{p.F0, p.F1, p.F2, p.F3})
}

// XoHash implements Hash.
func (t Tuple4[A, B, C, D]) XoHash() uint64 { return tupleHash(t.F0, t.F1, t.F2, t.F3) }

// XoCmp implements Ord.
func (t Tuple4[A, B, C, D]) XoCmp(o any) int {
	p := o.(Tuple4[A, B, C, D])
	return tupleCmp([]any{t.F0, t.F1, t.F2, t.F3}, []any{p.F0, p.F1, p.F2, p.F3})
}

// XoDebug implements Debug.
func (t Tuple5[A, B, C, D, E]) XoDebug(b *strings.Builder) {
	tupleDebug(b, t.F0, t.F1, t.F2, t.F3, t.F4)
}

// XoEq implements Eq.
func (t Tuple5[A, B, C, D, E]) XoEq(o any) bool {
	p, ok := o.(Tuple5[A, B, C, D, E])
	return ok && tupleEq([]any{t.F0, t.F1, t.F2, t.F3, t.F4}, []any{p.F0, p.F1, p.F2, p.F3, p.F4})
}

// XoHash implements Hash.
func (t Tuple5[A, B, C, D, E]) XoHash() uint64 { return tupleHash(t.F0, t.F1, t.F2, t.F3, t.F4) }

// XoCmp implements Ord.
func (t Tuple5[A, B, C, D, E]) XoCmp(o any) int {
	p := o.(Tuple5[A, B, C, D, E])
	return tupleCmp([]any{t.F0, t.F1, t.F2, t.F3, t.F4}, []any{p.F0, p.F1, p.F2, p.F3, p.F4})
}

// XoDebug implements Debug.
func (t Tuple6[A, B, C, D, E, F]) XoDebug(b *strings.Builder) {
	tupleDebug(b, t.F0, t.F1, t.F2, t.F3, t.F4, t.F5)
}

// XoEq implements Eq.
func (t Tuple6[A, B, C, D, E, F]) XoEq(o any) bool {
	p, ok := o.(Tuple6[A, B, C, D, E, F])
	return ok && tupleEq([]any{t.F0, t.F1, t.F2, t.F3, t.F4, t.F5}, []any{p.F0, p.F1, p.F2, p.F3, p.F4, p.F5})
}

// XoHash implements Hash.
func (t Tuple6[A, B, C, D, E, F]) XoHash() uint64 {
	return tupleHash(t.F0, t.F1, t.F2, t.F3, t.F4, t.F5)
}

// XoCmp implements Ord.
func (t Tuple6[A, B, C, D, E, F]) XoCmp(o any) int {
	p := o.(Tuple6[A, B, C, D, E, F])
	return tupleCmp([]any{t.F0, t.F1, t.F2, t.F3, t.F4, t.F5}, []any{p.F0, p.F1, p.F2, p.F3, p.F4, p.F5})
}

// XoDebug implements Debug: `()`.
func (Unit) XoDebug(b *strings.Builder) { b.WriteString("()") }

// DebugOf is the Debug text of v.
func DebugOf(tk *Task, v any) string { return Debug(v) }
