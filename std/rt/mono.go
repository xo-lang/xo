package rt

import (
	"math"
	"math/bits"
)

// CmpO orders two integers or strings without boxing: the typed form of
// Cmp that `xo build --mono` emits in generated XoCmpT methods.
func CmpO[T Integer | ~string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Typed checked Int arithmetic for `xo build --mono`. AddS and friends are
// Go generic functions: Go compiles them once per GC shape with a
// dictionary argument, and the shape copies are just over the inlining
// budget, so every checked `+` is a call. These int64 forms keep the fast
// path small enough to inline and move the fault to a separate function.

// AddI is checked `a + b` on Int.
func AddI(a, b int64, at string) int64 {
	c := a + b
	if (c > a) != (b > 0) {
		overflowI(a, "+", b, at)
	}
	return c
}

// SubI is checked `a - b` on Int.
func SubI(a, b int64, at string) int64 {
	c := a - b
	if (c < a) != (b > 0) {
		overflowI(a, "-", b, at)
	}
	return c
}

// MulI is checked `a * b` on Int. Operands that fit in 32 bits cannot
// overflow; others take the out of line check.
func MulI(a, b int64, at string) int64 {
	if uint64(a+1<<31)|uint64(b+1<<31) < 1<<32 {
		return a * b
	}
	return mulSlowI(a, b, at)
}

//go:noinline
func mulSlowI(a, b int64, at string) int64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	shi := int64(hi) - (a>>63)&b - (b>>63)&a
	if shi != int64(lo)>>63 {
		overflow(a, "*", b, at)
	}
	return int64(lo)
}

// DivI is checked `a / b` on Int (truncating).
func DivI(a, b int64, at string) int64 {
	if b == 0 || b == -1 && a == math.MinInt64 {
		divFaultI(a, "/", b, at)
	}
	return a / b
}

// RemI is checked `a % b` on Int.
func RemI(a, b int64, at string) int64 {
	if b == 0 || b == -1 {
		return remSlowI(a, b, at)
	}
	return a % b
}

//go:noinline
func overflowI(a int64, op string, b int64, at string) {
	overflow(a, op, b, at)
}

//go:noinline
func divFaultI(a int64, op string, b int64, at string) {
	if b == 0 {
		Faultf(at, "division by zero: %s %s 0", istr(a), op)
	}
	overflow(a, op, b, at)
}

//go:noinline
func remSlowI(a, b int64, at string) int64 { return RemS(a, b, at) }

// AtM is `xs[i]` for `xo build --mono`: At with the fault moved out of
// line so the shape copy inlines.
func (l List[T]) AtM(i int64, at string) T {
	if uint64(i) >= uint64(len(l.s)) {
		indexFault(i, len(l.s), at)
	}
	return l.s[i]
}

//go:noinline
func indexFault(i int64, n int, at string) {
	Faultf(at, "index %d out of range for List of length %d", i, n)
}
