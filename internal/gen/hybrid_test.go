package gen_test

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// Hybrid builds (decision 0084): functions compiled to Go assembler by
// the arm64 backend must behave exactly as their Go versions.

func needArm64(t *testing.T) {
	t.Helper()
	if runtime.GOARCH != "arm64" {
		t.Skip("hybrid assembly is arm64 only")
	}
}

// vetHybrid runs go vet (asmdecl checks the assembly against its Go
// declarations) over the generated packages of a build directory.
func vetHybrid(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "vet", "-asmdecl", "./m/...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0", "GOCACHE="+build.GoCacheDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
}

// TestGoldenHybrid builds every golden program with an eligible function
// with `--hybrid`, every eligible function in assembly (HybridAll), and
// compares with want.txt as TestGolden does; go vet must pass.
func TestGoldenHybrid(t *testing.T) {
	needArm64(t)
	if *updateGolden {
		t.Skip("want.txt is written by TestGoldenNoMono")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, dir := range dirs {
		dir := dir
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() || strings.HasPrefix(filepath.Base(dir), "test_") {
			continue
		}
		info := build.HybridPlan([]string{dir}, true)
		if len(info.Asm) == 0 {
			continue
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			skipOffline(t, dir)
			got, res := buildAndRunOpts(t, dir, build.Options{Mode: gen.Build, Hybrid: true, HybridAll: true})
			if res.Hybrid == nil || len(res.Hybrid.Asm) == 0 {
				t.Fatalf("no assembly in the build: %+v", res.Hybrid)
			}
			got.stderr = stableStderr(got.stderr)
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if dropGoFrames(got.String()) != dropGoFrames(string(want)) {
				t.Errorf("hybrid (%s) output differs\n--- got\n%s\n--- want\n%s", strings.Join(res.Hybrid.Asm, ", "), got.String(), want)
			}
			vetHybrid(t, res.Dir)
		})
	}
	if n == 0 {
		t.Fatal("no golden program has an eligible function")
	}
}

// TestHybridRandom builds random programs of eligible kernels (Int, Float,
// Bool, and struct Lists, `var` writes, recursion, and faults: index,
// overflow, division by zero, Float to Int) with random data, once with
// the kernels in assembly and once with tag xo_pure, and compares the
// runs. The full suite runs seeds 1 to 4 (each kind of kernel mix, with
// and without a fault), -short 1 to 3; XO_HYBRID_SEEDS=N runs 1 to N
// (bench/nightly.sh: 24, decision 0111).
func TestHybridRandom(t *testing.T) {
	t.Parallel() // temporary directories only
	needArm64(t)
	seeds := 4
	if testing.Short() {
		seeds = 3
	}
	if v := os.Getenv("XO_HYBRID_SEEDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("XO_HYBRID_SEEDS=%q: want a positive count", v)
		}
		seeds = n
	}
	for seed := 1; seed <= seeds; seed++ {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := randomKernels(rand.New(rand.NewSource(int64(seed))), seed%4, seed/4%2 == 0)
			os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/hybrid\n"), 0o644)
			os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644)
			hy, res := buildAndRunOpts(t, dir, build.Options{Mode: gen.Build, Hybrid: true, HybridAll: true, Out: filepath.Join(dir, "hybrid")})
			if res.Hybrid == nil || len(res.Hybrid.Asm) < 10 {
				t.Fatalf("expected the kernels in assembly: %+v", res.Hybrid)
			}
			pure, _ := buildAndRunOpts(t, dir, build.Options{Mode: gen.Build, Hybrid: true, HybridAll: true, HybridPure: true, Out: filepath.Join(dir, "pure")})
			hy.stderr, pure.stderr = stableStderr(hy.stderr), stableStderr(pure.stderr)
			if dropGoFrames(hy.String()) != dropGoFrames(pure.String()) {
				t.Errorf("hybrid and pure differ\n--- hybrid\n%s\n--- pure\n%s\n--- program\n%s", hy, pure, src)
			}
			t.Logf("exit %d, stderr %q, assembly: %s", hy.exit, hy.stderr, strings.Join(res.Hybrid.Asm, " "))
			vetHybrid(t, res.Dir)
		})
	}
}

// randomKernels writes a program that runs eligible kernels over random
// data; its last call may fault. No Float product feeds an addition: Go
// fuses those into one rounding (gotchas/go-backend-fma-contraction.md),
// the assembly does not.
func randomKernels(r *rand.Rand, kind int, fault bool) string {
	ints := func(n, lim int) string {
		var xs []string
		for i := 0; i < n; i++ {
			xs = append(xs, fmt.Sprint(r.Intn(2*lim+1)-lim))
		}
		return "[" + strings.Join(xs, ", ") + "]"
	}
	floats := func(n int) string {
		var xs []string
		for i := 0; i < n; i++ {
			xs = append(xs, fmt.Sprintf("%.3f", r.Float64()*200-100))
		}
		return "[" + strings.Join(xs, ", ") + "]"
	}
	bools := func(n int) string {
		var xs []string
		for i := 0; i < n; i++ {
			xs = append(xs, fmt.Sprint(r.Intn(2) == 0))
		}
		return "[" + strings.Join(xs, ", ") + "]"
	}
	n := 1 + r.Intn(40)
	var p []string
	for i := 0; i < n; i++ {
		p = append(p, fmt.Sprintf("P{a: %d, b: %.2f}", r.Intn(2001)-1000, r.Float64()*10))
	}
	k1, k2, k3 := 1+r.Intn(9), 1+r.Intn(9), 2+r.Intn(9)
	var b strings.Builder
	fmt.Fprintf(&b, `struct P {
  a: Int
  b: Float
}

fn ksum(xs: List[Int]) -> Int {
  var s = 0
  for i in 0..xs.len() {
    s = s + xs[i] * %d - xs[i] / %d + xs[i] %% %d
  }
  s
}

fn kmax(xs: List[Int]) -> Int {
  var m = -9223372036854775807 - 1
  for x in xs {
    m = m.max(x)
  }
  m
}

fn kprefix(var xs: List[Int]) {
  for i in 1..xs.len() {
    xs[i] = xs[i] + xs[i - 1]
  }
}

fn kreverse(var xs: List[Int]) {
  var i = 0
  var j = xs.len() - 1
  while i < j {
    let t = xs[i]
    xs[i] = xs[j]
    xs[j] = t
    i += 1
    j -= 1
  }
}

fn kbools(var bs: List[Bool], k: Int) -> Int {
  var c = 0
  for i in 0..bs.len() {
    if bs[i] {
      c += 1
    }
    bs[i] = (i %% k == 0) != bs[i]
  }
  c
}

fn kfloat(fs: List[Float]) -> Float {
  var s = 0.0
  for f in fs {
    s = s + f.abs().sqrt() - f.floor() / 3.0 + f.min(1.5) / 7.0
  }
  s
}

fn kstruct(var ps: List[P], d: Int) -> Int {
  var t = 0
  for i in 0..ps.len() {
    ps[i].a = ps[i].a + d
    ps[i].b = ps[i].b * 2.0
    t += ps[i].a
  }
  t
}

fn kshift(xs: List[Int], k: Int) -> Int {
  var s = 0
  for x in xs {
    s = s ^ ((x & 255) << (k %% 8)) ^ (x >> 3)
  }
  s
}

fn ksort(var xs: List[Int], lo: Int, hi: Int) {
  if lo >= hi {
    return
  }
  let pivot = xs[lo + (hi - lo) / 2]
  var i = lo
  var j = hi
  while i <= j {
    while xs[i] < pivot {
      i += 1
    }
    while pivot < xs[j] {
      j -= 1
    }
    if i <= j {
      let t = xs[i]
      xs[i] = xs[j]
      xs[j] = t
      i += 1
      j -= 1
    }
  }
  ksort(xs, lo, j)
  ksort(xs, i, hi)
}

fn kfib(n: Int) -> Int {
  if n < 2 {
    n
  } else {
    kfib(n - 1) + kfib(n - 2)
  }
}

fn kcount(xs: List[Int], v: Int) -> Int {
  var c = 0
  for x in xs {
    if x > v {
      c += 1
    }
  }
  c
}

fn kdot(ps: List[P]) -> Float {
  var s = 0.0
  for p in ps {
    s = s + Float(p.a) / p.b
  }
  s
}

fn kfault_index(xs: List[Int], k: Int) -> Int {
  var s = 0
  for i in 0..k {
    s += xs[i]
  }
  s
}

fn kfault_div(xs: List[Int], d: Int) -> Int {
  var s = 0
  for x in xs {
    s += x / d
  }
  s
}

fn kfault_mul(a: Int, n: Int) -> Int {
  var x = a
  for _ in 0..n {
    x = x * 3
  }
  x
}

fn kfault_ftoi(fs: List[Float], m: Float) -> Int {
  var s = 0
  for f in fs {
    s += Int(f * m)
  }
  s
}

fn show(xs: List[Int]) -> Str {
  var s = ""
  for x in xs {
    s = s + " ${x}"
  }
  s
}

fn showb(xs: List[Bool]) -> Str {
  var s = ""
  for x in xs {
    s = s + " ${x}"
  }
  s
}

fn main(os: Os) {
  var xs = %s
  var ys = %s
  var bs = %s
  let fs = %s
  var ps: List[P] = [%s]
`, k1, k2, k3, ints(n, 1000000), ints(1+r.Intn(30), 50), bools(1+r.Intn(30)), floats(1+r.Intn(30)), strings.Join(p, ", "))
	b.WriteString(`  os.stdio.println("sum ${ksum(xs)} max ${kmax(xs)} count ${kcount(xs, 0)} shift ${kshift(xs, 5)}")
  kprefix(ys)
  os.stdio.println("prefix ${show(ys)}")
  kreverse(xs)
  os.stdio.println("reverse ${show(xs)}")
`)
	fmt.Fprintf(&b, "  os.stdio.println(\"bools ${kbools(bs, %d)} ${showb(bs)}\")\n", 1+r.Intn(5))
	b.WriteString("  os.stdio.println(\"float ${Int(kfloat(fs) * 1000.0)} dot ${Int(kdot(ps) * 1000.0)}\")\n")
	fmt.Fprintf(&b, "  os.stdio.println(\"struct ${kstruct(ps, %d)} ${ps[0].a} ${Int(ps[0].b * 100.0)}\")\n", r.Intn(200)-100)
	b.WriteString("  ksort(xs, 0, xs.len() - 1)\n  os.stdio.println(\"sorted ${show(xs)}\")\n")
	fmt.Fprintf(&b, "  os.stdio.println(\"fib ${kfib(%d)}\")\n", r.Intn(20))
	pick := func(safe, bad string) string {
		if fault {
			return bad
		}
		return safe
	}
	switch kind {
	case 0:
		fmt.Fprintf(&b, "  os.stdio.println(\"index ${kfault_index(xs, %s)}\")\n", pick(fmt.Sprint(n), fmt.Sprint(n+1+r.Intn(3))))
	case 1:
		fmt.Fprintf(&b, "  os.stdio.println(\"div ${kfault_div(xs, %s)}\")\n", pick(fmt.Sprint(1+r.Intn(5)), "0"))
	case 2:
		fmt.Fprintf(&b, "  os.stdio.println(\"mul ${kfault_mul(%d, %s)}\")\n", 1+r.Intn(1000), pick(fmt.Sprint(20+r.Intn(5)), fmt.Sprint(40+r.Intn(5))))
	case 3:
		fmt.Fprintf(&b, "  os.stdio.println(\"ftoi ${kfault_ftoi(fs, %s)}\")\n", pick("1000.0", []string{"1.0e30", "0.0 / 0.0"}[r.Intn(2)]))
	}
	b.WriteString("}\n")
	return b.String()
}
