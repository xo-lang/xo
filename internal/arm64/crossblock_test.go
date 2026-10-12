package arm64

import (
	"testing"
)

// Tests of the MIR passes that rely on instructions of other blocks
// (opt.go, lsr.go). Each builds the shape its pass rewrites, checks
// that it fires, then breaks the assumption the rewrite rests on, one
// at a time, and checks that the pass leaves the code alone. The
// programs of testdata/run/cross_block run the same shapes end to end
// (TestRun). gotchas/arm64-reused-flags-of-dropped-compare.md is why.

func mb(id int) *mblock { return &mblock{id: id} }

func jumpTo(t *mblock) *inst { return &inst{kind: iJump, targets: []*mblock{t}} }

func br(text, cond string, uses []vreg, t, f *mblock) *inst {
	return &inst{kind: iCondBr, text: text, cond: cond, uses: uses, targets: []*mblock{t, f}}
}

func pmove(d vreg, s src) *inst {
	in := &inst{kind: iPmove, defs: []vreg{d}, srcs: []src{s}}
	if s.kind == sReg {
		in.uses = []vreg{s.v}
	}
	return in
}

func ret() *inst { return &inst{kind: iRet} }

func newFstate(nvreg int, hot ...*mblock) *fstate {
	return &fstate{fp: make([]bool, nvreg), hot: hot}
}

// threadConst: p sets r to a constant and jumps to j, which only tests
// r. The jump goes straight to the outcome only while r's last
// definition in p is that constant and j is nothing but the test.
func TestThreadConstAssumptions(t *testing.T) {
	const r, q = 1, 2
	type tc struct {
		name   string
		pinsts func(j *mblock) []*inst
		jtest  func(tt, ff *mblock) *inst
		jextra bool // j has an instruction before its test
		want   string
	}
	cbnz := func(tt, ff *mblock) *inst { return br("", "cbnz", []vreg{r}, tt, ff) }
	cases := []tc{
		{"cbnz on 1", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(1)), jumpTo(j)} }, cbnz, false, "T"},
		{"cbnz on 0", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(0)), jumpTo(j)} }, cbnz, false, "F"},
		{"cmp lt", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(3)), jumpTo(j)} },
			func(tt, ff *mblock) *inst { return br("cmp $u0, #5", "lt", []vreg{r}, tt, ff) }, false, "T"},
		{"cmp ge", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(7)), jumpTo(j)} },
			func(tt, ff *mblock) *inst { return br("cmp $u0, #5", "lt", []vreg{r}, tt, ff) }, false, "F"},
		// broken: r is redefined after the constant
		{"redefined", func(j *mblock) []*inst {
			return []*inst{pmove(r, isrc(1)), {kind: iPlain, text: "mov $d0, $u0", defs: []vreg{r}, uses: []vreg{q}}, jumpTo(j)}
		}, cbnz, false, "J"},
		// broken: r comes from a register
		{"register", func(j *mblock) []*inst { return []*inst{pmove(r, rsrc(q)), jumpTo(j)} }, cbnz, false, "J"},
		// broken: j does more than test
		{"j computes", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(1)), jumpTo(j)} }, cbnz, true, "J"},
		// broken: the test is not a compare with an immediate
		{"two uses", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(1)), jumpTo(j)} },
			func(tt, ff *mblock) *inst { return br("cmp $u0, $u1", "lt", []vreg{r, q}, tt, ff) }, false, "J"},
		// broken: a landing check (no uses) tests no register
		{"landing", func(j *mblock) []*inst { return []*inst{pmove(r, isrc(1)), jumpTo(j)} },
			func(tt, ff *mblock) *inst { return br("", "cbnz", nil, tt, ff) }, false, "J"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, j, tt, ff := mb(0), mb(1), mb(2), mb(3)
			p.insts = c.pinsts(j)
			j.insts = []*inst{c.jtest(tt, ff)}
			if c.jextra {
				j.insts = append([]*inst{{kind: iPlain, text: "add $d0, $u0, #1", defs: []vreg{q}, uses: []vreg{q}}}, j.insts...)
			}
			tt.insts, ff.insts = []*inst{ret()}, []*inst{ret()}
			s := newFstate(3, p, j, tt, ff)
			s.threadConst()
			got := map[*mblock]string{j: "J", tt: "T", ff: "F"}[p.term().targets[0]]
			if got != c.want {
				t.Errorf("p jumps to %s, want %s", got, c.want)
			}
		})
	}
}

// threadCset: p computes a Bool with cmp and cset, moves it into r, and
// jumps to j, which only branches on r. p then branches on the compare
// itself, only while that is what r holds and j is only the branch.
func TestThreadCsetAssumptions(t *testing.T) {
	const a, b, c, r, o = 1, 2, 3, 4, 5
	type tc struct {
		name  string
		def   *inst
		mvsrc vreg
		jcond string
		self  bool // j is p
		fires bool
		later bool // a successor of j reads r too
	}
	cset := func(text string) *inst {
		return &inst{kind: iPlain, text: text, defs: []vreg{c}, uses: []vreg{a, b}}
	}
	cases := []tc{
		{"fires", cset("cmp $u0, $u1\ncset $d0, lt"), c, "cbnz", false, true, false},
		{"r read later", cset("cmp $u0, $u1\ncset $d0, lt"), c, "cbnz", false, true, true},
		{"float compare", cset("fcmp $u0, $u1\nfccmp $u0, $u1, #0, vc\ncset $d0, mi"), c, "cbnz", false, true, false},
		// broken: r is not the Bool the compare made
		{"other register", cset("cmp $u0, $u1\ncset $d0, lt"), o, "cbnz", false, false, false},
		// broken: the Bool is not just the cset of the flags
		{"csel after", cset("cmp $u0, $u1\ncset $d0, lt\ncsel $d0, $u0, $d0, eq"), c, "cbnz", false, false, false},
		{"no cset", cset("add $d0, $u0, $u1"), c, "cbnz", false, false, false},
		// broken: j tests r another way
		{"tbz", cset("cmp $u0, $u1\ncset $d0, lt"), c, "tbz", false, false, false},
		// broken: j is p (a loop)
		{"self", cset("cmp $u0, $u1\ncset $d0, lt"), c, "cbnz", true, false, false},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			p, j, tt, ff := mb(0), mb(1), mb(2), mb(3)
			if k.self {
				j = p
			}
			p.insts = []*inst{k.def, pmove(r, rsrc(k.mvsrc)), jumpTo(j)}
			jt := br("", k.jcond, []vreg{r}, tt, ff)
			if k.self {
				p.insts = append(p.insts[:2], jumpTo(p))
			} else {
				j.insts = []*inst{jt}
			}
			tt.insts, ff.insts = []*inst{ret()}, []*inst{ret()}
			if k.later {
				tt.insts = []*inst{{kind: iPlain, text: "str $u0, [sp]", uses: []vreg{r}}, ret()}
			}
			s := newFstate(6, p, j, tt, ff)
			before := k.def.text
			s.threadCset()
			term := p.term()
			fired := term.kind == iCondBr
			if fired != k.fires {
				t.Fatalf("fired %v, want %v (terminator %+v)", fired, k.fires, term)
			}
			if !fired {
				return
			}
			// the branch tests the compare's flags with the cset's
			// condition, to j's targets in j's order
			if term.targets[0] != tt || term.targets[1] != ff {
				t.Errorf("targets are not j's")
			}
			if want := before[:len(before)-len("\ncset $d0, "+term.cond)]; term.text != want {
				t.Errorf("branch text %q, want %q", term.text, want)
			}
			if len(term.uses) != 2 || term.uses[0] != a || term.uses[1] != b {
				t.Errorf("branch uses %v, want the compare's", term.uses)
			}
			// the move into r stays when j's successors read it, and goes
			// when only j's branch did (decision 0184)
			mv := p.insts[len(p.insts)-2]
			kept := mv.kind == iPmove && len(mv.defs) > 0 && mv.defs[0] == r
			if kept != k.later {
				t.Errorf("the move into r kept %v, want %v", kept, k.later)
			}
		})
	}
}

// selectCompares: p branches on `x == y` to e or n, which each only
// compare and branch to the same two blocks. p then computes both
// compares and selects, only while e and n have no other predecessor
// and agree on their targets.
func TestSelectComparesAssumptions(t *testing.T) {
	const k1, k2, i1, i2 = 1, 2, 3, 4
	type tc struct {
		name   string
		extraE bool // another block jumps to e
		nOther bool // n branches to another block
		eTwo   bool // e computes before its compare
		fires  bool
	}
	cases := []tc{
		{"fires", false, false, false, true},
		{"e has two predecessors", true, false, false, false},
		{"targets differ", false, true, false, false},
		{"e computes", false, false, true, false},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			p, e, n, tt, ff, g, q := mb(0), mb(1), mb(2), mb(3), mb(4), mb(5), mb(6)
			p.insts = []*inst{br("cmp $u0, $u1", "ne", []vreg{k1, k2}, n, e)}
			e.insts = []*inst{br("cmp $u0, $u1", "lt", []vreg{i1, i2}, tt, ff)}
			if k.eTwo {
				e.insts = append([]*inst{{kind: iPlain, text: "add $d0, $u0, #1", defs: []vreg{i1}, uses: []vreg{i1}}}, e.insts...)
			}
			nf := ff
			if k.nOther {
				nf = g
			}
			n.insts = []*inst{br("cmp $u0, $u1", "lt", []vreg{k1, k2}, tt, nf)}
			tt.insts, ff.insts, g.insts = []*inst{ret()}, []*inst{ret()}, []*inst{ret()}
			hot := []*mblock{p, e, n, tt, ff, g}
			if k.extraE {
				q.insts = []*inst{jumpTo(e)}
				hot = append(hot, q)
			}
			s := newFstate(5, hot...)
			s.selectCompares()
			fired := p.term().cond == "tbnz"
			if fired != k.fires {
				t.Fatalf("fired %v, want %v", fired, k.fires)
			}
			if fired && (p.term().targets[0] != tt || p.term().targets[1] != ff) {
				t.Errorf("the select branches elsewhere")
			}
			if !fired && (p.term().targets[0] != n || p.term().targets[1] != e) {
				t.Errorf("p's branch changed although the pass did not fire")
			}
		})
	}
}

// reuseFlags: x only branches on the compare its one predecessor p
// ended with, so x's compare is dropped; not when x has another
// predecessor, the compares differ, or p's branch sets no flags.
func TestReuseFlagsAssumptions(t *testing.T) {
	const a, b, c = 1, 2, 3
	type tc struct {
		name  string
		pterm func(x, f *mblock) *inst
		xuses []vreg
		extra bool // another block jumps to x
		drops bool
	}
	cmp := func(x, f *mblock) *inst { return br("cmp $u0, $u1", "ne", []vreg{a, b}, x, f) }
	cases := []tc{
		{"drops", cmp, []vreg{a, b}, false, true},
		{"other registers", cmp, []vreg{a, c}, false, false},
		{"two predecessors", cmp, []vreg{a, b}, true, false},
		{"p jumps", func(x, f *mblock) *inst { return jumpTo(x) }, []vreg{a, b}, false, false},
		{"p cbnz", func(x, f *mblock) *inst { return br("", "cbnz", []vreg{a}, x, f) }, []vreg{a, b}, false, false},
		{"p other compare", func(x, f *mblock) *inst { return br("cmn $u0, $u1", "ne", []vreg{a, b}, x, f) }, []vreg{a, b}, false, false},
	}
	for _, k := range cases {
		t.Run(k.name, func(t *testing.T) {
			p, x, f, tt, ff, q := mb(0), mb(1), mb(2), mb(3), mb(4), mb(5)
			p.insts = []*inst{k.pterm(x, f)}
			x.insts = []*inst{br("cmp $u0, $u1", "lt", k.xuses, tt, ff)}
			f.insts, tt.insts, ff.insts = []*inst{ret()}, []*inst{ret()}, []*inst{ret()}
			hot := []*mblock{p, x, f, tt, ff}
			if k.extra {
				q.insts = []*inst{jumpTo(x)}
				hot = append(hot, q)
			}
			s := newFstate(4, hot...)
			s.reuseFlags()
			dropped := x.insts[0].text == ""
			if dropped != k.drops {
				t.Errorf("dropped %v, want %v", dropped, k.drops)
			}
		})
	}
}

// placeLatches moves a loop's latch after the test that skips an inner
// loop, and marks it copyable: emission then copies it after branches
// into it, so a latch may be copied only when it is moves and plain
// instructions without fault exits (a copied fault stub, or a recover,
// would be emitted twice under one label).
func TestPlaceLatchesAssumptions(t *testing.T) {
	const i, n, f, j = 1, 2, 3, 4
	build := func(latch []*inst) (*fstate, *mblock) {
		// H: i < n ? B : X;  B: flags ? PH : L;  PH: j = i, -> IH;
		// IH: j < n ? IB : L;  IB: j += 1, -> IH;  L: latch, -> H;  X: ret
		h, b, ih, ib, l, x, ph := mb(0), mb(1), mb(2), mb(3), mb(4), mb(5), mb(6)
		h.insts = []*inst{br("cmp $u0, $u1", "lt", []vreg{i, n}, b, x)}
		b.insts = []*inst{br("", "cbnz", []vreg{f}, ph, l)}
		ph.insts = []*inst{pmove(j, rsrc(i)), jumpTo(ih)}
		ih.insts = []*inst{br("cmp $u0, $u1", "lt", []vreg{j, n}, ib, l)}
		ib.insts = []*inst{{kind: iPlain, text: "add $d0, $u0, #1", defs: []vreg{j}, uses: []vreg{j}}, jumpTo(ih)}
		l.insts = append(latch, jumpTo(h))
		x.insts = []*inst{ret()}
		// the latch laid out last, away from b
		return newFstate(5, h, b, ph, ih, ib, x, l), l
	}
	plain := []*inst{{kind: iPlain, text: "add $d0, $u0, #1", defs: []vreg{i}, uses: []vreg{i}}}
	s, l := build(plain)
	s.placeLatches()
	if s.hot[2] != l {
		t.Fatalf("the latch was not moved after the test (layout %v)", ids(s.hot))
	}
	if !s.dupOK[l] {
		t.Errorf("a plain latch is not marked for copies")
	}
	faulting := []*inst{{kind: iPlain, text: "adds $d0, $u0, #1", defs: []vreg{i}, uses: []vreg{i}, fault: &fault{cond: "vs", fn: "xo_fault_overflow"}}}
	s, l = build(faulting)
	s.placeLatches()
	if s.dupOK[l] {
		t.Errorf("a latch with a fault exit is marked for copies")
	}
}

func ids(bs []*mblock) []int {
	var out []int
	for _, b := range bs {
		out = append(out, b.id)
	}
	return out
}
