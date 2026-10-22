import sys
out=sys.argv[1]
types=[("int8",1,"Int8",True,8),("int16",2,"Int16",True,16),("int32",3,"Int32",True,32),
       ("uint8",4,"UInt8",False,8),("uint16",5,"UInt16",False,16),("uint32",6,"UInt32",False,32),
       ("uint64",7,"UInt64",False,64)]
h='''package rt

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
'''
o=[h]
for (g,ty,xn,sg,bw) in types:
    W="int64" if sg else "uint64"
    ws=lambda v:"int64(%s)"%v   # fault operands as int64 bits
    p=lambda op,a,b:"panic(&sizedFault{int64(%s), int64(%s), '%s', %d, at})"%(a,b,op,ty)
    o.append("\n// %s: checked + - * / %% and << >> on %s.\n"%(g,xn))
    if bw<64:
        o.append(f'''func Add_{g}(a, b {g}, at string) {g} {{
	c := {W}(a) + {W}(b)
	if c != {W}({g}(c)) {{
		{p('+','a','b')}
	}}
	return {g}(c)
}}

func Sub_{g}(a, b {g}, at string) {g} {{
	{ 'c := int64(a) - int64(b)' if sg else ''}
	{ 'if c != int64('+g+'(c)) {' if sg else 'if b > a {'}
		{p('-','a','b')}
	}}
	return { g+'(c)' if sg else 'a - b'}
}}

func Mul_{g}(a, b {g}, at string) {g} {{
	c := {W}(a) * {W}(b)
	if c != {W}({g}(c)) {{
		{p('*','a','b')}
	}}
	return {g}(c)
}}
''')
    else:
        o.append(f'''func Add_{g}(a, b {g}, at string) {g} {{
	c := a + b
	if c < a {{
		{p('+','a','b')}
	}}
	return c
}}

func Sub_{g}(a, b {g}, at string) {g} {{
	if b > a {{
		{p('-','a','b')}
	}}
	return a - b
}}

func Mul_{g}(a, b {g}, at string) {g} {{
	hi, lo := bits.Mul64(a, b)
	if hi != 0 {{
		{p('*','a','b')}
	}}
	return lo
}}
''')
    if sg:
        o.append(f'''
func Div_{g}(a, b {g}, at string) {g} {{
	if b == 0 {{
		{p('/','a','b')}
	}}
	c := int64(a) / int64(b)
	if c != int64({g}(c)) {{
		{p('/','a','b')}
	}}
	return {g}(c)
}}

func Rem_{g}(a, b {g}, at string) {g} {{
	if b == 0 {{
		{p('%','a','b')}
	}}
	if b == -1 {{
		return 0
	}}
	return a % b
}}
''')
    else:
        o.append(f'''
func Div_{g}(a, b {g}, at string) {g} {{
	if b == 0 {{
		{p('/','a','b')}
	}}
	return a / b
}}

func Rem_{g}(a, b {g}, at string) {g} {{
	if b == 0 {{
		{p('%','a','b')}
	}}
	return a % b
}}
''')
    o.append(f'''
// Shl_{g} is `a << n` with the count widened to int64 by the caller.
func Shl_{g}(a {g}, n int64, at string) {g} {{
	if n < 0 || n >= {bw} {{
		panic(&sizedFault{{int64(a), n, 'r', {ty}, at}})
	}}
	c := a << uint(n)
	if c>>uint(n) != a {{
		panic(&sizedFault{{int64(a), n, 'l', {ty}, at}})
	}}
	return c
}}

func Shr_{g}(a {g}, n int64, at string) {g} {{
	if n < 0 || n >= {bw} {{
		panic(&sizedFault{{int64(a), n, 'r', {ty}, at}})
	}}
	return a >> uint(n)
}}
''')
open(out,"w").write("".join(o).replace("{\n\n\t","{\n\t"))
