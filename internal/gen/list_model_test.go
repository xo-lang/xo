package gen_test

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestListModel generates programs that copy, capture, send, and mutate
// lists through every path the generator lowers (bindings, fields,
// elements of lists of lists and of structs, Map values, Option and
// tuple payloads, closures, channels, Mutex.with, spawned tasks, `var`
// parameters that alias each other or a by value argument, loops over a
// list the body writes) and compares the output with a model in which
// every assignment is a deep copy. Lists write in place when they own
// their array (std/rt/list.go), so any missed ownership mark shows up as
// a value that changed through another copy.
//
// XO_LIST_SEEDS=N runs N more seeds (slow; for exploring).
func TestListModel(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	seeds := []int64{20261008, 7, 99}
	race := false
	if out, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err == nil && strings.TrimSpace(string(out)) == "1" {
		race = true
	}
	if n, err := strconv.Atoi(os.Getenv("XO_LIST_SEEDS")); err == nil {
		for i := 1; i <= n; i++ {
			seeds = append(seeds, int64(1000+i))
		}
	}
	for i, seed := range seeds {
		opts := build.Options{Mode: gen.Build}
		switch i {
		case 1:
			// the generic lowering marks through rt.Own and MarkP
			opts.NoMono = true
		case 2:
			// the plain (non atomic) block fields are only touched by
			// one task while a block is unshared: a race report means a
			// list escaped to another task unmarked
			if !race {
				continue
			}
			opts.Race = true
		}
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			src, want := listModelProgram(seed, 12, 40)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			if d := os.Getenv("XO_LIST_DUMP"); d != "" {
				// keep the program for debugging
				os.WriteFile(filepath.Join(d, fmt.Sprintf("list-model-%d.xo", seed)), []byte(src), 0o644)
			}
			got, _ := buildAndRunOpts(t, dir, opts)
			if got.exit != 0 || got.stdout != want {
				gl, wl := strings.Split(got.stdout, "\n"), strings.Split(want, "\n")
				for k := 0; k < len(gl) || k < len(wl); k++ {
					var g, w string
					if k < len(gl) {
						g = gl[k]
					}
					if k < len(wl) {
						w = wl[k]
					}
					if g != w {
						t.Errorf("first difference at output line %d\n got: %s\nwant: %s", k+1, g, w)
						break
					}
				}
				os.WriteFile(filepath.Join(os.TempDir(), fmt.Sprintf("xo-list-model-%d.xo", seed)), []byte(src), 0o644)
				t.Fatalf("exit %d, stderr:\n%s\n(program saved to %s)", got.exit, got.stderr, filepath.Join(os.TempDir(), fmt.Sprintf("xo-list-model-%d.xo", seed)))
			}
		})
	}
}

// lmS is the model of `struct S { xs: List[Int], n: Int }`.
type lmS struct {
	xs []int
	n  int
}

// lm is the model state of one scenario: every value is owned (deep
// copies on every assignment).
type lm struct {
	r       *rand.Rand
	b       strings.Builder
	out     strings.Builder
	ind     string
	lists   map[string][]int
	grids   map[string][][]int
	structs map[string]lmS
	ss      []lmS
	m       map[int][]int
	mkeys   []int
	snaps   []string // name=value lines printed at the end
	n       int      // fresh name counter
	sc      string   // scenario name
}

func cp(xs []int) []int { return append([]int{}, xs...) }

func cpg(g [][]int) [][]int {
	out := make([][]int, len(g))
	for i, r := range g {
		out[i] = cp(r)
	}
	return out
}

func cps(s lmS) lmS { return lmS{xs: cp(s.xs), n: s.n} }

func showL(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ",")
}

func showG(g [][]int) string {
	var b strings.Builder
	for _, r := range g {
		b.WriteString("[" + showL(r) + "]")
	}
	return b.String()
}

func showS(s lmS) string { return fmt.Sprintf("%d:[%s]", s.n, showL(s.xs)) }

func showSS(ss []lmS) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = showS(s)
	}
	return strings.Join(parts, " ")
}

func (s *lm) showM() string {
	parts := make([]string, len(s.mkeys))
	for i, k := range s.mkeys {
		parts[i] = fmt.Sprintf("%d=%s", k, showL(s.m[k]))
	}
	return strings.Join(parts, ";")
}

func (s *lm) line(format string, args ...any) {
	s.b.WriteString(s.ind + fmt.Sprintf(format, args...) + "\n")
}

func (s *lm) fresh(p string) string {
	s.n++
	return fmt.Sprintf("%s%d", p, s.n)
}

func (s *lm) pick(xs ...string) string { return xs[s.r.Intn(len(xs))] }

func (s *lm) val() int { return s.r.Intn(90) + 1 }

// listVar picks a List[Int] binding, nonempty if need.
func (s *lm) listVar(need bool) string {
	var c []string
	for _, n := range []string{"a", "b", "c"} {
		if !need || len(s.lists[n]) > 0 {
			c = append(c, n)
		}
	}
	if len(c) == 0 {
		return ""
	}
	return s.pick(c...)
}

func (s *lm) gridRow(need bool) (string, int) {
	type gr struct {
		g string
		i int
	}
	var c []gr
	for _, g := range []string{"g", "h"} {
		for i, r := range s.grids[g] {
			if !need || len(r) > 0 {
				c = append(c, gr{g, i})
			}
		}
	}
	if len(c) == 0 {
		return "", 0
	}
	x := c[s.r.Intn(len(c))]
	return x.g, x.i
}

func (s *lm) snap(name, value string) { s.snaps = append(s.snaps, name+"="+value) }

// op emits one random operation and applies it to the model.
func (s *lm) op() {
	r := s.r
	switch r.Intn(35) {
	case 0: // copy between list bindings
		x, y := s.listVar(false), s.listVar(false)
		if x == y {
			return
		}
		s.line("%s = %s", x, y)
		s.lists[x] = cp(s.lists[y])
	case 1, 2: // element write
		x := s.listVar(true)
		if x == "" {
			return
		}
		i, v := r.Intn(len(s.lists[x])), s.val()
		s.line("%s[%d] = %d", x, i, v)
		s.lists[x][i] = v
	case 3: // compound element write
		x := s.listVar(true)
		if x == "" {
			return
		}
		i, v := r.Intn(len(s.lists[x])), s.val()
		s.line("%s[%d] += %d", x, i, v)
		s.lists[x][i] += v
	case 4: // push
		x, v := s.listVar(false), s.val()
		s.line("%s.push(%d)", x, v)
		s.lists[x] = append(s.lists[x], v)
	case 5: // pop
		x := s.listVar(true)
		if x == "" {
			return
		}
		s.line("let _ = %s.pop()", x)
		s.lists[x] = s.lists[x][:len(s.lists[x])-1]
	case 6: // insert and remove_at
		x := s.listVar(false)
		i, v := r.Intn(len(s.lists[x])+1), s.val()
		s.line("%s.insert(%d, %d)", x, i, v)
		s.lists[x] = append(s.lists[x][:i], append([]int{v}, s.lists[x][i:]...)...)
		if r.Intn(2) == 0 {
			j := r.Intn(len(s.lists[x]))
			s.line("let _ = %s.remove_at(%d)", x, j)
			s.lists[x] = append(s.lists[x][:j], s.lists[x][j+1:]...)
		}
	case 7: // set
		x := s.listVar(true)
		if x == "" {
			return
		}
		i, v := r.Intn(len(s.lists[x])), s.val()
		s.line("%s.set(%d, %d)", x, i, v)
		s.lists[x][i] = v
	case 8: // snapshot with let
		k := s.fresh("k")
		switch r.Intn(5) {
		case 0:
			x := s.listVar(false)
			s.line("let %s = %s", k, x)
			s.snap(k, "${show("+k+")}")
			s.snapWant(k, showL(s.lists[x]))
		case 1:
			g := s.pick("g", "h")
			s.line("let %s = %s", k, g)
			s.snapWant(k, showG(s.grids[g]))
			s.snap(k, "${showg("+k+")}")
		case 2:
			st := s.pick("s", "t")
			s.line("let %s = %s", k, st)
			s.snapWant(k, showS(s.structs[st]))
			s.snap(k, "${shows("+k+")}")
		case 3:
			s.line("let %s = ss", k)
			s.snapWant(k, showSS(s.ss))
			s.snap(k, "${showss("+k+")}")
		case 4:
			s.line("let %s = m", k)
			s.snapWant(k, s.showM())
			s.snap(k, "${showm("+k+")}")
		}
	case 9: // closure capture
		f := s.fresh("f")
		switch r.Intn(4) {
		case 0:
			x := s.listVar(false)
			s.line("let %s = fn() { show(%s) }", f, x)
			s.snapWant(f, showL(s.lists[x]))
		case 1:
			g := s.pick("g", "h")
			s.line("let %s = fn() { showg(%s) }", f, g)
			s.snapWant(f, showG(s.grids[g]))
		case 2:
			x := s.listVar(false)
			s.line("let %s = reader(%s)", f, x)
			s.snapWant(f, showL(s.lists[x]))
		case 3:
			st := s.pick("s", "t")
			s.line("let %s = fn() { shows(%s) }", f, st)
			s.snapWant(f, showS(s.structs[st]))
		}
		s.snap(f, "${"+f+"()}")
	case 10: // var parameter
		x := s.listVar(true)
		if x == "" {
			return
		}
		i, v := r.Intn(len(s.lists[x])), s.val()
		s.line("setv(%s, %d, %d)", x, i, v)
		s.lists[x][i] = v
	case 11: // by value argument
		x, y := s.listVar(true), s.listVar(false)
		if x == "" {
			return
		}
		i := r.Intn(len(s.lists[x]))
		s.line("%s[%d] = total(%s)", x, i, y)
		t := 0
		for _, e := range s.lists[y] {
			t += e
		}
		s.lists[x][i] = t
	case 12: // two var parameters, maybe the same binding
		x, y := s.listVar(false), s.listVar(true)
		if y == "" {
			return
		}
		v := s.val()
		s.line("both(%s, %s, %d)", x, y, v)
		snap := cp(s.lists[x])
		for _, e := range snap {
			s.lists[y][len(s.lists[y])-1] += e
		}
		s.lists[x] = append(s.lists[x], v)
	case 13: // var parameter and a by value argument, maybe the same
		x, y := s.listVar(false), s.listVar(false)
		s.line("pushsum(%s, %s)", x, y)
		t := 0
		for _, e := range s.lists[y] {
			t += e
		}
		s.lists[x] = append(s.lists[x], t)
		s.lists[x][0]++
	case 14: // grid element writes
		g, i := s.gridRow(true)
		if g == "" {
			return
		}
		j, v := r.Intn(len(s.grids[g][i])), s.val()
		if r.Intn(2) == 0 {
			s.line("%s[%d][%d] = %d", g, i, j, v)
			s.grids[g][i][j] = v
		} else {
			s.line("%s[%d][%d] += %d", g, i, j, v)
			s.grids[g][i][j] += v
		}
	case 15: // grid row push through the place, or through a var parameter
		g, i := s.gridRow(false)
		if g == "" {
			return
		}
		v := s.val()
		if r.Intn(2) == 0 {
			s.line("%s[%d].push(%d)", g, i, v)
		} else {
			s.line("rowpush(%s, %d, %d)", g, i, v)
		}
		s.grids[g][i] = append(s.grids[g][i], v)
	case 16: // rows in and out of grids
		g := s.pick("g", "h")
		x := s.listVar(false)
		switch {
		case r.Intn(3) == 0 || len(s.grids[g]) == 0:
			s.line("%s.push(id(%s))", g, x)
			s.grids[g] = append(s.grids[g], cp(s.lists[x]))
		case r.Intn(2) == 0:
			i := r.Intn(len(s.grids[g]))
			s.line("%s = %s[%d]", x, g, i)
			s.lists[x] = cp(s.grids[g][i])
		default:
			i := r.Intn(len(s.grids[g]))
			s.line("%s[%d] = id(%s)", g, i, x)
			s.grids[g][i] = cp(s.lists[x])
		}
	case 17: // whole grid copies and a generic swap of rows
		if r.Intn(2) == 0 {
			x, y := s.pick("g", "h"), s.pick("g", "h")
			if x == y {
				return
			}
			s.line("%s = %s", x, y)
			s.grids[x] = cpg(s.grids[y])
			return
		}
		g := s.pick("g", "h")
		if len(s.grids[g]) < 2 {
			return
		}
		i, j := r.Intn(len(s.grids[g])), r.Intn(len(s.grids[g]))
		s.line("swap(%s, %d, %d)", g, i, j)
		s.grids[g][i], s.grids[g][j] = s.grids[g][j], s.grids[g][i]
	case 18: // struct field list writes
		st := s.pick("s", "t")
		v := s.val()
		switch {
		case len(s.structs[st].xs) > 0 && r.Intn(2) == 0:
			i := r.Intn(len(s.structs[st].xs))
			s.line("%s.xs[%d] = %d", st, i, v)
			s.structs[st].xs[i] = v
		case r.Intn(2) == 0:
			s.line("%s.xs.push(%d)", st, v)
			x := s.structs[st]
			x.xs = append(x.xs, v)
			s.structs[st] = x
		default:
			s.line("%s.n += %d", st, v)
			x := s.structs[st]
			x.n += v
			s.structs[st] = x
		}
	case 19: // structs and lists in and out of structs
		st := s.pick("s", "t")
		x := s.listVar(false)
		switch r.Intn(3) {
		case 0:
			o := "t"
			if st == "t" {
				o = "s"
			}
			s.line("%s = %s", st, o)
			s.structs[st] = cps(s.structs[o])
		case 1:
			s.line("%s = %s.xs", x, st)
			s.lists[x] = cp(s.structs[st].xs)
		case 2:
			s.line("%s.xs = %s", st, x)
			y := s.structs[st]
			y.xs = cp(s.lists[x])
			s.structs[st] = y
		}
	case 20: // list of structs
		st := s.pick("s", "t")
		switch {
		case len(s.ss) == 0 || r.Intn(4) == 0:
			s.line("ss.push(ids(%s))", st)
			s.ss = append(s.ss, cps(s.structs[st]))
		case r.Intn(3) == 0:
			i := r.Intn(len(s.ss))
			s.line("%s = ss[%d]", st, i)
			s.structs[st] = cps(s.ss[i])
		case r.Intn(2) == 0:
			i, v := r.Intn(len(s.ss)), s.val()
			if len(s.ss[i].xs) > 0 && r.Intn(2) == 0 {
				j := r.Intn(len(s.ss[i].xs))
				s.line("ss[%d].xs[%d] = %d", i, j, v)
				s.ss[i].xs[j] = v
			} else {
				s.line("ss[%d].xs.push(%d)", i, v)
				s.ss[i].xs = append(s.ss[i].xs, v)
			}
		default:
			i, v := r.Intn(len(s.ss)), s.val()
			s.line("ss[%d].n = %d", i, v)
			s.ss[i].n = v
		}
	case 21: // Map values
		k, v := r.Intn(3), s.val()
		x := s.listVar(false)
		add := func() {
			if _, ok := s.m[k]; !ok {
				s.mkeys = append(s.mkeys, k)
				s.m[k] = nil
			}
		}
		switch r.Intn(4) {
		case 0:
			s.line("m.update(%d, [], fn(var xs) { xs.push(%d) })", k, v)
			add()
			s.m[k] = append(s.m[k], v)
		case 1:
			s.line("m.update(%d, [], fn(var xs) {\n%s  if xs.len() > 0 {\n%s    xs[0] = %d\n%s  }\n%s})", k, s.ind, s.ind, v, s.ind, s.ind)
			add()
			if len(s.m[k]) > 0 {
				s.m[k][0] = v
			}
		case 2:
			s.line("%s = m.get(%d) ?? []", x, k)
			s.lists[x] = cp(s.m[k])
		case 3:
			s.line("m[%d] = id(%s)", k, x)
			add()
			s.m[k] = cp(s.lists[x])
		}
	case 22: // a loop over a list its body writes
		x := s.listVar(true)
		if x == "" {
			return
		}
		if r.Intn(2) == 0 {
			i := r.Intn(len(s.lists[x]))
			s.line("for e in %s {\n%s  %s[%d] = %s[%d] + e\n%s}", x, s.ind, x, i, x, i, s.ind)
			for _, e := range cp(s.lists[x]) {
				s.lists[x][i] += e
			}
		} else {
			s.line("for e in %s {\n%s  %s.push(e)\n%s}", x, s.ind, x, s.ind)
			s.lists[x] = append(s.lists[x], cp(s.lists[x])...)
		}
	case 23: // a loop over a grid its body writes
		g := s.pick("g", "h")
		if len(s.grids[g]) == 0 {
			return
		}
		i := r.Intn(len(s.grids[g]))
		s.line("for row in %s {\n%s  %s[%d].push(row.len())\n%s}", g, s.ind, g, i, s.ind)
		for _, row := range cpg(s.grids[g]) {
			s.grids[g][i] = append(s.grids[g][i], len(row))
		}
	case 24: // channel
		x, y := s.listVar(true), s.listVar(false)
		if x == "" {
			return
		}
		ch := s.fresh("ch")
		i, v := r.Intn(len(s.lists[x])), s.val()
		s.line("let %s = Chan[List[Int]].new(cap: 1)", ch)
		s.line("%s.send(%s)", ch, x)
		sent := cp(s.lists[x])
		s.line("%s[%d] = %d", x, i, v)
		s.lists[x][i] = v
		s.line("%s = %s.recv() ?? []", y, ch)
		s.lists[y] = sent
	case 25: // Mutex
		x, y := s.listVar(false), s.listVar(false)
		mu := s.fresh("mu")
		v, w := s.val(), s.val()
		s.line("let %s = Mutex[List[Int]].new(%s)", mu, x)
		held := cp(s.lists[x])
		s.line("%s.with(fn(var xs) { xs.push(%d) })", mu, v)
		held = append(held, v)
		if len(held) > 0 {
			s.line("let _ = %s.with(fn(var xs) -> Int ! Str {\n%s  xs[0] = %d\n%s  Err(\"no\")\n%s}).ignore_err(\"model: the write must not be kept\")", mu, s.ind, w, s.ind, s.ind)
		}
		s.line("%s.push(%d)", x, w)
		s.lists[x] = append(s.lists[x], w)
		s.line("%s = %s.with(fn(xs) { xs })", y, mu)
		s.lists[y] = held
	case 26: // a spawned task copies a list while the parent writes it
		x, y := s.listVar(false), s.listVar(false)
		ch := s.fresh("ch")
		v, w := s.val(), s.val()
		s.line("let %s = Chan[List[Int]].new(cap: 1)", ch)
		s.line("scope sc {\n%s  sc.spawn(fn() {\n%s    var mine = %s\n%s    mine.push(%d)\n%s    %s.send(mine)\n%s  })\n%s  %s.push(%d)\n%s}", s.ind, s.ind, x, s.ind, v, s.ind, ch, s.ind, s.ind, x, w, s.ind)
		theirs := append(cp(s.lists[x]), v)
		s.lists[x] = append(s.lists[x], w)
		s.line("%s = %s.recv() ?? []", y, ch)
		s.lists[y] = theirs
	case 27: // Option and tuple payloads
		x, y := s.listVar(true), s.listVar(false)
		if x == "" {
			return
		}
		o := s.fresh("o")
		i, v := r.Intn(len(s.lists[x])), s.val()
		if r.Intn(2) == 0 {
			s.line("let %s: List[Int]? = Some(%s)", o, x)
			held := cp(s.lists[x])
			s.line("%s[%d] = %d", x, i, v)
			s.lists[x][i] = v
			s.line("if let Some(z) = %s {\n%s  %s = z\n%s}", o, s.ind, y, s.ind)
			s.lists[y] = held
		} else {
			s.line("var %s = (%s, 1)", o, x)
			held := cp(s.lists[x])
			s.line("%s.0.push(%d)", o, v)
			held = append(held, v)
			s.line("%s[%d] = %d", x, i, v)
			s.lists[x][i] = v
			s.line("%s = %s.0", y, o)
			s.lists[y] = held
		}
	case 28: // concatenation and slices
		x, y := s.listVar(false), s.listVar(false)
		z := s.listVar(false)
		if r.Intn(2) == 0 {
			s.line("%s = %s + %s", z, x, y)
			s.lists[z] = append(cp(s.lists[x]), s.lists[y]...)
		} else {
			n := len(s.lists[x])
			lo := r.Intn(n + 1)
			hi := lo + r.Intn(n-lo+1)
			s.line("%s = %s[%d..%d]", z, x, lo, hi)
			s.lists[z] = cp(s.lists[x][lo:hi])
		}
	case 29: // a list returned from a function, then written
		x, y := s.listVar(false), s.listVar(false)
		s.line("%s = grown(%s)", y, x)
		g := append(cp(s.lists[x]), 7)
		g[0] = -1
		s.lists[y] = g
	case 30: // std results that copy elements out of a grid
		g := s.pick("g", "h")
		if len(s.grids[g]) == 0 {
			return
		}
		x := s.listVar(false)
		switch r.Intn(3) {
		case 0:
			s.line("%s = %s.reverse()[0]", x, g)
			s.lists[x] = cp(s.grids[g][len(s.grids[g])-1])
		case 1:
			s.line("%s = %s.first() ?? []", x, g)
			s.lists[x] = cp(s.grids[g][0])
		case 2:
			s.line("%s = %s.filter(fn(r) { true })[0]", x, g)
			s.lists[x] = cp(s.grids[g][0])
		}
	case 31: // a nested block expression that writes the list it reads
		x := s.listVar(true)
		if x == "" {
			return
		}
		y := s.listVar(false)
		i, v := r.Intn(len(s.lists[x])), s.val()
		s.line("%s = [%s[%d], {\n%s  %s[%d] = %d\n%s  %s[%d]\n%s}]", y, x, i, s.ind, x, i, v, s.ind, x, i, s.ind)
		old := s.lists[x][i]
		s.lists[x][i] = v
		s.lists[y] = []int{old, v}
	case 32: // an element of a list of structs read field by field
		if len(s.ss) == 0 {
			return
		}
		x := s.listVar(true)
		if x == "" {
			return
		}
		i := r.Intn(len(s.ss))
		j := r.Intn(len(s.lists[x]))
		s.line("%s[%d] = ss[%d].n + ss[%d].xs.len()", x, j, i, i)
		s.lists[x][j] = s.ss[i].n + len(s.ss[i].xs)
	case 33: // pop then push on a copy's shared array
		x := s.listVar(true)
		if x == "" {
			return
		}
		k := s.fresh("k")
		v := s.val()
		s.line("let %s = %s", k, x)
		s.snapWant(k, showL(s.lists[x]))
		s.snap(k, "${show("+k+")}")
		s.line("let _ = %s.pop()", x)
		s.line("%s.push(%d)", x, v)
		s.lists[x][len(s.lists[x])-1] = v
	case 34: // a by value argument read after the `var` one is written
		x, y := s.listVar(true), s.listVar(false)
		if x == "" {
			return
		}
		z := s.listVar(false)
		s.line("%s = [wv(%s, %s)]", z, x, y)
		t := 0
		for _, e := range s.lists[y] {
			t += e
		}
		s.lists[x][len(s.lists[x])-1] = 1000
		s.lists[z] = []int{t}
	}
}

// snapWant records the expected printed value of a snapshot.
func (s *lm) snapWant(name, value string) {
	s.out.WriteString(fmt.Sprintf("%s %s=%s\n", s.sc, name, value))
}

// listModelProgram builds a program of n scenarios of k operations each
// and its expected output.
func listModelProgram(seed int64, n, k int) (string, string) {
	r := rand.New(rand.NewSource(seed))
	var prog, want strings.Builder
	prog.WriteString(`struct S {
  xs: List[Int]
  n: Int
}

fn show(xs: List[Int]) -> Str { xs.map(fn(x) { "${x}" }).join(",") }

fn showg(g: List[List[Int]]) -> Str { g.map(fn(r) { "[${show(r)}]" }).join("") }

fn shows(s: S) -> Str { "${s.n}:[${show(s.xs)}]" }

fn showss(ss: List[S]) -> Str { ss.map(fn(s) { shows(s) }).join(" ") }

fn showm(m: Map[Int, List[Int]]) -> Str { m.entries().map(fn(e) { "${e.0}=${show(e.1)}" }).join(";") }

fn setv(var xs: List[Int], i: Int, v: Int) { xs[i] = v }

fn total(xs: List[Int]) -> Int {
  var t = 0
  for x in xs {
    t += x
  }
  t
}

fn both(var x: List[Int], var y: List[Int], v: Int) {
  for e in x {
    y[y.len() - 1] = y[y.len() - 1] + e
  }
  x.push(v)
}

fn wv(var x: List[Int], y: List[Int]) -> Int {
  x[x.len() - 1] = 1000
  total(y)
}

fn pushsum(var x: List[Int], y: List[Int]) {
  x.push(total(y))
  x[0] = x[0] + 1
}

// id and ids keep stores through a call: XO0520 (lost update) would
// reject a binding stored into a collection and mutated afterwards,
// which is what these programs do on purpose.
fn id(xs: List[Int]) -> List[Int] { xs }

fn ids(s: S) -> S { s }

fn rowpush(var g: List[List[Int]], i: Int, v: Int) { g[i].push(v) }

fn reader(xs: List[Int]) -> fn() -> Str { fn() { show(xs) } }

fn grown(xs: List[Int]) -> List[Int] {
  var c = xs
  c.push(7)
  c[0] = -1
  c
}

fn swap[T](var xs: List[T], i: Int, j: Int) {
  let t = xs[i]
  xs[i] = xs[j]
  xs[j] = t
}

`)
	for sc := 0; sc < n; sc++ {
		s := &lm{r: r, ind: "  ", sc: fmt.Sprintf("sc%d", sc),
			lists:   map[string][]int{"a": {1, 2, 3}, "b": {4, 5}, "c": {}},
			grids:   map[string][][]int{"g": {{1}, {2, 3}}, "h": {{4}}},
			structs: map[string]lmS{"s": {xs: []int{1}, n: 0}, "t": {xs: []int{2, 2}, n: 1}},
			ss:      []lmS{{xs: []int{3}, n: 2}},
			m:       map[int][]int{0: {1}}, mkeys: []int{0},
		}
		s.line("var a: List[Int] = [1, 2, 3]")
		s.line("var b: List[Int] = [4, 5]")
		s.line("var c: List[Int] = []")
		s.line("var g: List[List[Int]] = [[1], [2, 3]]")
		s.line("var h: List[List[Int]] = [[4]]")
		s.line("var s = S{xs: [1], n: 0}")
		s.line("var t = S{xs: [2, 2], n: 1}")
		s.line("var ss: List[S] = [S{xs: [3], n: 2}]")
		s.line("var m: Map[Int, List[Int]] = {0: [1]}")
		for i := 0; i < k; i++ {
			s.op()
		}
		fmt.Fprintf(&prog, "fn %s(os: Os) {\n%s", s.sc, s.b.String())
		fmt.Fprintf(&prog, "  os.stdio.println(\"%s a=${show(a)} b=${show(b)} c=${show(c)}\")\n", s.sc)
		fmt.Fprintf(&prog, "  os.stdio.println(\"%s g=${showg(g)} h=${showg(h)}\")\n", s.sc)
		fmt.Fprintf(&prog, "  os.stdio.println(\"%s s=${shows(s)} t=${shows(t)} ss=${showss(ss)} m=${showm(m)}\")\n", s.sc)
		for _, sn := range s.snaps {
			name, value, _ := strings.Cut(sn, "=")
			fmt.Fprintf(&prog, "  os.stdio.println(\"%s %s=%s\")\n", s.sc, name, value)
		}
		prog.WriteString("}\n\n")
		fmt.Fprintf(&want, "%s a=%s b=%s c=%s\n", s.sc, showL(s.lists["a"]), showL(s.lists["b"]), showL(s.lists["c"]))
		fmt.Fprintf(&want, "%s g=%s h=%s\n", s.sc, showG(s.grids["g"]), showG(s.grids["h"]))
		fmt.Fprintf(&want, "%s s=%s t=%s ss=%s m=%s\n", s.sc, showS(s.structs["s"]), showS(s.structs["t"]), showSS(s.ss), s.showM())
		want.WriteString(s.out.String())
	}
	prog.WriteString("fn main(os: Os) {\n")
	for sc := 0; sc < n; sc++ {
		fmt.Fprintf(&prog, "  sc%d(os)\n", sc)
	}
	prog.WriteString("}\n")
	return prog.String(), want.String()
}
