package progen

import (
	"fmt"
	"math/rand"
	"strings"
)

// Arith writes the arithmetic program of seed, for differential testing
// of the backends (internal/gen's TestArithProperty): integer code only,
// so every backend compiles it and the comparison is of values, where
// Program covers syntax. Functions of Int parameters with let bindings,
// a mutable accumulator, if and match (as statements and expressions),
// bounded for loops with break and continue, early returns, and calls
// to earlier functions (so every program ends). Divisors cannot be zero;
// arithmetic can overflow, which faults, and the fault is part of the
// output compared.
func Arith(seed int64) string {
	g := &arithGen{r: rand.New(rand.NewSource(seed))}
	nf := 2 + g.r.Intn(4)
	for i := 0; i < nf; i++ {
		g.function(i)
	}
	g.main(nf)
	return g.b.String()
}

type arithGen struct {
	r      *rand.Rand
	b      strings.Builder
	ind    int
	nfn    int      // functions defined so far (callable)
	arity  []int    // parameter counts of the functions
	vars   []string // Int bindings in scope
	next   int      // fresh name counter
	loops  int      // enclosing for loops
	budget int      // statements left in this function
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

func (g *arithGen) function(i int) {
	n := 1 + g.r.Intn(3)
	var ps []string
	g.vars = nil
	for k := 0; k < n; k++ {
		p := fmt.Sprintf("p%d", k)
		ps = append(ps, p+": Int")
		g.vars = append(g.vars, p)
	}
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
	g.arity = append(g.arity, n)
	g.nfn = i + 1
}

// block emits statements at nesting depth d; the bindings it declares
// go out of scope after it, and each is added to acc so it is used.
func (g *arithGen) block(d int) {
	outer := len(g.vars)
	n := 1 + g.r.Intn(3)
	for k := 0; k < n && g.budget > 0; k++ {
		g.budget--
		g.stmt(d)
	}
	for _, v := range g.vars[outer:] {
		g.line("acc += %s", v)
	}
	g.vars = g.vars[:outer]
}

func (g *arithGen) stmt(d int) {
	switch c := g.r.Intn(10); {
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
		g.line("return %s", g.expr(2))
		g.ind--
		g.line("}")
	default:
		g.line("acc += %s", g.expr(3))
	}
}

// matchSubject is an Int expression with a small range of values.
func (g *arithGen) matchSubject() string {
	return fmt.Sprintf("((%s) %% %d)", g.expr(2), 2+g.r.Intn(4))
}

func (g *arithGen) atom() string {
	if len(g.vars) > 0 && g.r.Intn(3) > 0 {
		return g.vars[g.r.Intn(len(g.vars))]
	}
	if g.r.Intn(2) == 0 {
		return "acc"
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
	switch c := g.r.Intn(14); {
	case c < 3:
		return g.atom()
	case c < 6:
		op := []string{"+", "-", "*"}[g.r.Intn(3)]
		return fmt.Sprintf("(%s %s %s)", g.expr(depth-1), op, g.expr(depth-1))
	case c == 6:
		// never zero: x % 7 is in -6..6
		op := []string{"/", "%"}[g.r.Intn(2)]
		return fmt.Sprintf("(%s %s ((%s %% 7) + 8))", g.expr(depth-1), op, g.expr(depth-1))
	case c == 7:
		op := []string{"&", "|", "^"}[g.r.Intn(3)]
		return fmt.Sprintf("(%s %s %s)", g.expr(depth-1), op, g.expr(depth-1))
	case c == 8:
		op := []string{"<<", ">>"}[g.r.Intn(2)]
		return fmt.Sprintf("(%s %s (%s & 7))", g.expr(depth-1), op, g.expr(depth-1))
	case c == 9:
		return fmt.Sprintf("(if %s { %s } else { %s })", g.cond(depth-1), g.expr(depth-1), g.expr(depth-1))
	case c == 10:
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
	}
	return g.atom()
}

func (g *arithGen) cond(depth int) string {
	switch c := g.r.Intn(6); {
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
		return fmt.Sprintf("(%s %s %s)", a, map[bool]string{true: "&&", false: "||"}[c == 3], b)
	}
	return fmt.Sprintf("!%s", g.cond(depth-1))
}

func (g *arithGen) main(nf int) {
	g.line("fn main(os: Os) {")
	g.ind++
	g.vars = nil
	for i := 0; i < nf; i++ {
		var args []string
		for k := 0; k < g.arity[i]; k++ {
			args = append(args, fmt.Sprint(g.r.Intn(60)-10))
		}
		g.line(`os.stdio.println("f%d ${f%d(%s)}")`, i, i, strings.Join(args, ", "))
	}
	g.ind--
	g.line("}")
}
