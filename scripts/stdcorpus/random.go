package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
)

// std/random (decision 0606): Go's NormFloat64, ExpFloat64 and Perm on a
// seeded PCG stream. The stream's Uint64s are recorded; the Xo test
// replays them through a Rand that maps each draw as Go splits a Uint64
// (r.int(-2^31, 2^31) is int32(u) and r.int(0, 128) u>>32&127 of the same
// u; r.float() is Go's Float64; r.int(0, n) is Go's uint64n), so the
// values must be Go's bit for bit.

type recSrc struct {
	s   *rand.PCG
	out []uint64
}

func (r *recSrc) Uint64() uint64 { v := r.s.Uint64(); r.out = append(r.out, v); return v }

func i64s(xs []uint64) string {
	var b strings.Builder
	for k, x := range xs {
		if k%8 == 0 {
			b.WriteString("\n  ")
		}
		fmt.Fprintf(&b, "%d, ", int64(x))
	}
	return b.String()
}

// xfloat is f as an Xo Float literal.
func xfloat(f float64) string {
	t := fmt.Sprint(f)
	if !strings.ContainsAny(t, ".e") {
		t += ".0"
	}
	return t
}

// zipfStrict is Go's Zipf (math/rand/v2 zipf.go, go1.26, Copyright 2009
// The Go Authors, BSD-style license) with every product rounded on its own
// (float64() of each product: the Go spec lets arm64, ppc64, riscv64 and
// s390x fuse x*y+z into one rounding, and Go's own Zipf on this Mac fuses
// ur := hxm + r*hx0minusHxm). Xo's floats are strict (decision 0287; Go on
// amd64 does not fuse either), so the corpus's answer is the unfused one,
// the same on every platform (task #116: random fuzz seed 3, draw 35 of
// s=1.08 v=11.39 off by one on arm64 Go).
type zipfStrict struct {
	r                                                        *rand.Rand
	imax, v, q, s, oneminusQ, oneminusQinv, hxm, hx0minusHxm float64
}

// strictExp and strictLog are Go's pure math.Exp and math.Log (exp.go,
// log.go) with every product rounded on its own, as std/rt/gomath_gen.go
// has them (decision 0287): Go on arm64 runs math.Exp in assembly and
// fuses inside math.Log, so a zipfStrict that guarded only its own
// products still drew one more than Xo (#116, random fuzz seed 17: draw
// 139 of s=1.0032 v=3.21, 147611500472720 from Go on arm64,
// 147611500472719 from Go with fusion off and from Xo).
func strictExp(x float64) float64 {
	const (
		Ln2Hi     = 6.93147180369123816490e-01
		Ln2Lo     = 1.90821492927058770002e-10
		Log2e     = 1.44269504088896338700e+00
		Overflow  = 7.09782712893383973096e+02
		Underflow = -7.45133219101941108420e+02
		NearZero  = 1.0 / (1 << 28)
	)
	switch {
	case math.IsNaN(x):
		return x
	case x > Overflow:
		return math.Inf(1)
	case x < Underflow:
		return 0
	case -NearZero < x && x < NearZero:
		return 1 + x
	}
	var k int
	switch {
	case x < 0:
		k = int(float64(Log2e*x) - 0.5)
	case x > 0:
		k = int(float64(Log2e*x) + 0.5)
	}
	hi := x - float64(float64(k)*Ln2Hi)
	lo := float64(float64(k) * Ln2Lo)
	const (
		P1 = 1.66666666666666657415e-01
		P2 = -2.77777777770155933842e-03
		P3 = 6.61375632143793436117e-05
		P4 = -1.65339022054652515390e-06
		P5 = 4.13813679705723846039e-08
	)
	r := hi - lo
	t := float64(r * r)
	c := r - float64(t*(P1+float64(t*(P2+float64(t*(P3+float64(t*(P4+float64(t*P5)))))))))
	y := 1 - ((lo - (float64(r*c))/(2-c)) - hi)
	return math.Ldexp(y, k)
}

func strictLog(x float64) float64 {
	const (
		Ln2Hi = 6.93147180369123816490e-01
		Ln2Lo = 1.90821492927058770002e-10
		L1    = 6.666666666666735130e-01
		L2    = 3.999999999940941908e-01
		L3    = 2.857142874366239149e-01
		L4    = 2.222219843214978396e-01
		L5    = 1.818357216161805012e-01
		L6    = 1.531383769920937332e-01
		L7    = 1.479819860511658591e-01
	)
	switch {
	case math.IsNaN(x) || math.IsInf(x, 1):
		return x
	case x < 0:
		return math.NaN()
	case x == 0:
		return math.Inf(-1)
	}
	f1, ki := math.Frexp(x)
	if f1 < math.Sqrt2/2 {
		f1 *= 2
		ki--
	}
	f := f1 - 1
	k := float64(ki)
	s := f / (2 + f)
	s2 := float64(s * s)
	s4 := float64(s2 * s2)
	t1 := float64(s2 * (L1 + float64(s4*(L3+float64(s4*(L5+float64(s4*L7)))))))
	t2 := float64(s4 * (L2 + float64(s4*(L4+float64(s4*L6)))))
	R := t1 + t2
	hfsq := float64(float64(0.5*f) * f)
	return float64(k*Ln2Hi) - ((hfsq - (float64(s*(hfsq+R)) + float64(k*Ln2Lo))) - f)
}

func (z *zipfStrict) h(x float64) float64 {
	return float64(strictExp(float64(z.oneminusQ*strictLog(z.v+x))) * z.oneminusQinv)
}

func (z *zipfStrict) hinv(x float64) float64 {
	return strictExp(float64(z.oneminusQinv*strictLog(float64(z.oneminusQ*x)))) - z.v
}

func newZipfStrict(r *rand.Rand, s float64, v float64, imax uint64) *zipfStrict {
	if s <= 1.0 || v < 1 {
		return nil
	}
	z := &zipfStrict{r: r, imax: float64(imax), v: v, q: s}
	z.oneminusQ = 1.0 - z.q
	z.oneminusQinv = 1.0 / z.oneminusQ
	z.hxm = z.h(z.imax + 0.5)
	z.hx0minusHxm = z.h(0.5) - strictExp(float64(strictLog(z.v)*(-z.q))) - z.hxm
	z.s = 1 - z.hinv(z.h(1.5)-strictExp(float64(-z.q*strictLog(z.v+1.0))))
	return z
}

func (z *zipfStrict) Uint64() uint64 {
	k := 0.0
	for {
		r := z.r.Float64()
		ur := z.hxm + float64(r*z.hx0minusHxm)
		x := z.hinv(ur)
		k = math.Floor(x + 0.5)
		if k-x <= z.s {
			break
		}
		if ur >= z.h(k+0.5)-strictExp(float64(-strictLog(k+z.v)*z.q)) {
			break
		}
	}
	return uint64(k)
}

// randomFuzzSeed is XO_RANDOMFUZZ_SEED (default 0, the committed corpus),
// added to every PCG seed: internal/stdtest/corpusfuzz_test.go's fresh
// seeds (task #116), which also draw 40 more Zipf parameter sets.
func randomFuzzSeed() uint64 {
	seed, _ := strconv.ParseUint(os.Getenv("XO_RANDOMFUZZ_SEED"), 10, 64)
	return seed
}

type zipfParams = []struct {
	s, v float64
	imax uint64
}

func randomZipfParams() zipfParams {
	ps := zipfParams{{1.1, 1, 10}, {2, 1, 100}, {1.5, 3, 1000}, {3, 1, 5}, {1.01, 1, 1 << 40}, {2.5, 10.5, 0}, {1.2, 2, 1}, {1.0001, 1, 1000000}, {7, 1, 1 << 62}, {1, 1, 10}, {0.5, 1, 10}, {2, 0.5, 10}}
	seed := randomFuzzSeed()
	if seed == 0 {
		return ps
	}
	r := rand.New(rand.NewPCG(seed, 99))
	for k := 0; k < 40; k++ {
		s := 1 + r.Float64()*4
		v := 1 + r.Float64()*20
		if r.IntN(10) == 0 {
			s = r.Float64() * 1.2 // NewZipf refuses s <= 1
		}
		if r.IntN(10) == 0 {
			v = r.Float64() * 1.2 // and v < 1
		}
		// Xo's zipf takes imax as an Int: a draw past Int's range (a shift
		// of 0 with the top bit set; seed 4, #116) is halved, with the same
		// draws taken, so seed 1's corpus is unchanged
		imax := r.Uint64() >> r.IntN(64)
		if imax > math.MaxInt64 {
			imax >>= 1
		}
		ps = append(ps, struct {
			s, v float64
			imax uint64
		}{s, v, imax})
	}
	return ps
}

func randomCorpus() string {
	var o strings.Builder
	o.WriteString(header("math/rand/v2's NormFloat64, ExpFloat64, Perm and Zipf on recorded PCG streams, replayed draw for draw"))
	o.WriteString("use std/random\n\n")
	for _, c := range []struct {
		name string
		n    int
		f    func(*rand.Rand) float64
	}{{"NORM", 1500, (*rand.Rand).NormFloat64}, {"EXP", 1500, (*rand.Rand).ExpFloat64}} {
		src := &recSrc{s: rand.NewPCG(606+randomFuzzSeed(), uint64(len(c.name)))}
		r := rand.New(src)
		var want []uint64
		for k := 0; k < c.n; k++ {
			want = append(want, math.Float64bits(c.f(r)))
		}
		fmt.Fprintf(&o, "/// The Uint64s Go's %s drew, in order.\nconst %s_U: List[Int] = [%s\n]\n\n", c.name, c.name, i64s(src.out))
		fmt.Fprintf(&o, "/// Go's %d values (Float64bits).\nconst %s_WANT: List[Int] = [%s\n]\n\n", c.n, c.name, i64s(want))
	}
	src := &recSrc{s: rand.NewPCG(606+randomFuzzSeed(), 3)}
	r := rand.New(src)
	o.WriteString("/// Go's Perm(n) for each n, in order.\nconst PERM_WANT: List[List[Int]] = [\n")
	sizes := []int{0, 1, 2, 3, 5, 8, 13, 50, 100, 1000}
	for k := 0; k < 30; k++ {
		sizes = append(sizes, 2+k*7)
	}
	for _, n := range sizes {
		p := r.Perm(n)
		var s []string
		for _, x := range p {
			s = append(s, fmt.Sprint(x))
		}
		fmt.Fprintf(&o, "  [%s],\n", strings.Join(s, ", "))
	}
	o.WriteString("]\n\n")
	var ns []string
	for _, n := range sizes {
		ns = append(ns, fmt.Sprint(n))
	}
	fmt.Fprintf(&o, "const PERM_N: List[Int] = [%s]\n\n/// The Uint64s Go's Perm calls drew, in order.\nconst PERM_U: List[Int] = [%s\n]\n", strings.Join(ns, ", "), i64s(src.out))
	// Zipf: NewZipf's parameters (nil ones included; Go's Zipf with products rounded, zipfStrict) and 200 draws of each
	zsrc := &recSrc{s: rand.NewPCG(606+randomFuzzSeed(), 4)}
	zr := rand.New(zsrc)
	o.WriteString("\n/// (s, v, imax, whether Go's NewZipf gives one, its first 200 Uint64s)\nconst ZIPF_WANT: List[(Float, Float, Int, Bool, List[Int])] = [\n")
	for _, p := range []struct {
		s, v float64
		imax uint64
	}(randomZipfParams()) {
		z := newZipfStrict(zr, p.s, p.v, p.imax)
		var ks []string
		if z != nil {
			for k := 0; k < 200; k++ {
				ks = append(ks, fmt.Sprint(z.Uint64()))
			}
		}
		fmt.Fprintf(&o, "  (%s, %s, %d, %v, [%s]),\n", xfloat(p.s), xfloat(p.v), p.imax, z != nil, strings.Join(ks, ", "))
	}
	fmt.Fprintf(&o, "]\n\n/// The Uint64s Go's Zipf draws took, in order.\nconst ZIPF_U: List[Int] = [%s\n]\n", i64s(zsrc.out))
	o.WriteString(randomTests)
	return o.String()
}

const randomTests = `
/// A Rand replaying Go's Uint64 stream: each draw maps as Go's caller
/// splits a Uint64 (see the generator).
struct Replay {
  us: List[Int]
  at: Mutex[Int]
  /// true for normal and exp (a try's int(Int.min, Int.max) is Go's
  /// Uint64, which the try splits); false for perm (every int is Go's
  /// uint64n)
  zig: Bool
}

fn replay(us: List[Int], zig: Bool) -> Replay { Replay{us: us, at: Mutex[Int].new(0), zig: zig} }

fn (s: Replay) next() -> Int {
  let i = s.at.with(fn(var x: Int) -> Int {
    let k = x
    x = x + 1
    k
  })
  if i >= s.us.len() {
    fault("replay: the stream ran out")
  }
  s.us[i]
}

fn (s: Replay) int(lo: Int, hi: Int) -> Int uses rand {
  if s.zig && lo == Int.min && hi == Int.max {
    // r.Uint64(); Int.max, which the range leaves out, is not in the stream
    return s.next()
  }
  // Go's uint64n(hi) for Shuffle
  let n = hi
  if n & (n - 1) == 0 {
    return s.next() & (n - 1)
  }
  var u = s.next()
  var h = u.mul_hi_unsigned(n)
  var l = u.wrapping_mul(n)
  if l.lt_unsigned(n) {
    // (2^64 - n) mod n, for n below 2^62
    let thresh = ((Int.max % n + 1) % n * 2) % n
    while l.lt_unsigned(thresh) {
      u = s.next()
      h = u.mul_hi_unsigned(n)
      l = u.wrapping_mul(n)
    }
  }
  h
}

fn (s: Replay) float() -> Float uses rand {
  // Go's Float64: the low 53 bits over 2^53
  Float((s.next().wrapping_shl(11)).shr_logical(11)) / 9007199254740992.0
}

fn (s: Replay) bool() -> Bool uses rand { fault("replay: bool") }
fn (s: Replay) shuffle[T](xs: List[T]) -> List[T] uses rand { fault("replay: shuffle") }
fn (s: Replay) bytes(n: Int) -> Bytes uses rand { fault("replay: bytes") }

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "normal is Go's NormFloat64 draw for draw" {
  let r = replay(NORM_U, true)
  var bad: List[(Int, Int, Int)] = []
  for (k, want) in NORM_WANT.indexed() {
    let got = Int(random.normal(r).to_bits())
    if got != want {
      bad.push((k, got, want))
    }
  }
  expect(first_bad(bad)) == []
  expect(r.at.with(fn(var x: Int) -> Int { x })) == NORM_U.len()
}

test "exp is Go's ExpFloat64 draw for draw" {
  let r = replay(EXP_U, true)
  var bad: List[(Int, Int, Int)] = []
  for (k, want) in EXP_WANT.indexed() {
    let got = Int(random.exp(r).to_bits())
    if got != want {
      bad.push((k, got, want))
    }
  }
  expect(first_bad(bad)) == []
  expect(r.at.with(fn(var x: Int) -> Int { x })) == EXP_U.len()
}

test "perm is Go's Perm draw for draw" {
  let r = replay(PERM_U, false)
  var bad: List[Int] = []
  for (k, n) in PERM_N.indexed() {
    if random.perm(r, n) != PERM_WANT[k] {
      bad.push(n)
    }
  }
  expect(bad) == []
  expect(r.at.with(fn(var x: Int) -> Int { x })) == PERM_U.len()
}

test "zipf is Go's NewZipf and Zipf.Uint64 draw for draw" {
  let r = replay(ZIPF_U, false)
  var bad: List[(Float, Float, Int)] = []
  for (s, v, imax, ok, want) in ZIPF_WANT {
    match random.zipf(s, v, imax) {
      Some(z) => {
        var got: List[Int] = []
        for _ in 0..want.len() {
          got.push(z.draw(r))
        }
        if !ok || got != want {
          bad.push((s, v, imax))
        }
      }
      None => if ok {
        bad.push((s, v, imax))
      }
    }
  }
  expect(bad) == []
  expect(r.at.with(fn(var x: Int) -> Int { x })) == ZIPF_U.len()
}

test "the moments over a million draws of a seeded Rand" {
  let r = os.rand
  var s1 = 0.0
  var s2 = 0.0
  var e1 = 0.0
  for _ in 0..1000000 {
    let x = random.normal(r)
    s1 += x
    s2 += x * x
    e1 += random.exp(r)
  }
  let m = s1 / 1000000.0
  let v = s2 / 1000000.0 - m * m
  expect(m.abs() < 0.005 && (v - 1.0).abs() < 0.01 && (e1 / 1000000.0 - 1.0).abs() < 0.005) == true
}
`
