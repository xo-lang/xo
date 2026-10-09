package rt

import (
	"fmt"
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

// Typed checked Int arithmetic, emitted by default (`xo build`; the
// generic AddS and friends under --no-mono and for sized integer types).
// AddS and friends are Go generic functions: Go compiles them once per GC
// shape with a dictionary argument, and the shape copies are just over the
// inlining budget, so every checked `+` would be a call. These int64 forms
// are small enough to inline into small callers: the fault is a panic of
// an *arithFault (Go's inliner counts a panic as about one node, a call as
// about sixty), and the overflow tests use intrinsics (bits.Mul64), not
// calls. The fault message and the stack are built only when the panic is
// recovered (arithFault.fault), with the same text and frames as a fault
// raised in place (gotchas/checked-arith-inline-budget.md).

// arithFault is the panic value of a failed typed Int operation: op is
// '+', '-', '*', '/', '%' (overflow, or division by zero when b is 0 for
// '/' and '%'), or 'i' (List index a out of range for length b).
type arithFault struct {
	a, b int64
	op   byte
	at   string
}

// fault is the FaultErr the operation would have raised in place; it must
// run in a deferred function while the panic is in flight, so the stack
// below runtime.gopanic is still the stack of the operation.
func (x *arithFault) fault() *FaultErr {
	var msg string
	switch {
	case x.op == 'i':
		msg = fmt.Sprintf("index %d out of range for List of length %d", x.a, x.b)
	case (x.op == '/' || x.op == '%') && x.b == 0:
		msg = fmt.Sprintf("division by zero: %s %c 0", istr(x.a), x.op)
	default:
		msg = fmt.Sprintf("integer overflow: %s %c %s does not fit Int", istr(x.a), x.op, istr(x.b))
	}
	return &FaultErr{Msg: msg, Loc: x.at, Stack: panicStack()}
}

// AddI is checked `a + b` on Int: the sum overflowed when its sign differs
// from the signs of both operands.
func AddI(a, b int64, at string) int64 {
	c := a + b
	if (a^c)&(b^c) < 0 {
		panic(&arithFault{a, b, '+', at})
	}
	return c
}

// SubI is checked `a - b` on Int: the difference overflowed when the
// operands' signs differ and its sign differs from a's.
func SubI(a, b int64, at string) int64 {
	c := a - b
	if (a^b)&(a^c) < 0 {
		panic(&arithFault{a, b, '-', at})
	}
	return c
}

// MulI is checked `a * b` on Int. Operands that fit in 32 bits cannot
// overflow; otherwise the 128 bit product (bits.Mul64, an intrinsic) must
// be the sign extension of its low 64 bits.
func MulI(a, b int64, at string) int64 {
	if uint64(a+1<<31)|uint64(b+1<<31) < 1<<32 {
		return a * b
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	// signed high word: subtract b when a < 0 and a when b < 0
	if int64(hi)-(a>>63)&b-(b>>63)&a != int64(lo)>>63 {
		panic(&arithFault{a, b, '*', at})
	}
	return int64(lo)
}

// Checked Int operations with a constant operand k, emitted when the
// generator knows k (internal/gen/expr.go intOp): the overflow test needs
// one comparison. AddIUp and SubIUp take a k that makes the result grow
// (k > 0 for `+`, k < 0 for `-`), AddIDown and SubIDown one that makes it
// shrink. MulIK and MulKI take the range [lo, hi] of the other operand for
// which the product fits (computed by the generator from k, |k| >= 2);
// MulKI is `k * a`, so its fault prints the operands in source order.

// AddIUp is checked `a + k` for k > 0.
func AddIUp(a, k int64, at string) int64 {
	c := a + k
	if c < a {
		panic(&arithFault{a, k, '+', at})
	}
	return c
}

// AddIDown is checked `a + k` for k < 0.
func AddIDown(a, k int64, at string) int64 {
	c := a + k
	if c > a {
		panic(&arithFault{a, k, '+', at})
	}
	return c
}

// SubIUp is checked `a - k` for k < 0.
func SubIUp(a, k int64, at string) int64 {
	c := a - k
	if c < a {
		panic(&arithFault{a, k, '-', at})
	}
	return c
}

// SubIDown is checked `a - k` for k > 0.
func SubIDown(a, k int64, at string) int64 {
	c := a - k
	if c > a {
		panic(&arithFault{a, k, '-', at})
	}
	return c
}

// MulIK is checked `a * k` where a must lie in [lo, hi].
func MulIK(a, k, lo, hi int64, at string) int64 {
	if a < lo || a > hi {
		panic(&arithFault{a, k, '*', at})
	}
	return a * k
}

// MulKI is checked `k * a` where a must lie in [lo, hi].
func MulKI(k, a, lo, hi int64, at string) int64 {
	if a < lo || a > hi {
		panic(&arithFault{k, a, '*', at})
	}
	return k * a
}

// DivI is checked `a / b` on Int (truncating).
func DivI(a, b int64, at string) int64 {
	if b == 0 || b == -1 && a == math.MinInt64 {
		panic(&arithFault{a, b, '/', at})
	}
	return a / b
}

// RemI is checked `a % b` on Int.
func RemI(a, b int64, at string) int64 {
	if b == 0 {
		panic(&arithFault{a, b, '%', at})
	}
	if b == -1 {
		return 0
	}
	return a % b
}

// List_AtU is `xs[i]` where the generator's range analysis proved i in range
// (internal/gen/ranges.go). Go's own bounds check stays: if the analysis
// were wrong the program would stop with an internal fault, never read
// out of range.
func List_AtU[T any](l List[T], i int64) T { return l.s[i] }

// List_AtM is `xs[i]`, emitted by default: At with an inline panic as the
// fault, so it inlines.
func List_AtM[T any](l List[T], i int64, at string) T {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	return l.s[i]
}

// List_RefM is the address of element i for reading one field of it in place
// (`xs[i].f`, emitted by default): the generated code reads through it at
// once, so it never sees a later write. Copying a struct of more than
// four fields out of the list (AtM) would cost a memory copy, since Go
// keeps only small structs in registers.
func List_RefM[T any](l List[T], i int64, at string) *T {
	if uint64(i) >= uint64(len(l.s)) {
		panic(&arithFault{i, int64(len(l.s)), 'i', at})
	}
	return &l.s[i]
}

// List_RefU is RefM where the range analysis proved i in range.
func List_RefU[T any](l List[T], i int64) *T { return &l.s[i] }
