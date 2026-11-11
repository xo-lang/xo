package main

import (
	"fmt"
	"math"
	"math/rand/v2"
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

func randomCorpus() string {
	var o strings.Builder
	o.WriteString(header("math/rand/v2's NormFloat64, ExpFloat64 and Perm on recorded PCG streams, replayed draw for draw"))
	o.WriteString("use std/random\n\n")
	for _, c := range []struct {
		name string
		n    int
		f    func(*rand.Rand) float64
	}{{"NORM", 1500, (*rand.Rand).NormFloat64}, {"EXP", 1500, (*rand.Rand).ExpFloat64}} {
		src := &recSrc{s: rand.NewPCG(606, uint64(len(c.name)))}
		r := rand.New(src)
		var want []uint64
		for k := 0; k < c.n; k++ {
			want = append(want, math.Float64bits(c.f(r)))
		}
		fmt.Fprintf(&o, "/// The Uint64s Go's %s drew, in order.\nconst %s_U: List[Int] = [%s\n]\n\n", c.name, c.name, i64s(src.out))
		fmt.Fprintf(&o, "/// Go's %d values (Float64bits).\nconst %s_WANT: List[Int] = [%s\n]\n\n", c.n, c.name, i64s(want))
	}
	src := &recSrc{s: rand.NewPCG(606, 3)}
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
	o.WriteString(randomTests)
	return o.String()
}

const randomTests = `
/// A Rand replaying Go's Uint64 stream: each draw maps as Go's caller
/// splits a Uint64 (see the generator).
struct Replay {
  us: List[Int]
  at: Mutex[Int]
  /// the last Uint64 a try drew, whose high half the second int reads
  last: Mutex[Int]
  /// true for normal and exp (a try's two ints share one Uint64); false
  /// for perm (every int is Go's uint64n)
  zig: Bool
}

fn replay(us: List[Int], zig: Bool) -> Replay { Replay{us: us, at: Mutex[Int].new(0), last: Mutex[Int].new(0), zig: zig} }

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

fn (s: Replay) keep(u: Int) -> Int {
  s.last.with(fn(var x: Int) -> Int {
    x = u
    u
  })
}

fn (s: Replay) int(lo: Int, hi: Int) -> Int uses rand {
  if s.zig && lo == -2147483648 && hi == 2147483648 {
    // int32(u)
    let u = s.keep(s.next())
    let w = u & 0xFFFFFFFF
    return if w >= 2147483648 { w - 4294967296 } else { w }
  }
  if s.zig && lo == 0 && hi == 4294967296 {
    return s.keep(s.next()) & 0xFFFFFFFF
  }
  if s.zig && lo == 0 && hi == 128 {
    return s.last.with(fn(var x: Int) -> Int { x }).shr_logical(32) & 127
  }
  if s.zig && lo == 0 && hi == 256 {
    return s.last.with(fn(var x: Int) -> Int { x }).shr_logical(32) & 255
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
