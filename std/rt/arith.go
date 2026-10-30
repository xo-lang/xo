package rt

import (
	"math"
	"math/bits"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// Signed is every Xo signed integer type.
type Signed interface {
	~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned is every Xo unsigned integer type.
type Unsigned interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Integer is every Xo integer type (Duration is an int64 underneath).
type Integer interface {
	Signed | Unsigned
}

// Number is every Xo numeric type.
type Number interface {
	Integer | ~float32 | ~float64
}

func bitsOf[T Integer](x T) int { return int(unsafe.Sizeof(x)) * 8 }

func minS[T Signed]() T {
	var z T
	return T(-1) << (bitsOf(z) - 1)
}

func typeName[T Integer](x T) string {
	switch any(x).(type) {
	case int8:
		return "Int8"
	case int16:
		return "Int16"
	case int32:
		return "Int32"
	case int64:
		return "Int"
	case uint8:
		return "UInt8"
	case uint16:
		return "UInt16"
	case uint32:
		return "UInt32"
	case uint64:
		return "UInt64"
	case Duration:
		return "Duration"
	case Time:
		return "Time"
	}
	return "integer"
}

func istr[T Integer](x T) string {
	if x < 0 {
		return strconv.FormatInt(int64(x), 10)
	}
	return strconv.FormatUint(uint64(x), 10)
}

func overflow[T Integer](a T, op string, b T, at string) {
	Faultf(at, "integer overflow: %s %s %s does not fit %s", istr(a), op, istr(b), typeName(a))
}

// AddS is checked `a + b` on signed integers (core 2.1: overflow faults).
func AddS[T Signed](a, b T, at string) T {
	c := a + b
	if (c > a) != (b > 0) {
		overflow(a, "+", b, at)
	}
	return c
}

// SubS is checked `a - b` on signed integers.
func SubS[T Signed](a, b T, at string) T {
	c := a - b
	if (c < a) != (b > 0) {
		overflow(a, "-", b, at)
	}
	return c
}

// MulS is checked `a * b` on signed integers.
func MulS[T Signed](a, b T, at string) T {
	if a == 0 || b == 0 {
		return 0
	}
	c := a * b
	if (a == -1 && b == minS[T]()) || (b == -1 && a == minS[T]()) || c/b != a {
		overflow(a, "*", b, at)
	}
	return c
}

// DivS is checked `a / b` on signed integers (truncating).
func DivS[T Signed](a, b T, at string) T {
	if b == 0 {
		Faultf(at, "division by zero: %s / 0", istr(a))
	}
	if b == -1 && a == minS[T]() {
		overflow(a, "/", b, at)
	}
	return a / b
}

// RemS is checked `a % b` on signed integers.
func RemS[T Signed](a, b T, at string) T {
	if b == 0 {
		Faultf(at, "division by zero: %s %% 0", istr(a))
	}
	if b == -1 {
		return 0
	}
	return a % b
}

// NegS is checked unary minus.
func NegS[T Signed](a T, at string) T {
	if a == minS[T]() {
		Faultf(at, "integer overflow: -(%s) does not fit %s", istr(a), typeName(a))
	}
	return -a
}

// AddU is checked `a + b` on unsigned integers.
func AddU[T Unsigned](a, b T, at string) T {
	c := a + b
	if c < a {
		overflow(a, "+", b, at)
	}
	return c
}

// SubU is checked `a - b` on unsigned integers.
func SubU[T Unsigned](a, b T, at string) T {
	if b > a {
		overflow(a, "-", b, at)
	}
	return a - b
}

// MulU is checked `a * b` on unsigned integers.
func MulU[T Unsigned](a, b T, at string) T {
	if a == 0 || b == 0 {
		return 0
	}
	c := a * b
	if c/a != b {
		overflow(a, "*", b, at)
	}
	return c
}

// DivU is checked `a / b` on unsigned integers.
func DivU[T Unsigned](a, b T, at string) T {
	if b == 0 {
		Faultf(at, "division by zero: %s / 0", istr(a))
	}
	return a / b
}

// RemU is checked `a % b` on unsigned integers.
func RemU[T Unsigned](a, b T, at string) T {
	if b == 0 {
		Faultf(at, "division by zero: %s %% 0", istr(a))
	}
	return a % b
}

// NegU is unary minus on an unsigned integer: only -0 fits.
func NegU[T Unsigned](a T, at string) T {
	if a != 0 {
		Faultf(at, "integer overflow: -(%s) does not fit %s", istr(a), typeName(a))
	}
	return 0
}

// Shl is checked `a << n`: the shift must be in range and lose no bits.
func Shl[T Integer, N Integer](a T, n N, at string) T {
	if n < 0 || int64(n) >= int64(bitsOf(a)) {
		Faultf(at, "shift amount %s out of range for %s", istr(n), typeName(a))
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		Faultf(at, "integer overflow: %s << %s does not fit %s", istr(a), istr(n), typeName(a))
	}
	return c
}

// Shr is checked `a >> n` (arithmetic for signed types).
func Shr[T Integer, N Integer](a T, n N, at string) T {
	if n < 0 || int64(n) >= int64(bitsOf(a)) {
		Faultf(at, "shift amount %s out of range for %s", istr(n), typeName(a))
	}
	return a >> uint(n)
}

// Conv converts between integer types, faulting out of range (core 2.1:
// `Int32(x)` faults out of range).
func Conv[To, From Integer](x From, at string) To {
	y := To(x)
	if From(y) != x || (y < 0) != (x < 0) {
		var z To
		Faultf(at, "%s is out of range for %s", istr(x), typeName(z))
	}
	return y
}

// TryConv is `Int32.try(x)`: None when out of range.
func TryConv[To, From Integer](x From) Option[To] {
	y := To(x)
	if From(y) != x || (y < 0) != (x < 0) {
		return Option[To]{}
	}
	return Some(y)
}

// FloatToInt is `Int(f)` and the sized variants: truncates toward zero,
// faults on NaN or out of range.
func FloatToInt[To Integer, F ~float32 | ~float64](f F, at string) To {
	x := float64(f)
	var z To
	if math.IsNaN(x) {
		Faultf(at, "cannot convert NaN to %s", typeName(z))
	}
	t := math.Trunc(x)
	var lo, hi float64
	if minOK := To(0) - 1; minOK < 0 {
		b := bitsOf(z)
		lo = -math.Ldexp(1, b-1)
		hi = math.Ldexp(1, b-1)
	} else {
		lo = 0
		hi = math.Ldexp(1, bitsOf(z))
	}
	if t < lo || t >= hi {
		Faultf(at, "%s is out of range for %s", strconv.FormatFloat(x, 'g', -1, 64), typeName(z))
	}
	return To(t)
}

// Int_pow is `n.pow(e)`, checked.
func Int_pow(_ *Task, n, e int64) int64 {
	if e < 0 {
		FaultHere("negative exponent %d in pow", e)
	}
	r := int64(1)
	for i := int64(0); i < e; i++ {
		r = MulS(r, n, "")
	}
	return r
}

// Int_abs is `n.abs()`, faulting on the minimum value.
func Int_abs(_ *Task, n int64) int64 {
	if n < 0 {
		if n == math.MinInt64 {
			FaultHere("integer overflow: abs(%d) does not fit Int", n)
		}
		return -n
	}
	return n
}

// Int_wrapping_add is `n.wrapping_add(m)`.
func Int_wrapping_add(_ *Task, a, b int64) int64 { return a + b }

// Int_wrapping_sub is `n.wrapping_sub(m)`.
func Int_wrapping_sub(_ *Task, a, b int64) int64 { return a - b }

// Int_wrapping_mul is `n.wrapping_mul(m)`.
func Int_wrapping_mul(_ *Task, a, b int64) int64 { return a * b }

// Int_wrapping_shl is `n.wrapping_shl(k)` (decision 0260): k mod 64.
func Int_wrapping_shl(_ *Task, a, k int64) int64 { return a << (uint64(k) & 63) }

// Int_shr_logical is `n.shr_logical(k)`: a logical shift by k mod 64.
func Int_shr_logical(_ *Task, a, k int64) int64 { return int64(uint64(a) >> (uint64(k) & 63)) }

// Int_rotate_right is `n.rotate_right(k)`.
func Int_rotate_right(_ *Task, a, k int64) int64 {
	return int64(bits.RotateLeft64(uint64(a), -int(uint64(k)&63)))
}

// Int_rotate_right32 is `n.rotate_right32(k)`: the low 32 bits, zero extended.
func Int_rotate_right32(_ *Task, a, k int64) int64 {
	return int64(bits.RotateLeft32(uint32(a), -int(uint64(k)&31)))
}

// Int_swap_bytes is `n.swap_bytes()`.
func Int_swap_bytes(_ *Task, a int64) int64 { return int64(bits.ReverseBytes64(uint64(a))) }

// Int_mul_hi_unsigned is `n.mul_hi_unsigned(m)`.
func Int_mul_hi_unsigned(_ *Task, a, b int64) int64 {
	hi, _ := bits.Mul64(uint64(a), uint64(b))
	return int64(hi)
}

// Int_lt_unsigned is `n.lt_unsigned(m)`.
func Int_lt_unsigned(_ *Task, a, b int64) bool { return uint64(a) < uint64(b) }

// Int_le_unsigned is `n.le_unsigned(m)`.
func Int_le_unsigned(_ *Task, a, b int64) bool { return uint64(a) <= uint64(b) }

// Int_saturating_add is `n.saturating_add(m)`.
func Int_saturating_add(_ *Task, a, b int64) int64 {
	c := a + b
	if (c > a) != (b > 0) {
		if b > 0 {
			return math.MaxInt64
		}
		return math.MinInt64
	}
	return c
}

// Int_saturating_sub is `n.saturating_sub(m)`.
func Int_saturating_sub(_ *Task, a, b int64) int64 {
	c := a - b
	if (c < a) != (b > 0) {
		if b > 0 {
			return math.MinInt64
		}
		return math.MaxInt64
	}
	return c
}

// Int_saturating_mul is `n.saturating_mul(m)`.
func Int_saturating_mul(_ *Task, a, b int64) int64 {
	if r, ok := checkedMul(a, b); ok {
		return r
	}
	if (a < 0) != (b < 0) {
		return math.MinInt64
	}
	return math.MaxInt64
}

func checkedMul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	if (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64) || c/b != a {
		return 0, false
	}
	return c, true
}

// Int_checked_add is `n.checked_add(m)`.
func Int_checked_add(_ *Task, a, b int64) Option[int64] {
	c := a + b
	if (c > a) != (b > 0) {
		return Option[int64]{}
	}
	return Some(c)
}

// Int_checked_sub is `n.checked_sub(m)`.
func Int_checked_sub(_ *Task, a, b int64) Option[int64] {
	c := a - b
	if (c < a) != (b > 0) {
		return Option[int64]{}
	}
	return Some(c)
}

// Int_checked_mul is `n.checked_mul(m)`.
func Int_checked_mul(_ *Task, a, b int64) Option[int64] {
	if r, ok := checkedMul(a, b); ok {
		return Some(r)
	}
	return Option[int64]{}
}

// Int_checked_div is `n.checked_div(m)`.
func Int_checked_div(_ *Task, a, b int64) Option[int64] {
	if b == 0 || (b == -1 && a == math.MinInt64) {
		return Option[int64]{}
	}
	return Some(a / b)
}

// Int_parse is `Int.parse(s)`.
func Int_parse(_ *Task, s string) Option[int64] {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return Option[int64]{}
	}
	return Some(n)
}

// Float_parse is `Float.parse(s)`.
func Float_parse(_ *Task, s string) Option[float64] {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return Option[float64]{}
	}
	return Some(f)
}

// Int8_try is `Int8.try(x)`.
func Int8_try(_ *Task, x int64) Option[int8] { return TryConv[int8](x) }

// Int16_try is `Int16.try(x)`.
func Int16_try(_ *Task, x int64) Option[int16] { return TryConv[int16](x) }

// Int32_try is `Int32.try(x)`.
func Int32_try(_ *Task, x int64) Option[int32] { return TryConv[int32](x) }

// Int64_try is `Int64.try(x)`.
func Int64_try(_ *Task, x int64) Option[int64] { return Some(x) }

// UInt8_try is `UInt8.try(x)`.
func UInt8_try(_ *Task, x int64) Option[uint8] { return TryConv[uint8](x) }

// UInt16_try is `UInt16.try(x)`.
func UInt16_try(_ *Task, x int64) Option[uint16] { return TryConv[uint16](x) }

// UInt32_try is `UInt32.try(x)`.
func UInt32_try(_ *Task, x int64) Option[uint32] { return TryConv[uint32](x) }

// UInt64_from_bits is `UInt64.from_bits(b)`: the same 64 bits.
func UInt64_from_bits(_ *Task, b int64) uint64 { return uint64(b) }

// UInt64_to_bits is `u.to_bits()`: the same 64 bits.
func UInt64_to_bits(_ *Task, u uint64) int64 { return int64(u) }

// UInt64_try is `UInt64.try(x)`.
func UInt64_try(_ *Task, x int64) Option[uint64] { return TryConv[uint64](x) }

// Float_floor is `f.floor()`.
func Float_floor(_ *Task, f float64) float64 { return math.Floor(f) }

// Float_ceil is `f.ceil()`.
func Float_ceil(_ *Task, f float64) float64 { return math.Ceil(f) }

// Float_round is `f.round()`: half away from zero.
func Float_round(_ *Task, f float64) float64 { return math.Round(f) }

// Float_trunc is `f.trunc()`.
func Float_trunc(_ *Task, f float64) float64 { return math.Trunc(f) }

// Float_abs is `f.abs()`.
func Float_abs(_ *Task, f float64) float64 { return math.Abs(f) }

// Float_sqrt is `f.sqrt()`.
func Float_sqrt(_ *Task, f float64) float64 { return math.Sqrt(f) }

// Float_is_nan is `f.is_nan()`.
func Float_is_nan(_ *Task, f float64) bool { return math.IsNaN(f) }

// FloatCmp orders floats totally: NaN sorts after every other value (std.md
// 1).
func FloatCmp(a, b float64) int {
	an, bn := math.IsNaN(a), math.IsNaN(b)
	switch {
	case an && bn:
		return 0
	case an:
		return 1
	case bn:
		return -1
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// IntToFloat is `Float(i)` for any integer type.
func IntToFloat[F ~float32 | ~float64, I Integer](x I) F { return F(x) }

// RuneOf is `Rune(n)`: the character with code point n, faulting when n
// is not a Unicode scalar value (decision 0079).
func RuneOf[From Integer](n From, at string) Rune {
	if !validRune(n) {
		Faultf(at, "%s is not a Unicode scalar value, so it is not a Rune", istr(n))
	}
	return Rune(n)
}

// Rune_try is `Rune.try(n)`.
func Rune_try(_ *Task, n int64) Option[Rune] {
	if !validRune(n) {
		return Option[Rune]{}
	}
	return Some(Rune(n))
}

func validRune[T Integer](n T) bool {
	if n < 0 {
		return false
	}
	u := uint64(n)
	return u <= 0x10ffff && (u < 0xd800 || u > 0xdfff)
}

// Int_parse_base is `Int.parse_base(s, base)` (decision 0287).
func Int_parse_base(_ *Task, s string, base int64) Option[int64] {
	if base != 0 && (base < 2 || base > 36) {
		FaultHere("Int.parse_base: base %d is not 0 or 2 to 36", base)
	}
	n, err := strconv.ParseInt(s, int(base), 64)
	if err != nil {
		return Option[int64]{}
	}
	return Some(n)
}

// Str_pad_left, Str_pad_right and Str_center are `s.pad_left(width, fill)`
// and the others (decision 0410): fill one rune, a width in runes.
func Str_pad_left(_ *Task, s string, width int64, fill string) string {
	n := padFill(s, width, fill, "pad_left")
	if n < 0 {
		return s
	}
	return strings.Repeat(fill, int(width)-n) + s
}

func Str_pad_right(_ *Task, s string, width int64, fill string) string {
	n := padFill(s, width, fill, "pad_right")
	if n < 0 {
		return s
	}
	return s + strings.Repeat(fill, int(width)-n)
}

func Str_center(_ *Task, s string, width int64, fill string) string {
	n := padFill(s, width, fill, "center")
	if n < 0 {
		return s
	}
	left := (int(width) - n) / 2
	return strings.Repeat(fill, left) + s + strings.Repeat(fill, int(width)-n-left)
}

// padFill checks the fill and returns the rune count of s, or -1 when s is
// already width runes long.
func padFill(s string, width int64, fill, what string) int {
	if utf8.RuneCountInString(fill) != 1 {
		FaultHere("Str.%s: the fill %q is not one rune", what, fill)
	}
	n := utf8.RuneCountInString(s)
	if int64(n) >= width {
		return -1
	}
	return n
}

// Float_to_fixed, Float_to_exp and Float_to_general are `f.to_fixed(prec,
// width, zero, plus)` and the others (decision 0410): strconv.FormatFloat's
// 'f', 'e' and 'g' text, padded as Int_to_str_width pads.
func Float_to_fixed(_ *Task, f float64, prec, width int64, zero, plus bool) string {
	return padNumber(strconv.FormatFloat(f, 'f', int(prec), 64), width, zero, plus)
}

func Float_to_exp(_ *Task, f float64, prec, width int64, zero, plus bool) string {
	return padNumber(strconv.FormatFloat(f, 'e', int(prec), 64), width, zero, plus)
}

func Float_to_general(_ *Task, f float64, prec, width int64, zero, plus bool) string {
	return padNumber(strconv.FormatFloat(f, 'g', int(prec), 64), width, zero, plus)
}

func padNumber(text string, width int64, zero, plus bool) string {
	sign, body := "", text
	if strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+") {
		sign, body = text[:1], text[1:]
	} else if plus {
		sign = "+"
	}
	used := len(sign) + len(body)
	if width < 0 {
		w := int(-width)
		if used >= w {
			return sign + body
		}
		return sign + body + strings.Repeat(" ", w-used)
	}
	if used >= int(width) {
		return sign + body
	}
	if zero && body != "Inf" && body != "NaN" {
		return sign + strings.Repeat("0", int(width)-used) + body
	}
	return strings.Repeat(" ", int(width)-used) + sign + body
}

// Int_to_str_width is `n.to_str_width(width, base, zero, plus)` (decision 0410).
func Int_to_str_width(_ *Task, n, width, base int64, zero, plus bool) string {
	if base < 2 || base > 36 {
		FaultHere("Int.to_str_base: base %d is not 2 to 36", base)
	}
	body := strconv.FormatInt(n, int(base))
	sign := ""
	if plus {
		sign = "+"
	}
	if strings.HasPrefix(body, "-") {
		sign = "-"
		body = body[1:]
	}
	used := len(sign) + len(body)
	if width < 0 {
		w := int(-width)
		if used >= w {
			return sign + body
		}
		return sign + body + strings.Repeat(" ", w-used)
	}
	if used >= int(width) {
		return sign + body
	}
	if zero {
		return sign + strings.Repeat("0", int(width)-used) + body
	}
	return strings.Repeat(" ", int(width)-used) + sign + body
}

// Int_to_str_base is `n.to_str_base(base)` (decision 0287).
func Int_to_str_base(_ *Task, n, base int64) string {
	if base < 2 || base > 36 {
		FaultHere("Int.to_str_base: base %d is not 2 to 36", base)
	}
	return strconv.FormatInt(n, int(base))
}

// Bool_parse is `Bool.parse(s)` (decision 0287).
func Bool_parse(_ *Task, s string) Option[bool] {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return Option[bool]{}
	}
	return Some(b)
}

// Str_unquote is `s.unquote()` (decision 0287): None also when the text is
// not UTF-8 (a \x escape of a byte alone).
func Str_unquote(_ *Task, s string) Option[string] {
	u, err := strconv.Unquote(s)
	if err != nil || !utf8.ValidString(u) {
		return Option[string]{}
	}
	return Some(u)
}
