package llvm

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// intsTypes are the sized integer types TestIntsDiff covers: name, bits,
// unsigned.
var intsTypes = []struct {
	name string
	bits int
	uns  bool
}{
	{"Int8", 8, false}, {"Int16", 16, false}, {"Int32", 32, false},
	{"UInt8", 8, true}, {"UInt16", 16, true}, {"UInt32", 32, true}, {"UInt64", 64, true},
}

// intsDiffProg is the program of TestIntsDiff: for each input line
// `<type index> <a> <b> <k>` it prints every operation of the sized type
// on a and b (and the Int k as a shift amount), every conversion of a to
// the other integer types and Rune, and Int32.try and friends of the Int
// a, each result or fault message (task.isolate). A UInt64 operand is
// given as its Int word.
func intsDiffProg() string {
	var b strings.Builder
	b.WriteString(`use std/task

fn show(io: Stdio, label: Str, r: Result[Str, task.Isolated[()]]) uses stdio {
  match r {
    Ok(v) => io.println("${label}: ${v}")
    Err(task.Isolated.Faulted(info)) => io.println("${label}: fault ${info.msg} at ${info.location}")
    Err(_) => io.println("${label}: failed")
  }
}

/// The UInt64 of the Int word w.
fn u64(w: Int) -> UInt64 { (UInt64((w >> 32) & 0xFFFFFFFF) << 32) | UInt64(w & 0xFFFFFFFF) }

fn int_ops(io: Stdio, x: Int, y: Int) uses stdio {
  show(io, "Int checked", task.isolate(fn() -> Str ! () { "${x.checked_add(y) ?? 7} ${x.checked_sub(y) ?? 7} ${x.checked_mul(y) ?? 7} ${x.checked_div(y) ?? 7} ${x.checked_add(y).is_some()} ${x.checked_sub(y).is_some()} ${x.checked_mul(y).is_some()} ${x.checked_div(y).is_some()}" }))
  show(io, "Int saturating", task.isolate(fn() -> Str ! () { "${x.saturating_add(y)} ${x.saturating_sub(y)} ${x.saturating_mul(y)}" }))
  show(io, "Int wrapping", task.isolate(fn() -> Str ! () { "${x.wrapping_add(y)} ${x.wrapping_sub(y)} ${x.wrapping_mul(y)}" }))
  show(io, "Int pow", task.isolate(fn() -> Str ! () { "${x.pow(y % 70)}" }))
  show(io, "Int Float32", task.isolate(fn() -> Str ! () { "${Float32(x)} ${Float32(Float(x) * 1.0000001)} ${Float32(x) + Float32(y)} ${Float32(x) / Float32(y)}" }))
}

fn conv(io: Stdio, n: Str, x: Int) uses stdio {
  show(io, n + " try", task.isolate(fn() -> Str ! () { "${Int8.try(x) ?? 0} ${Int16.try(x) ?? 0} ${Int32.try(x) ?? 0} ${UInt8.try(x) ?? 0} ${UInt16.try(x) ?? 0} ${UInt32.try(x) ?? 0} ${UInt64.try(x) ?? 0} ${Int(Rune.try(x) ?? 'a')}" }))
  show(io, n + " try?", task.isolate(fn() -> Str ! () { "${Int8.try(x).is_some()} ${Int16.try(x).is_some()} ${Int32.try(x).is_some()} ${UInt8.try(x).is_some()} ${UInt16.try(x).is_some()} ${UInt32.try(x).is_some()} ${UInt64.try(x).is_some()} ${Rune.try(x).is_some()}" }))
}
`)
	for _, t := range intsTypes {
		n := t.name
		fmt.Fprintf(&b, "\nfn run_%s(io: Stdio, x: %s, y: %s, k: Int) uses stdio {\n", strings.ToLower(n), n, n)
		for _, op := range []string{"+", "-", "*", "/", "%", "<<", ">>", "&", "|", "^"} {
			line := fmt.Sprintf("show(io, \"%s %s\", task.isolate(fn() -> Str ! () { \"${x %s y}\" }))", n, op, op)
			if t.bits == 64 && (op == "<<" || op == ">>") {
				// a UInt64 shift amount of 2^63 or more: the Go backend
				// shifts it (std/rt Shl and Shr test int64(n)), the native
				// backends fault (gotchas/uint64-shift-amount-above-2-63.md)
				line = "if y <= 9223372036854775807 {\n    " + line + "\n  }"
			}
			b.WriteString("  " + line + "\n")
		}
		fmt.Fprintf(&b, "  show(io, \"%s neg\", task.isolate(fn() -> Str ! () { \"${-x}\" }))\n", n)
		fmt.Fprintf(&b, "  show(io, \"%s not\", task.isolate(fn() -> Str ! () { \"${~x}\" }))\n", n)
		fmt.Fprintf(&b, "  show(io, \"%s <<k\", task.isolate(fn() -> Str ! () { \"${x << k}\" }))\n", n)
		fmt.Fprintf(&b, "  show(io, \"%s >>k\", task.isolate(fn() -> Str ! () { \"${x >> k}\" }))\n", n)
		fmt.Fprintf(&b, "  show(io, \"%s +=\", task.isolate(fn() -> Str ! () {\n    var z = x\n    z += y\n    z *= y\n    \"${z}\"\n  }))\n", n)
		// constant operands: the division by a constant and the constant
		// shifts skip checks
		consts := []string{"x / 3", "x % 7", "x * 2", "x << 3", "x >> 2", "x / 1", "x + 1", "x - 1", fmt.Sprintf("x << %d", t.bits-1), fmt.Sprintf("x >> %d", t.bits-1)}
		if !t.uns {
			consts = append(consts, "x / -1", "x % -1", "x * -1")
		}
		for _, c := range consts {
			fmt.Fprintf(&b, "  show(io, \"%s %s\", task.isolate(fn() -> Str ! () { \"${%s}\" }))\n", n, c, c)
		}
		for _, to := range intsTypes {
			if to.name != n {
				fmt.Fprintf(&b, "  show(io, \"%s to %s\", task.isolate(fn() -> Str ! () { \"${%s(x)}\" }))\n", n, to.name, to.name)
			}
		}
		fmt.Fprintf(&b, "  show(io, \"%s to Int\", task.isolate(fn() -> Str ! () { \"${Int(x)}\" }))\n", n)
		fmt.Fprintf(&b, "  show(io, \"%s to Rune\", task.isolate(fn() -> Str ! () { \"${Int(Rune(x))}\" }))\n", n)
		fmt.Fprintf(&b, "  show(io, \"%s to Float\", task.isolate(fn() -> Str ! () { \"${Float(x)}\" }))\n", n)
		// rt.u64_to_float, rt.i64_to_f32, rt.f32_round (decision 0195)
		fmt.Fprintf(&b, "  show(io, \"%s to Float32\", task.isolate(fn() -> Str ! () { \"${Float32(x)} ${Float32(Float(x) / 3.0)} ${Float32(x) * Float32(y)}\" }))\n", n)
		b.WriteString("}\n")
	}
	b.WriteString(`
fn main(os: Os) {
  let io = os.stdio
  let input = match io.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  for line in input.lines() {
    let f = line.split(" ")
    if f.len() != 4 {
      continue
    }
    let t = Int.parse(f[0]) ?? 0
    let a = Int.parse(f[1]) ?? 0
    let b = Int.parse(f[2]) ?? 0
    let k = Int.parse(f[3]) ?? 0
    io.println("# ${line}")
    match t {
`)
	for i, t := range intsTypes {
		if t.name == "UInt64" {
			fmt.Fprintf(&b, "      %d => run_%s(io, u64(a), u64(b), k)\n", i, strings.ToLower(t.name))
		} else {
			fmt.Fprintf(&b, "      %d => run_%s(io, %s(a), %s(b), k)\n", i, strings.ToLower(t.name), t.name, t.name)
		}
	}
	b.WriteString(`      8 => int_ops(io, a, b)
      _ => conv(io, "Int", a)
    }
  }
}
`)
	return b.String()
}

// intsCorpus is the input of intsDiffProg: for each type, every pair of
// its bound values (the minimum and maximum and their neighbors, 0, 1,
// -1, 2, the shift widths, half the range), with k over the shift widths,
// and random pairs; then conversions (type 7) of the bounds of every
// type, of the Unicode surrogates and limits, and random Ints.
func intsCorpus() string {
	rng := rand.New(rand.NewPCG(7, 8))
	var b strings.Builder
	for i, t := range intsTypes {
		var lo, hi int64
		if t.uns {
			lo, hi = 0, int64(1)<<t.bits-1
			if t.bits == 64 {
				hi = -1 // the word of 2^64 - 1
			}
		} else {
			lo, hi = -(int64(1) << (t.bits - 1)), int64(1)<<(t.bits-1)-1
		}
		vals := []int64{lo, lo + 1, hi, hi - 1, 0, 1, 2, 3, 7, int64(t.bits - 1), int64(t.bits), int64(t.bits + 1)}
		if t.bits == 64 {
			vals = append(vals, int64(-1)<<63, 1<<63-1, 1<<32, 1<<32-1, -2, int64(-1)<<62)
		} else {
			vals = append(vals, hi/2, hi/2+1, lo/2)
		}
		if !t.uns {
			vals = append(vals, -1, -2, -3)
		}
		fits := func(v int64) bool { return t.bits == 64 || (v >= lo && v <= hi) }
		ks := []int64{0, 1, int64(t.bits - 1), int64(t.bits), -1, 70}
		n := 0
		for _, x := range vals {
			for _, y := range vals {
				if !fits(x) || !fits(y) {
					continue
				}
				fmt.Fprintf(&b, "%d %d %d %d\n", i, x, y, ks[n%len(ks)])
				n++
			}
		}
		for range 200 {
			var x, y int64
			if t.bits == 64 {
				x, y = int64(rng.Uint64()), int64(rng.Uint64()>>rng.IntN(64))
			} else {
				span := hi - lo + 1
				x, y = lo+rng.Int64N(span), lo+rng.Int64N(span)
				if rng.IntN(3) == 0 {
					y = lo + rng.Int64N(min(span, 40))
				}
			}
			fmt.Fprintf(&b, "%d %d %d %d\n", i, x, y, rng.Int64N(int64(t.bits+2))-1)
		}
	}
	conv := []int64{0, 1, -1, 0x7F, 0x80, -0x80, -0x81, 0xFF, 0x100, 0x7FFF, 0x8000, -0x8000, -0x8001, 0xFFFF, 0x10000,
		0x7FFFFFFF, 0x80000000, -0x80000000, -0x80000001, 0xFFFFFFFF, 0x100000000, 0xD7FF, 0xD800, 0xDBFF, 0xDC00, 0xDFFF, 0xE000,
		0x10FFFF, 0x110000, 1<<63 - 1, -1 << 63}
	for range 100 {
		conv = append(conv, int64(rng.Uint64())>>rng.IntN(64))
	}
	for _, x := range conv {
		fmt.Fprintf(&b, "%d %d 0 0\n", len(intsTypes), x)
	}
	// Int's checked_, saturating_, and wrapping_ methods and pow
	// (ints.fits, decision 0195): every pair of the bounds, then random
	ib := []int64{-1 << 63, -1<<63 + 1, 1<<63 - 1, 1<<63 - 2, 0, 1, -1, 2, -2, 3, 1 << 31, -1 << 31, 1<<32 + 7, 3037000499, 3037000500, -3037000500}
	for _, x := range ib {
		for _, y := range ib {
			fmt.Fprintf(&b, "%d %d %d 0\n", len(intsTypes)+1, x, y)
		}
	}
	for range 300 {
		x, y := int64(rng.Uint64())>>rng.IntN(64), int64(rng.Uint64())>>rng.IntN(64)
		fmt.Fprintf(&b, "%d %d %d 0\n", len(intsTypes)+1, x, y)
	}
	return b.String()
}

// TestIntsDiff runs intsDiffProg over intsCorpus on the Go backend and on
// both native backends, with the sized integer operations inline and their
// faults in Xo (std/native/llrt/ints.xo, decision 0195; decision 0198
// deleted the C path), and requires the Go backend's lines: every
// operation of every sized type at its bounds, faults included.
func TestIntsDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds five programs")
	}
	dir := t.TempDir()
	in := intsCorpus()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(intsDiffProg()), 0o644); err != nil {
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
	for _, path := range []string{"xo"} {
		for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
			t.Run(path+"/"+be, func(t *testing.T) {
				r := buildAndRun(t, dir, xotool.Options{Backend: be})
				if r.exit != 0 {
					t.Fatalf("exit %d: %s", r.exit, r.stderr)
				}
				diffLines(t, strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n"), want)
			})
		}
	}
}

// diffLines reports the lines of got that differ from want (at most 20).
func diffLines(t *testing.T, got, want []string) {
	t.Helper()
	bad := 0
	for i := 0; i < len(want) || i < len(got); i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			if bad < 20 {
				t.Errorf("line %d: got %q, want %q", i+1, g, w)
			}
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d lines differ", bad, len(want))
	}
}
