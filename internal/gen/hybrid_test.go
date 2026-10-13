package gen_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/gotool"
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
	cmd.Env = append(goVetEnv(), "CGO_ENABLED=0")
	defer gotool.HoldCache()()
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

// TestHybridPreemption checks that a long loop in Go assembler polls for
// preemption (decision 0131): testdata/hybrid_preempt runs `spin` (in
// assembly) in a task while main allocates and sleeps until spin is
// done. With GOMAXPROCS=4 no stop the world of the GC (gctrace) may wait
// for the loop past 10 ms and main never stalls past 100 ms; with
// GOMAXPROCS=1 main still gets the P back (sysmon preempts after 10 ms).
// Without the polls both wait for the whole loop (0.9 s for 6M rounds on
// 2026-10-10). The full suite runs about 2 s of loop, also under the
// race detector; -short about 0.3 s.
//
// A loaded host delays a stop the world by itself, polls or not
// (gotchas/hybrid-preemption-stw-under-load.md): a run that misses the
// fixed bounds is compared with the same program with spin in Go
// (xo_pure, preempted by Go's own signals) run right after it, and
// passes when it is within twice the control's figures plus a margin.
// The bounds never exceed a quarter of the loop, which a run without
// polls (its first stop the world waits for the rest of the loop)
// always misses; the no-poll control at the end checks that. The runs
// set GOGC=10 so a GC (and its stop the world) always happens while the
// loop runs, whatever the host's sleep granularity.
func TestHybridPreemption(t *testing.T) {
	// not parallel: the other tests of the package would load the host
	needArm64(t)
	rounds := "12000000"
	if testing.Short() {
		rounds = "2000000"
	}
	type run struct {
		procs string
		stw   float64 // bound on the longest stop the world (ms)
		gap   int     // bound on main's longest gap (ms)
		race  bool
	}
	runs := []run{{procs: "4", stw: 10, gap: 100}, {procs: "1", gap: 100}}
	if !testing.Short() {
		runs = append(runs, run{procs: "4", stw: 10, gap: 250, race: true})
	}
	dir := filepath.Join("testdata", "hybrid_preempt")
	type key struct{ race, pure bool }
	bins := map[key]string{}
	for _, race := range []bool{false, true} {
		if race && testing.Short() {
			continue
		}
		for _, pure := range []bool{false, true} {
			out := filepath.Join(t.TempDir(), "preempt")
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Hybrid: true, HybridPure: pure, Race: race, Out: out})
			if err != nil || res.Failed {
				t.Fatalf("build (race %v, pure %v): %v %+v", race, pure, err, res)
			}
			if res.Hybrid == nil || strings.Join(res.Hybrid.Asm, " ") != "spin" {
				t.Fatalf("want spin in assembly: %+v", res.Hybrid)
			}
			bins[key{race, pure}] = res.Binaries[build.Host()]
		}
	}
	for _, r := range runs {
		// a bound missed three times fails (without the polls every run
		// misses it by most of the loop)
		var errs []string
		for attempt := 0; attempt < 3; attempt++ {
			errs = preemptCheck(t, bins[key{r.race, false}], bins[key{r.race, true}], rounds, r.procs, r.race, r.stw, r.gap)
			if len(errs) == 0 {
				break
			}
			t.Logf("attempt %d: %s", attempt+1, strings.Join(errs, "; "))
		}
		for _, e := range errs {
			t.Error(e)
		}
	}
	if testing.Short() {
		return
	}
	// the control: without the polls the same check misses both bounds
	out := filepath.Join(t.TempDir(), "nopoll")
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Hybrid: true, HybridNoPoll: true, Out: out})
	if err != nil || res.Failed {
		t.Fatalf("build (no polls): %v %+v", err, res)
	}
	if errs := preemptCheck(t, res.Binaries[build.Host()], bins[key{false, true}], rounds, "4", false, 10, 100); len(errs) < 2 {
		t.Errorf("without polls: want the stop the world and main's gap past their bounds, got %v", errs)
	}
}

// preemptCheck runs the preemption program once and returns the bounds
// it missed. A miss of the fixed bounds runs the pure control (spin in
// Go) once under the same load and widens each bound to twice the
// control's figure plus a margin (20 ms for a stop the world, 50 ms for
// main's gap), capped at a quarter of the hybrid run's loop.
func preemptCheck(t *testing.T, bin, pure, rounds, procs string, race bool, stwBound float64, gapBound int) []string {
	t.Helper()
	m := preemptRun(t, "hybrid", bin, rounds, procs, race)
	errs := m.missed(procs, race, stwBound, gapBound)
	if len(errs) == 0 {
		return nil
	}
	c := preemptRun(t, "control", pure, rounds, procs, race)
	limit := m.ms / 4
	if stwBound > 0 { // 0: no stop the world bound (GOMAXPROCS=1)
		stwBound = min(max(stwBound, 2*c.stw+20), float64(limit))
	}
	gapBound = min(max(gapBound, 2*c.gap+50), limit)
	t.Logf("fixed bounds missed (%s); against the control: stop the world %.0f ms, gap %d ms (a quarter of the loop: %d ms)",
		strings.Join(errs, "; "), stwBound, gapBound, limit)
	return m.missed(procs, race, stwBound, gapBound)
}

// preemptRes is one run of the preemption program: the loop's length,
// main's longest gap, the GCs, and the longest stop the world.
type preemptRes struct {
	ms, gap, gcs int
	stw          float64
}

func (m preemptRes) missed(procs string, race bool, stwBound float64, gapBound int) []string {
	var errs []string
	if m.gap > gapBound {
		errs = append(errs, fmt.Sprintf("GOMAXPROCS=%s race=%v: main stalled %d ms (bound %d) while the hybrid loop ran %d ms", procs, race, m.gap, gapBound, m.ms))
	}
	if stwBound > 0 && m.gcs == 0 {
		errs = append(errs, fmt.Sprintf("GOMAXPROCS=%s race=%v: no GC ran during the loop", procs, race))
	}
	if stwBound > 0 && m.stw > stwBound {
		errs = append(errs, fmt.Sprintf("GOMAXPROCS=%s race=%v: a stop the world took %.3f ms (bound %.0f)", procs, race, m.stw, stwBound))
	}
	return errs
}

// preemptRun runs the preemption program once.
func preemptRun(t *testing.T, what, bin, rounds, procs string, race bool) preemptRes {
	t.Helper()
	cmd := exec.Command(bin, rounds)
	// GOGC=10 (which also turns off the Xo heap floor, decision 0065)
	// puts the heap goal near 400 KB, so main (at least 16 KB a round)
	// starts a GC every few dozen rounds while spin runs. With the default
	// floor (16 MiB at GOMAXPROCS=4) a -short loop on Linux, where a 100us
	// sleep takes about 1 ms, allocated about 12 MB and never started one
	// (gotchas/hybrid-preemption-stw-under-load.md).
	cmd.Env = append(goldenEnv(), "GOMAXPROCS="+procs, "GODEBUG=gctrace=1", "GOGC=10")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	var total, worst, iters, kept, ms int
	if _, err := fmt.Sscanf(stdout.String(), "spin %d worst_ms %d iters %d kept %d total_ms %d", &total, &worst, &iters, &kept, &ms); err != nil {
		t.Fatalf("output %q: %v", stdout.String(), err)
	}
	gcRE := regexp.MustCompile(`gc \d+ @\S+ \S+: ([\d.]+)\+[\d.]+\+([\d.]+) ms clock`)
	stw, gcs := 0.0, 0
	for _, m := range gcRE.FindAllStringSubmatch(stderr.String(), -1) {
		gcs++
		for _, x := range m[1:] {
			v, _ := strconv.ParseFloat(x, 64)
			stw = max(stw, v)
		}
	}
	t.Logf("%s GOMAXPROCS=%s race=%v: spin %d ms, main's worst gap %d ms over %d iterations, %d GCs, longest stop the world %.3f ms",
		what, procs, race, ms, worst, iters, gcs, stw)
	return preemptRes{ms: ms, gap: worst, gcs: gcs, stw: stw}
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
