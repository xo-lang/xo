package progen

import (
	"fmt"
	"math/rand"
	"strings"
)

// Arith writes the arithmetic program of seed, for differential testing
// of the backends (internal/gen's TestArithProperty): code every backend
// compiles (--no-gc included), so the comparison is of values, where
// Program covers syntax. Functions of Int parameters with let bindings,
// a mutable accumulator, if and match (as statements and expressions),
// bounded for loops with break and continue, early returns, and calls
// to earlier functions (so every program ends). Expressions reach
// Float (printed, compared, converted), Str, List[Int], a struct, an
// enum with payloads, and closures that capture, each folded back into
// Int; Str functions print Float and Str values directly.
//
// Integer operands lean to edge values (0, -1, 63, 64, the minimum and
// maximum Int), shifts go up to 63 (or any amount, rarely), and
// divisors include -1 next to divisors that cannot be zero, so the
// minimum divided by -1 occurs. Arithmetic can overflow and shifts can
// be out of range, which faults; the fault is part of the output
// compared.
func Arith(seed int64) string {
	g := &arithGen{r: rand.New(rand.NewSource(seed)), tv: map[string][]string{}}
	g.b.WriteString(arithPreamble)
	nf := 2 + g.r.Intn(4)
	for i := 0; i < nf; i++ {
		g.function(i)
	}
	ns := g.r.Intn(3)
	for i := 0; i < ns; i++ {
		g.strFunction(i)
	}
	g.main(nf, ns)
	return g.b.String()
}

const arithPreamble = `struct P {
  a: Int
  b: Int
}

enum E {
  A(n: Int)
  B(x: Int, y: Int)
  C
}

`

// The types of the typed (non Int) values: a let binding of one is in
// tv[type] until its block ends.
const (
	tFloat = "Float"
	tStr   = "Str"
	tList  = "List"
	tP     = "P"
	tE     = "E"
	tFn    = "Fn"
)

var typedKinds = []string{tFloat, tStr, tList, tP, tE, tFn}

type arithGen struct {
	r      *rand.Rand
	b      strings.Builder
	ind    int
	nfn    int                 // functions defined so far (callable)
	arity  []int               // parameter counts of the functions
	vars   []string            // Int bindings in scope
	tv     map[string][]string // typed bindings in scope, by type
	next   int                 // fresh name counter
	loops  int                 // enclosing for loops
	budget int                 // statements left in this function
	inline int                 // > 0: inside `${}`, no newlines (no match)
}

// in generates f's expression for a `${}`: on one line.
func (g *arithGen) in(f func() string) string {
	g.inline++
	defer func() { g.inline-- }()
	return f()
}

func (g *arithGen) line(format string, args ...any) {
	g.b.WriteString(strings.Repeat("  ", g.ind))
	fmt.Fprintf(&g.b, format, args...)
	g.b.WriteByte('\n')
}

func (g *arithGen) fresh(prefix string) string {
	g.next++
	return fmt.Sprintf("%s%d", prefix, g.next)
}

func (g *arithGen) params() []string {
	n := 1 + g.r.Intn(3)
	var ps []string
	g.vars = nil
	g.tv = map[string][]string{}
	for k := 0; k < n; k++ {
		p := fmt.Sprintf("p%d", k)
		ps = append(ps, p+": Int")
		g.vars = append(g.vars, p)
	}
	return ps
}

func (g *arithGen) function(i int) {
	ps := g.params()
	g.line("fn f%d(%s) -> Int {", i, strings.Join(ps, ", "))
	g.ind++
	// every parameter is read here, so none is unused
	g.line("var acc = %s", strings.Join(g.vars, " + "))
	g.budget = 6 + g.r.Intn(10)
	g.block(0)
	g.line("acc + %s", g.expr(2))
	g.ind--
	g.line("}")
	g.line("")
	g.arity = append(g.arity, len(ps))
	g.nfn = i + 1
}

// strFunction writes s<i>, which shows Float and Str values (their
// formatting is compared too) after a block like f<i>'s.
func (g *arithGen) strFunction(i int) {
	ps := g.params()
	g.line("fn s%d(%s) -> Str {", i, strings.Join(ps, ", "))
	g.ind++
	g.line("var acc = %s", strings.Join(g.vars, " + "))
	g.budget = 4 + g.r.Intn(6)
	g.block(0)
	a := g.in(func() string { return g.texpr(tFloat, 2) })
	b := g.in(func() string { return g.texpr(tStr, 2) })
	c := g.in(func() string { return g.texpr(tFloat, 1) })
	g.line(`"${acc} ${%s} ${%s} ${%s}"`, a, b, c)
	g.ind--
	g.line("}")
	g.line("")
}

// block emits statements at nesting depth d; the bindings it declares
// go out of scope after it, and each is added to acc so it is used.
func (g *arithGen) block(d int) {
	outer := len(g.vars)
	touter := map[string]int{}
	for _, k := range typedKinds {
		touter[k] = len(g.tv[k])
	}
	n := 1 + g.r.Intn(3)
	for k := 0; k < n && g.budget > 0; k++ {
		g.budget--
		g.stmt(d)
	}
	for _, v := range g.vars[outer:] {
		g.line("acc += %s", v)
	}
	g.vars = g.vars[:outer]
	for _, k := range typedKinds {
		for _, v := range g.tv[k][touter[k]:] {
			g.line("acc += %s", g.fold(k, v))
		}
		g.tv[k] = g.tv[k][:touter[k]]
	}
}

func (g *arithGen) stmt(d int) {
	switch c := g.r.Intn(12); {
	case c < 3 || d >= 3:
		v := g.fresh("v")
		g.line("let %s = %s", v, g.expr(3))
		g.vars = append(g.vars, v)
	case c == 3:
		g.line("acc = %s", g.expr(3))
	case c == 4:
		g.line("if %s {", g.cond(2))
		g.ind++
		g.block(d + 1)
		g.ind--
		if g.r.Intn(2) == 0 {
			g.line("} else {")
			g.ind++
			g.block(d + 1)
			g.ind--
		}
		g.line("}")
	case c == 5:
		i := g.fresh("i")
		g.line("for %s in 0..%d {", i, 1+g.r.Intn(6))
		g.ind++
		g.loops++
		g.vars = append(g.vars, i)
		g.line("acc += %s", i)
		if g.r.Intn(3) == 0 {
			g.line("if %s {", g.cond(1))
			g.ind++
			if g.r.Intn(2) == 0 {
				g.line("continue")
			} else {
				g.line("break")
			}
			g.ind--
			g.line("}")
		}
		g.block(d + 1)
		g.vars = g.vars[:len(g.vars)-1]
		g.loops--
		g.ind--
		g.line("}")
	case c == 6:
		g.line("match %s {", g.matchSubject())
		g.ind++
		for k := 0; k < 1+g.r.Intn(3); k++ {
			g.line("%d => {", k)
			g.ind++
			g.block(d + 1)
			g.ind--
			g.line("}")
		}
		g.line("_ => {")
		g.ind++
		g.block(d + 1)
		g.ind--
		g.line("}")
		g.ind--
		g.line("}")
	case c == 7 && d > 0:
		g.line("if %s {", g.cond(1))
		g.ind++
		g.line("return %s", g.retExpr())
		g.ind--
		g.line("}")
	case c == 8 || c == 9:
		// a typed binding, folded into acc when its block ends
		k := typedKinds[g.r.Intn(len(typedKinds))]
		v := g.fresh("t")
		g.line("let %s = %s", v, g.texpr(k, 2))
		g.tv[k] = append(g.tv[k], v)
	case c == 10:
		// a mutable list grown in a loop, then summed
		l, i, x := g.fresh("l"), g.fresh("i"), g.fresh("x")
		g.line("var %s = %s", l, g.texpr(tList, 1))
		g.line("for %s in 0..%d {", i, 1+g.r.Intn(4))
		g.ind++
		g.vars = append(g.vars, i)
		g.line("%s.push(%s + %s)", l, i, g.expr(1))
		g.vars = g.vars[:len(g.vars)-1]
		g.ind--
		g.line("}")
		g.line("for %s in %s {", x, l)
		g.ind++
		g.vars = append(g.vars, x)
		g.line("acc = %s", g.expr(1))
		g.line("acc += %s", x)
		g.vars = g.vars[:len(g.vars)-1]
		g.ind--
		g.line("}")
	default:
		g.line("acc += %s", g.expr(3))
	}
}

// retExpr is what an early return returns: Int in f<i>, Str in s<i>,
// which differ by their accumulator's use (both have acc).
func (g *arithGen) retExpr() string {
	if strings.HasPrefix(g.curFn(), "fn s") {
		return fmt.Sprintf(`"r${%s}"`, g.in(func() string { return g.expr(2) }))
	}
	return g.expr(2)
}

// curFn is the header of the function being written.
func (g *arithGen) curFn() string {
	s := g.b.String()
	i := strings.LastIndex(s, "\nfn ")
	return s[i+1:]
}

// fold is an Int that depends on the typed value v of kind k.
func (g *arithGen) fold(k, v string) string {
	switch k {
	case tFloat:
		c := g.r.Intn(3)
		if strings.Contains(v, "\n") {
			c = 1
		}
		switch c {
		case 0:
			// its formatting
			return fmt.Sprintf(`"${%s}".len()`, v)
		case 1:
			return fmt.Sprintf("(if %s.is_nan() { 7 } else { if %s < 0.5 { 3 } else { 5 } })", v, v)
		}
		return fmt.Sprintf(`(Int.parse("${%s}") ?? (-2))`, v)
	case tStr:
		if g.r.Intn(2) == 0 {
			return fmt.Sprintf("%s.len()", v)
		}
		return fmt.Sprintf("(Int.parse(%s) ?? %s.rune_count())", v, v)
	case tList:
		x := g.fresh("x")
		return fmt.Sprintf("(%s.len() + %s.fold(0, fn(a: Int, %s: Int) -> Int { a ^ %s }))", v, v, x, x)
	case tP:
		return fmt.Sprintf("(%s.a ^ %s.b)", v, v)
	case tE:
		if g.inline > 0 {
			return fmt.Sprintf("(if %s == E.C { 1 } else { 2 })", v)
		}
		return g.matchE(v, 1)
	case tFn:
		return fmt.Sprintf("%s(%s)", v, g.atom())
	}
	panic("progen: fold of " + k)
}

// matchE is an exhaustive match on an E value.
func (g *arithGen) matchE(v string, depth int) string {
	n, x, y := g.fresh("n"), g.fresh("x"), g.fresh("y")
	g.vars = append(g.vars, n)
	a := g.expr(depth)
	g.vars = append(g.vars[:len(g.vars)-1], x, y)
	b := fmt.Sprintf("(%s ^ %s)", x, y)
	if g.r.Intn(2) == 0 {
		b = fmt.Sprintf("(%s - %s)", x, y)
	}
	g.vars = g.vars[:len(g.vars)-2]
	c := g.expr(depth)
	// A's binding may go unread in a, so A reads it through an add
	ind := strings.Repeat("  ", g.ind+1)
	return fmt.Sprintf("(match %s {\n%sA(%s) => %s + %s\n%sB(%s, %s) => %s\n%sC => %s\n%s})", v, ind, n, n, a, ind, x, y, b, ind, c, strings.Repeat("  ", g.ind))
}

// matchSubject is an Int expression with a small range of values.
func (g *arithGen) matchSubject() string {
	return fmt.Sprintf("((%s) %% %d)", g.expr(2), 2+g.r.Intn(4))
}

// Edge operands: small ones are common, huge ones rare (they overflow
// most operations, and a fault ends the program).
var (
	smallEdges = []string{"0", "1", "(-1)", "63", "64", "2147483647", "(-2147483648)"}
	hugeEdges  = []string{"9223372036854775807", "(-9223372036854775807 - 1)", "4611686018427387904", "(-4611686018427387904)"}
)

func (g *arithGen) edge() string {
	if g.r.Intn(5) == 0 {
		return hugeEdges[g.r.Intn(len(hugeEdges))]
	}
	return smallEdges[g.r.Intn(len(smallEdges))]
}

func (g *arithGen) atom() string {
	if len(g.vars) > 0 && g.r.Intn(3) > 0 {
		return g.vars[g.r.Intn(len(g.vars))]
	}
	switch g.r.Intn(8) {
	case 0, 1, 2:
		return "acc"
	case 3:
		return g.edge()
	}
	n := g.r.Intn(120) - 20
	if n < 0 {
		return fmt.Sprintf("(%d)", n)
	}
	return fmt.Sprint(n)
}

func (g *arithGen) expr(depth int) string {
	if depth <= 0 {
		return g.atom()
	}
	switch c := g.r.Intn(19); {
	case c < 3:
		return g.atom()
	case c < 6:
		op := []string{"+", "-", "*"}[g.r.Intn(3)]
		return fmt.Sprintf("(%s %s %s)", g.expr(depth-1), op, g.expr(depth-1))
	case c == 6:
		op := []string{"/", "%"}[g.r.Intn(2)]
		switch g.r.Intn(4) {
		case 0:
			// -1 or 1: the minimum / -1 overflows
			return fmt.Sprintf("(%s %s ((%s %% 2) | 1))", g.expr(depth-1), op, g.expr(depth-1))
		case 1:
			return fmt.Sprintf("(%s %s (-1))", g.expr(depth-1), op)
		}
		// never zero: x % 7 is in -6..6
		return fmt.Sprintf("(%s %s ((%s %% 7) + 8))", g.expr(depth-1), op, g.expr(depth-1))
	case c == 7:
		op := []string{"&", "|", "^"}[g.r.Intn(3)]
		return fmt.Sprintf("(%s %s %s)", g.expr(depth-1), op, g.expr(depth-1))
	case c == 8:
		op := []string{"<<", ">>"}[g.r.Intn(2)]
		mask := "7"
		switch g.r.Intn(6) {
		case 0, 1:
			mask = "63"
		case 2:
			// any amount: 64 and above, and negative ones, fault
			if g.r.Intn(4) == 0 {
				return fmt.Sprintf("(%s %s %s)", g.expr(depth-1), op, g.expr(depth-1))
			}
		}
		return fmt.Sprintf("(%s %s (%s & %s))", g.expr(depth-1), op, g.expr(depth-1), mask)
	case c == 9:
		return fmt.Sprintf("(if %s { %s } else { %s })", g.cond(depth-1), g.expr(depth-1), g.expr(depth-1))
	case c == 10 && g.inline == 0:
		var arms []string
		for k := 0; k < 1+g.r.Intn(3); k++ {
			arms = append(arms, fmt.Sprintf("%d => %s", k, g.expr(depth-1)))
		}
		ind := strings.Repeat("  ", g.ind+1)
		return fmt.Sprintf("(match %s {\n%s%s\n%s_ => %s\n%s})", g.matchSubject(), ind, strings.Join(arms, "\n"+ind), ind, g.expr(depth-1), strings.Repeat("  ", g.ind))
	case c == 11 && g.nfn > 0:
		f := g.r.Intn(g.nfn)
		var args []string
		for k := 0; k < g.arity[f]; k++ {
			args = append(args, g.expr(depth-1))
		}
		return fmt.Sprintf("f%d(%s)", f, strings.Join(args, ", "))
	case c == 12:
		return fmt.Sprintf("(-%s)", g.expr(depth-1))
	case c >= 13:
		return g.typedInt(depth)
	}
	return g.atom()
}

// typedInt is an Int computed through a typed value.
func (g *arithGen) typedInt(depth int) string {
	k := typedKinds[g.r.Intn(len(typedKinds))]
	if vs := g.tv[k]; len(vs) > 0 && g.r.Intn(2) == 0 {
		return g.fold(k, vs[g.r.Intn(len(vs))])
	}
	switch k {
	case tFloat:
		switch g.r.Intn(4) {
		case 0:
			op := []string{"<", "<=", ">", ">=", "==", "!="}[g.r.Intn(6)]
			return fmt.Sprintf("(if %s %s %s { %s } else { %s })", g.texpr(tFloat, depth-1), op, g.texpr(tFloat, depth-1), g.expr(depth-1), g.expr(depth-1))
		case 1:
			// in range: truncates toward zero
			return fmt.Sprintf("Int((Float(%s %% 1000000) / %s).%s())", g.expr(depth-1), []string{"7.0", "(-2.5)", "0.3", "1e3"}[g.r.Intn(4)], []string{"floor", "ceil", "round", "trunc"}[g.r.Intn(4)])
		case 2:
			// any value: NaN and out of range fault
			if g.r.Intn(3) == 0 {
				return fmt.Sprintf("Int(%s)", g.texpr(tFloat, depth-1))
			}
		}
		return g.fold(tFloat, g.texpr(tFloat, depth-1))
	case tList:
		l := g.texpr(tList, depth-1)
		switch g.r.Intn(3) {
		case 0:
			return fmt.Sprintf("(%s.get(%s) ?? %s)", l, g.expr(depth-1), g.atom())
		case 1:
			return fmt.Sprintf("(%s.first() ?? %s)", l, g.atom())
		}
		return g.fold(tList, l)
	case tE:
		if g.inline > 0 {
			return g.fold(tE, g.texpr(tE, depth-1))
		}
		return g.matchE(g.texpr(tE, depth-1), depth-1)
	case tP:
		return fmt.Sprintf("%s.%s", g.texpr(tP, depth-1), []string{"a", "b"}[g.r.Intn(2)])
	}
	return g.fold(k, g.texpr(k, depth-1))
}

var (
	floatLits = []string{"0.0", "(-0.0)", "0.5", "1.0", "(-1.5)", "0.1", "3.25", "1e308", "(-1e308)", "1e-310", "123456789.125", "1e21", "1e20", "0.000001", "1e-7", "4.9e-324", "9007199254740993.0"}
	strLits   = []string{`""`, `"a"`, `"xo"`, `"12"`, `"-3"`, `"héllo"`, `" pad "`, `"9223372036854775807"`, `"1e3"`}
)

// texpr is an expression of the typed kind k.
func (g *arithGen) texpr(k string, depth int) string {
	if vs := g.tv[k]; len(vs) > 0 && g.r.Intn(3) == 0 {
		return vs[g.r.Intn(len(vs))]
	}
	switch k {
	case tFloat:
		if depth <= 0 {
			if g.r.Intn(2) == 0 {
				return floatLits[g.r.Intn(len(floatLits))]
			}
			return fmt.Sprintf("Float(%s)", g.atom())
		}
		switch g.r.Intn(7) {
		case 0, 1:
			op := []string{"+", "-", "*", "/", "%"}[g.r.Intn(5)]
			return fmt.Sprintf("(%s %s %s)", g.texpr(tFloat, depth-1), op, g.texpr(tFloat, depth-1))
		case 2:
			return fmt.Sprintf("(-%s)", g.texpr(tFloat, depth-1))
		case 3:
			return fmt.Sprintf("%s.%s()", g.texpr(tFloat, depth-1), []string{"floor", "ceil", "round", "trunc", "abs", "sqrt"}[g.r.Intn(6)])
		case 4:
			return fmt.Sprintf("(if %s { %s } else { %s })", g.cond(depth-1), g.texpr(tFloat, depth-1), g.texpr(tFloat, depth-1))
		case 5:
			return fmt.Sprintf("(Float.parse(%s) ?? %s)", g.texpr(tStr, depth-1), floatLits[g.r.Intn(len(floatLits))])
		}
		return fmt.Sprintf("Float(%s)", g.expr(depth-1))
	case tStr:
		if depth <= 0 {
			return strLits[g.r.Intn(len(strLits))]
		}
		switch g.r.Intn(7) {
		case 0:
			return fmt.Sprintf(`"${%s}"`, g.in(func() string { return g.expr(depth - 1) }))
		case 1:
			return fmt.Sprintf(`"f${%s}"`, g.in(func() string { return g.texpr(tFloat, depth-1) }))
		case 2:
			return fmt.Sprintf("(%s + %s)", g.texpr(tStr, depth-1), g.texpr(tStr, depth-1))
		case 3:
			return fmt.Sprintf("%s.repeat(%s & 3)", g.texpr(tStr, depth-1), g.expr(depth-1))
		case 4:
			return fmt.Sprintf("%s.%s()", g.texpr(tStr, depth-1), []string{"to_upper", "to_lower", "trim"}[g.r.Intn(3)])
		case 5:
			return fmt.Sprintf("(if %s { %s } else { %s })", g.cond(depth-1), g.texpr(tStr, depth-1), g.texpr(tStr, depth-1))
		}
		return fmt.Sprintf(`%s.replace("1", "-")`, g.texpr(tStr, depth-1))
	case tList:
		if depth <= 0 {
			return fmt.Sprintf("[%s]", g.atom())
		}
		switch g.r.Intn(6) {
		case 0, 1:
			var xs []string
			for i := 0; i < 1+g.r.Intn(4); i++ {
				xs = append(xs, g.expr(depth-1))
			}
			return "[" + strings.Join(xs, ", ") + "]"
		case 2:
			return fmt.Sprintf("(%s + %s)", g.texpr(tList, depth-1), g.texpr(tList, depth-1))
		case 3:
			return fmt.Sprintf("%s.%s()", g.texpr(tList, depth-1), []string{"reverse", "sort"}[g.r.Intn(2)])
		case 4:
			x := g.fresh("x")
			l := g.texpr(tList, depth-1)
			g.vars = append(g.vars, x)
			e := g.expr(depth - 1)
			g.vars = g.vars[:len(g.vars)-1]
			return fmt.Sprintf("%s.map(fn(%s: Int) -> Int { %s - %s })", l, x, x, e)
		}
		x := g.fresh("x")
		l := g.texpr(tList, depth-1)
		g.vars = append(g.vars, x)
		e := g.expr(depth - 1)
		g.vars = g.vars[:len(g.vars)-1]
		return fmt.Sprintf("%s.filter(fn(%s: Int) -> Bool { %s > %s })", l, x, x, e)
	case tP:
		if depth > 0 && g.r.Intn(3) == 0 {
			return fmt.Sprintf("(%s with { %s: %s })", g.texpr(tP, depth-1), []string{"a", "b"}[g.r.Intn(2)], g.expr(depth-1))
		}
		return fmt.Sprintf("P{a: %s, b: %s}", g.expr(depth-1), g.expr(depth-1))
	case tE:
		switch g.r.Intn(4) {
		case 0:
			return "E.C"
		case 1:
			return fmt.Sprintf("E.A(n: %s)", g.expr(depth-1))
		case 2:
			if depth > 0 {
				return fmt.Sprintf("(if %s { %s } else { %s })", g.cond(depth-1), g.texpr(tE, depth-1), g.texpr(tE, depth-1))
			}
		}
		return fmt.Sprintf("E.B(x: %s, y: %s)", g.expr(depth-1), g.expr(depth-1))
	case tFn:
		// a closure over the bindings in scope; the parameter is read
		x := g.fresh("x")
		g.vars = append(g.vars, x)
		e := g.expr(depth)
		g.vars = g.vars[:len(g.vars)-1]
		op := []string{"+", "-", "^", "*"}[g.r.Intn(4)]
		return fmt.Sprintf("fn(%s: Int) -> Int { %s %s %s }", x, x, op, e)
	}
	panic("progen: texpr of " + k)
}

func (g *arithGen) cond(depth int) string {
	switch c := g.r.Intn(7); {
	case c < 3 || depth <= 0:
		op := []string{"<", "<=", ">", ">=", "==", "!="}[g.r.Intn(6)]
		return fmt.Sprintf("(%s %s %s)", g.expr(depth), op, g.expr(depth))
	case c == 3 || c == 4:
		// two different sides: `a && a` is legal but go vet reports the
		// Go it becomes (redundant and), which is the program's doing
		a, b := g.cond(depth-1), g.cond(depth-1)
		for k := 0; a == b && k < 8; k++ {
			b = g.cond(depth - 1)
		}
		if a == b {
			return a
		}
		// go vet also reports `x != 0 || x != 78` (suspect or) and
		// `x == 0 && x == 78` (suspect and): two such compares join with
		// the other operator
		and := c == 3
		if strings.Contains(a, " != ") && strings.Contains(b, " != ") {
			and = true
		} else if strings.Contains(a, " == ") && strings.Contains(b, " == ") {
			and = false
		}
		return fmt.Sprintf("(%s %s %s)", a, map[bool]string{true: "&&", false: "||"}[and], b)
	case c == 5:
		switch g.r.Intn(3) {
		case 0:
			return fmt.Sprintf("(%s == %s)", g.texpr(tStr, depth-1), g.texpr(tStr, depth-1))
		case 1:
			return fmt.Sprintf("%s.contains(%s)", g.texpr(tList, depth-1), g.expr(depth-1))
		}
		return fmt.Sprintf("(%s < %s)", g.texpr(tFloat, depth-1), g.texpr(tFloat, depth-1))
	}
	return fmt.Sprintf("!%s", g.cond(depth-1))
}

func (g *arithGen) main(nf, ns int) {
	g.line("fn main(os: Os) {")
	g.ind++
	g.vars = nil
	g.tv = map[string][]string{}
	for i := 0; i < nf; i++ {
		var args []string
		for k := 0; k < g.arity[i]; k++ {
			args = append(args, g.mainArg())
		}
		g.line(`os.stdio.println("f%d ${f%d(%s)}")`, i, i, strings.Join(args, ", "))
	}
	for i := 0; i < ns; i++ {
		g.line(`os.stdio.println("s%d ${s%d(%s)}")`, i, i, strings.Join(g.sParams(i), ", "))
	}
	g.ind--
	g.line("}")
}

func (g *arithGen) mainArg() string {
	if g.r.Intn(10) == 0 {
		return g.edge()
	}
	return fmt.Sprint(g.r.Intn(60) - 10)
}

// sParams are arguments for s<i>, whose arity is read from its header.
func (g *arithGen) sParams(i int) []string {
	s := g.b.String()
	h := s[strings.Index(s, fmt.Sprintf("fn s%d(", i)):]
	h = h[:strings.Index(h, ")")]
	var args []string
	for k := 0; k < strings.Count(h, ": Int"); k++ {
		args = append(args, g.mainArg())
	}
	return args
}
