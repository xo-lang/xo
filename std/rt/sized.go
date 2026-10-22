package rt

// Checked arithmetic on the sized integer types (Int8 to Int32, UInt8 to
// UInt64), one non-generic function per type and operation. rt.AddS and
// its family are generic, and Go does not inline them (the shape copies
// cost 80 to 240 against a budget of 80, and a fault call costs about 57
// of it), so every checked `a % 251` of a UInt16 was a call: the portable
// group ran 5 to 6x the native backend. These inline: the fault is a
// panic of a *sizedFault (a panic costs about one node), the message and
// the stack are built when it is recovered, as for *arithFault (mono.go),
// and the operands of 32 bits or less are widened to int64 or uint64,
// where overflow is a range test (UInt64 uses bits.Mul64, an intrinsic).
// compiler/gen emits them (binop); the file is written by
// scripts/gen-rt-sized.py (python3 scripts/gen-rt-sized.py std/rt/sized.go).

import (
	"fmt"
	"math/bits"
)

// sizedNames are the Xo names of the types, by sizedFault.ty.
var sizedNames = [...]string{"Int", "Int8", "Int16", "Int32", "UInt8", "UInt16", "UInt32", "UInt64"}

// sizedFault is the panic value of a failed checked operation on a sized
// integer type: op is '+', '-', '*', '/', '%' (overflow, or division by
// zero when b is 0 for '/' and '%'), 'r' (shift amount b out of range)
// or 'l' (a << b lost bits; b is the shift count, a signed int64);
// ty indexes sizedNames; a and b are the operands widened to int64
// (the bits of a UInt64).
type sizedFault struct {
	a, b int64
	op   byte
	ty   uint8
	at   string
}

func (x *sizedFault) str(v int64) string {
	if x.ty == 7 {
		return fmt.Sprintf("%d", uint64(v))
	}
	return fmt.Sprintf("%d", v)
}

// fault is the FaultErr the operation would have raised in place; like
// arithFault.fault it runs in a deferred function while the panic is in
// flight.
func (x *sizedFault) fault() *FaultErr {
	var msg string
	name := sizedNames[x.ty]
	switch {
	case x.op == 'r':
		msg = fmt.Sprintf("shift amount %d out of range for %s", x.b, name)
	case x.op == 'l':
		msg = fmt.Sprintf("integer overflow: %s << %d does not fit %s", x.str(x.a), x.b, name)
	case (x.op == '/' || x.op == '%') && x.b == 0:
		msg = fmt.Sprintf("division by zero: %s %c 0", x.str(x.a), x.op)
	default:
		msg = fmt.Sprintf("integer overflow: %s %c %s does not fit %s", x.str(x.a), x.op, x.str(x.b), name)
	}
	return &FaultErr{Msg: msg, Loc: x.at, Stack: panicStack()}
}

// int8: checked + - * / % and << >> on Int8.
func Add_int8(a, b int8, at string) int8 {
	c := int64(a) + int64(b)
	if c != int64(int8(c)) {
		panic(&sizedFault{int64(a), int64(b), '+', 1, at})
	}
	return int8(c)
}

func Sub_int8(a, b int8, at string) int8 {
	c := int64(a) - int64(b)
	if c != int64(int8(c)) {
		panic(&sizedFault{int64(a), int64(b), '-', 1, at})
	}
	return int8(c)
}

func Mul_int8(a, b int8, at string) int8 {
	c := int64(a) * int64(b)
	if c != int64(int8(c)) {
		panic(&sizedFault{int64(a), int64(b), '*', 1, at})
	}
	return int8(c)
}

func Div_int8(a, b int8, at string) int8 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 1, at})
	}
	c := int64(a) / int64(b)
	if c != int64(int8(c)) {
		panic(&sizedFault{int64(a), int64(b), '/', 1, at})
	}
	return int8(c)
}

func Rem_int8(a, b int8, at string) int8 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 1, at})
	}
	if b == -1 {
		return 0
	}
	return a % b
}

// Shl_int8 is `a << n` with the count widened to int64 by the caller.
func Shl_int8(a int8, n int64, at string) int8 {
	if n < 0 || n >= 8 {
		panic(&sizedFault{int64(a), n, 'r', 1, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 1, at})
	}
	return c
}

func Shr_int8(a int8, n int64, at string) int8 {
	if n < 0 || n >= 8 {
		panic(&sizedFault{int64(a), n, 'r', 1, at})
	}
	return a >> uint(n)
}

// int16: checked + - * / % and << >> on Int16.
func Add_int16(a, b int16, at string) int16 {
	c := int64(a) + int64(b)
	if c != int64(int16(c)) {
		panic(&sizedFault{int64(a), int64(b), '+', 2, at})
	}
	return int16(c)
}

func Sub_int16(a, b int16, at string) int16 {
	c := int64(a) - int64(b)
	if c != int64(int16(c)) {
		panic(&sizedFault{int64(a), int64(b), '-', 2, at})
	}
	return int16(c)
}

func Mul_int16(a, b int16, at string) int16 {
	c := int64(a) * int64(b)
	if c != int64(int16(c)) {
		panic(&sizedFault{int64(a), int64(b), '*', 2, at})
	}
	return int16(c)
}

func Div_int16(a, b int16, at string) int16 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 2, at})
	}
	c := int64(a) / int64(b)
	if c != int64(int16(c)) {
		panic(&sizedFault{int64(a), int64(b), '/', 2, at})
	}
	return int16(c)
}

func Rem_int16(a, b int16, at string) int16 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 2, at})
	}
	if b == -1 {
		return 0
	}
	return a % b
}

// Shl_int16 is `a << n` with the count widened to int64 by the caller.
func Shl_int16(a int16, n int64, at string) int16 {
	if n < 0 || n >= 16 {
		panic(&sizedFault{int64(a), n, 'r', 2, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 2, at})
	}
	return c
}

func Shr_int16(a int16, n int64, at string) int16 {
	if n < 0 || n >= 16 {
		panic(&sizedFault{int64(a), n, 'r', 2, at})
	}
	return a >> uint(n)
}

// int32: checked + - * / % and << >> on Int32.
func Add_int32(a, b int32, at string) int32 {
	c := int64(a) + int64(b)
	if c != int64(int32(c)) {
		panic(&sizedFault{int64(a), int64(b), '+', 3, at})
	}
	return int32(c)
}

func Sub_int32(a, b int32, at string) int32 {
	c := int64(a) - int64(b)
	if c != int64(int32(c)) {
		panic(&sizedFault{int64(a), int64(b), '-', 3, at})
	}
	return int32(c)
}

func Mul_int32(a, b int32, at string) int32 {
	c := int64(a) * int64(b)
	if c != int64(int32(c)) {
		panic(&sizedFault{int64(a), int64(b), '*', 3, at})
	}
	return int32(c)
}

func Div_int32(a, b int32, at string) int32 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 3, at})
	}
	c := int64(a) / int64(b)
	if c != int64(int32(c)) {
		panic(&sizedFault{int64(a), int64(b), '/', 3, at})
	}
	return int32(c)
}

func Rem_int32(a, b int32, at string) int32 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 3, at})
	}
	if b == -1 {
		return 0
	}
	return a % b
}

// Shl_int32 is `a << n` with the count widened to int64 by the caller.
func Shl_int32(a int32, n int64, at string) int32 {
	if n < 0 || n >= 32 {
		panic(&sizedFault{int64(a), n, 'r', 3, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 3, at})
	}
	return c
}

func Shr_int32(a int32, n int64, at string) int32 {
	if n < 0 || n >= 32 {
		panic(&sizedFault{int64(a), n, 'r', 3, at})
	}
	return a >> uint(n)
}

// uint8: checked + - * / % and << >> on UInt8.
func Add_uint8(a, b uint8, at string) uint8 {
	c := uint64(a) + uint64(b)
	if c != uint64(uint8(c)) {
		panic(&sizedFault{int64(a), int64(b), '+', 4, at})
	}
	return uint8(c)
}

func Sub_uint8(a, b uint8, at string) uint8 {

	if b > a {
		panic(&sizedFault{int64(a), int64(b), '-', 4, at})
	}
	return a - b
}

func Mul_uint8(a, b uint8, at string) uint8 {
	c := uint64(a) * uint64(b)
	if c != uint64(uint8(c)) {
		panic(&sizedFault{int64(a), int64(b), '*', 4, at})
	}
	return uint8(c)
}

func Div_uint8(a, b uint8, at string) uint8 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 4, at})
	}
	return a / b
}

func Rem_uint8(a, b uint8, at string) uint8 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 4, at})
	}
	return a % b
}

// Shl_uint8 is `a << n` with the count widened to int64 by the caller.
func Shl_uint8(a uint8, n int64, at string) uint8 {
	if n < 0 || n >= 8 {
		panic(&sizedFault{int64(a), n, 'r', 4, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 4, at})
	}
	return c
}

func Shr_uint8(a uint8, n int64, at string) uint8 {
	if n < 0 || n >= 8 {
		panic(&sizedFault{int64(a), n, 'r', 4, at})
	}
	return a >> uint(n)
}

// uint16: checked + - * / % and << >> on UInt16.
func Add_uint16(a, b uint16, at string) uint16 {
	c := uint64(a) + uint64(b)
	if c != uint64(uint16(c)) {
		panic(&sizedFault{int64(a), int64(b), '+', 5, at})
	}
	return uint16(c)
}

func Sub_uint16(a, b uint16, at string) uint16 {

	if b > a {
		panic(&sizedFault{int64(a), int64(b), '-', 5, at})
	}
	return a - b
}

func Mul_uint16(a, b uint16, at string) uint16 {
	c := uint64(a) * uint64(b)
	if c != uint64(uint16(c)) {
		panic(&sizedFault{int64(a), int64(b), '*', 5, at})
	}
	return uint16(c)
}

func Div_uint16(a, b uint16, at string) uint16 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 5, at})
	}
	return a / b
}

func Rem_uint16(a, b uint16, at string) uint16 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 5, at})
	}
	return a % b
}

// Shl_uint16 is `a << n` with the count widened to int64 by the caller.
func Shl_uint16(a uint16, n int64, at string) uint16 {
	if n < 0 || n >= 16 {
		panic(&sizedFault{int64(a), n, 'r', 5, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 5, at})
	}
	return c
}

func Shr_uint16(a uint16, n int64, at string) uint16 {
	if n < 0 || n >= 16 {
		panic(&sizedFault{int64(a), n, 'r', 5, at})
	}
	return a >> uint(n)
}

// uint32: checked + - * / % and << >> on UInt32.
func Add_uint32(a, b uint32, at string) uint32 {
	c := uint64(a) + uint64(b)
	if c != uint64(uint32(c)) {
		panic(&sizedFault{int64(a), int64(b), '+', 6, at})
	}
	return uint32(c)
}

func Sub_uint32(a, b uint32, at string) uint32 {

	if b > a {
		panic(&sizedFault{int64(a), int64(b), '-', 6, at})
	}
	return a - b
}

func Mul_uint32(a, b uint32, at string) uint32 {
	c := uint64(a) * uint64(b)
	if c != uint64(uint32(c)) {
		panic(&sizedFault{int64(a), int64(b), '*', 6, at})
	}
	return uint32(c)
}

func Div_uint32(a, b uint32, at string) uint32 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 6, at})
	}
	return a / b
}

func Rem_uint32(a, b uint32, at string) uint32 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 6, at})
	}
	return a % b
}

// Shl_uint32 is `a << n` with the count widened to int64 by the caller.
func Shl_uint32(a uint32, n int64, at string) uint32 {
	if n < 0 || n >= 32 {
		panic(&sizedFault{int64(a), n, 'r', 6, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 6, at})
	}
	return c
}

func Shr_uint32(a uint32, n int64, at string) uint32 {
	if n < 0 || n >= 32 {
		panic(&sizedFault{int64(a), n, 'r', 6, at})
	}
	return a >> uint(n)
}

// uint64: checked + - * / % and << >> on UInt64.
func Add_uint64(a, b uint64, at string) uint64 {
	c := a + b
	if c < a {
		panic(&sizedFault{int64(a), int64(b), '+', 7, at})
	}
	return c
}

func Sub_uint64(a, b uint64, at string) uint64 {
	if b > a {
		panic(&sizedFault{int64(a), int64(b), '-', 7, at})
	}
	return a - b
}

func Mul_uint64(a, b uint64, at string) uint64 {
	hi, lo := bits.Mul64(a, b)
	if hi != 0 {
		panic(&sizedFault{int64(a), int64(b), '*', 7, at})
	}
	return lo
}

func Div_uint64(a, b uint64, at string) uint64 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '/', 7, at})
	}
	return a / b
}

func Rem_uint64(a, b uint64, at string) uint64 {
	if b == 0 {
		panic(&sizedFault{int64(a), int64(b), '%', 7, at})
	}
	return a % b
}

// Shl_uint64 is `a << n` with the count widened to int64 by the caller.
func Shl_uint64(a uint64, n int64, at string) uint64 {
	if n < 0 || n >= 64 {
		panic(&sizedFault{int64(a), n, 'r', 7, at})
	}
	c := a << uint(n)
	if c>>uint(n) != a {
		panic(&sizedFault{int64(a), n, 'l', 7, at})
	}
	return c
}

func Shr_uint64(a uint64, n int64, at string) uint64 {
	if n < 0 || n >= 64 {
		panic(&sizedFault{int64(a), n, 'r', 7, at})
	}
	return a >> uint(n)
}
