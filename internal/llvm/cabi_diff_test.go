package llvm

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// cabiDiffProg is the program of TestCABIDiff: each input line exercises
// a piece of the C runtime that calls Xo through the C ABI exports of
// std/native/llrt/cabi.xo (decision 0207), and prints what the Go
// backend prints:
//
//	i a b k     the fault texts of Int arithmetic, shifts, division,
//	            negation, abs, and List and Bytes indexes
//	f text      a Float (Go's shortest text): its Int conversion (or its fault)
//	            and its Float32 text
//	s hex       bytes: rune counts, decoding, and when they are UTF-8
//	            the Str's rune count, split into runes, trim, and the
//	            white space of each rune
//	n lists addr  whether Net.only of the lists (`;` between the lists,
//	            `,` between entries) denies a dial of addr
func cabiDiffProg() string {
	return `use std/task

fn show(io: Stdio, label: Str, r: Result[Str, task.Isolated[()]]) uses stdio {
  match r {
    Ok(v) => io.println("${label}: ${v}")
    Err(task.Isolated.Faulted(info)) => io.println("${label}: fault ${info.msg}")
    Err(_) => io.println("${label}: failed")
  }
}

fn ints(io: Stdio, a: Int, b: Int, k: Int) uses stdio {
  let xs = [1, 2, 3]
  let bs = "xyz".bytes()
  show(io, "add", task.isolate(fn() -> Str ! () { "${a + b}" }))
  show(io, "sub", task.isolate(fn() -> Str ! () { "${a - b}" }))
  show(io, "mul", task.isolate(fn() -> Str ! () { "${a * b}" }))
  show(io, "div", task.isolate(fn() -> Str ! () { "${a / b}" }))
  show(io, "rem", task.isolate(fn() -> Str ! () { "${a % b}" }))
  show(io, "shl", task.isolate(fn() -> Str ! () { "${a << k}" }))
  show(io, "shr", task.isolate(fn() -> Str ! () { "${a >> k}" }))
  show(io, "neg", task.isolate(fn() -> Str ! () { "${-a}" }))
  show(io, "abs", task.isolate(fn() -> Str ! () { "${a.abs()}" }))
  show(io, "index", task.isolate(fn() -> Str ! () { "${xs[k]}" }))
  show(io, "bytes index", task.isolate(fn() -> Str ! () { "${bs[k]}" }))
}

fn floats(io: Stdio, f: Float) uses stdio {
  show(io, "int", task.isolate(fn() -> Str ! () { "${Int(f)}" }))
  show(io, "f32", task.isolate(fn() -> Str ! () { "${Float32(f)}" }))
}

fn text(io: Stdio, b: Bytes) uses stdio {
  io.print("bytes ${b.rune_count()}:")
  var i = 0
  while i < b.len() {
    let (r, n) = b.decode_rune(i)
    io.print(" ${Int(r)}/${n}")
    i += n
  }
  io.println("")
  if let Some(s) = b.to_str() {
    io.print("str ${s.rune_count()} ${s.split("").len()} [${s.trim()}]")
    for r in s.runes() {
      io.print(if r.is_space() { " s" } else { " -" })
    }
    io.println("")
  }
}

fn net(io: Stdio, n0: Net, lists: Str, addr: Str) uses stdio, net.dial {
  var n = n0
  for l in lists.split(";") {
    n = n.only(if l.is_empty() { [] } else { l.split(",") })
  }
  match n.dial(addr) {
    Ok(c) => {
      c.close()
      io.println("net ${addr}: connected")
    }
    Err(e) => {
      // the variant, not the printed text (decision 0358 made it plain)
      let kind = match e {
        Denied(_) => "denied"
        _ => "other"
      }
      io.println("net ${addr}: ${kind}")
    }
  }
}

fn main(os: Os) {
  let io = os.stdio
  let input = match io.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  for line in input.lines() {
    let f = line.split(" ")
    io.println("# ${line}")
    match f[0] {
      "i" => ints(io, Int.parse(f[1]) ?? 0, Int.parse(f[2]) ?? 0, Int.parse(f[3]) ?? 0)
      "f" => floats(io, Float.parse(f[1]) ?? 0.0)
      "s" => text(io, Bytes.from_hex(f[1]) ?? "".bytes())
      "n" => net(io, os.net, f[1], f[2])
      _ => {}
    }
  }
}
`
}

// cabiCorpus is the input of cabiDiffProg: Int bounds and random words,
// Float bits at the edges of Int's range, NaNs, infinities, subnormals,
// and random; byte strings of random UTF-8 (white space, every length of
// sequence) and invalid sequences; Net.only lists and dial addresses of
// loopback literals (an admitted dial is refused: nothing leaves the
// machine, no name is looked up but localhost).
func cabiCorpus() string {
	rng := rand.New(rand.NewPCG(207, 4))
	var b strings.Builder
	ints := []int64{math.MinInt64, math.MinInt64 + 1, math.MaxInt64, math.MaxInt64 - 1, 0, 1, -1, 2, -2, 3, 1 << 32, -1 << 32, 3037000500}
	for _, x := range ints {
		for _, y := range ints {
			fmt.Fprintf(&b, "i %d %d %d\n", x, y, rng.Int64N(80)-5)
		}
	}
	for range 300 {
		fmt.Fprintf(&b, "i %d %d %d\n", int64(rng.Uint64())>>rng.IntN(64), int64(rng.Uint64())>>rng.IntN(64), rng.Int64N(80)-5)
	}
	floats := []float64{0, math.Copysign(0, -1), 1, -1, 9.2233720368547758e18, -9.2233720368547758e18, 9.223372036854775e18, -9.2233720368547748e18,
		math.Inf(1), math.Inf(-1), math.NaN(), 5e-324, math.MaxFloat64, -math.MaxFloat64, 1e300, 0.1, 100, 1e21, 123456789012345678, 3.4028235e38, 1.401298464324817e-45}
	for _, f := range floats {
		fmt.Fprintf(&b, "f %s\n", strconv.FormatFloat(f, 'g', -1, 64))
	}
	for range 300 {
		fmt.Fprintf(&b, "f %s\n", strconv.FormatFloat(math.Float64frombits(rng.Uint64()), 'g', -1, 64))
	}
	for range 200 {
		fmt.Fprintf(&b, "f %s\n", strconv.FormatFloat(rng.NormFloat64()*math.Pow(10, float64(rng.IntN(40)-10)), 'g', -1, 64))
	}
	runes := []rune{'a', ' ', '\t', '\n', '\v', '\f', '\r', 0x85, 0xA0, 0x1680, 0x2000, 0x2005, 0x200A, 0x200B, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF,
		'é', 'ü', '日', '語', 0x10000, 0x1F600, 0x10FFFF, 0xFFFD, 0x7F, 0x80, 0x7FF, 0x800, 0xFFFF, 0xD7FF, 0xE000}
	invalid := [][]byte{{0xFF}, {0xC0, 0x80}, {0xC1, 0xBF}, {0xE0, 0x80, 0x80}, {0xED, 0xA0, 0x80}, {0xF4, 0x90, 0x80, 0x80}, {0xF5}, {0x80}, {0xE2, 0x82}, {0xF0, 0x9F, 0x98}, {0xC3}}
	for range 400 {
		var s []byte
		for range rng.IntN(12) {
			if rng.IntN(5) == 0 {
				s = append(s, invalid[rng.IntN(len(invalid))]...)
			} else {
				s = append(s, string(runes[rng.IntN(len(runes))])...)
			}
		}
		fmt.Fprintf(&b, "s %x\n", s)
	}
	entries := []string{"127.0.0.1", "127.0.0.1:1", "127.0.0.1:2", "[::1]", "::1", "[::1]:1", "localhost", "localhost:1", "*.example", "*.0.0.1",
		"LOCALHOST", "[127.0.0.1]", "0.0.0.0", ":1", "unix:/nonexistent/xo.sock", "127.0.0.2", "[::1]:2", "a:b:c", "]x[", "*", "*."}
	addrs := []string{"127.0.0.1:1", "127.0.0.1:2", "[::1]:1", "[::1]:2", "localhost:1", "LocalHost:1", "127.0.0.1", "[::1]", "::1:1", "127.0.0.1:1:2",
		"[127.0.0.1]:1", ":1", "unix:/nonexistent/xo.sock", "unix:/nonexistent/other.sock", "[::1]x:1", "127.0.0.2:1", "[::1]]:1", "[[::1]:1", "0.0.0.0:1"}
	for range 300 {
		var lists []string
		for range 1 + rng.IntN(2) {
			var es []string
			for range rng.IntN(4) {
				es = append(es, entries[rng.IntN(len(entries))])
			}
			lists = append(lists, strings.Join(es, ","))
		}
		fmt.Fprintf(&b, "n %s %s\n", strings.Join(lists, ";"), addrs[rng.IntN(len(addrs))])
	}
	return b.String()
}

// TestCABIDiff runs cabiDiffProg over cabiCorpus on the Go backend and on
// both native backends and requires the Go backend's lines: the pieces of
// the C runtime that call Xo through C ABI exports (decision 0207), the
// fault texts byte for byte among them.
func TestCABIDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three programs")
	}
	dir := t.TempDir()
	in := cabiCorpus()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(cabiDiffProg()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	ref := buildAndRun(t, dir, xotool.Options{Backend: xotool.BackendGo})
	if ref.exit != 0 {
		t.Fatalf("go backend: exit %d: %s", ref.exit, ref.stderr)
	}
	want := strings.Split(strings.TrimSuffix(ref.stdout, "\n"), "\n")
	t.Logf("%d input lines, %d output lines", strings.Count(in, "\n"), len(want))
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		t.Run(be, func(t *testing.T) {
			r := buildAndRun(t, dir, xotool.Options{Backend: be})
			if r.exit != 0 {
				t.Fatalf("exit %d: %s", r.exit, r.stderr)
			}
			diffLines(t, strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n"), want)
		})
	}
}
