package arm64

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestDeadFunctionsDropped: the dead function removal before code
// generation (compiler/arm64 reach.xo) leaves out the runtime and std
// functions no root reaches; hello world had 1,015 functions and kept
// 441 on darwin (2026-10-18). XO_DCE=off keeps them all.
func TestDeadFunctionsDropped(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the arm64 backend builds for the host")
	}
	if _, err := exec.LookPath("nm"); err != nil {
		t.Skip("no nm")
	}
	dir := filepath.Join("..", "llvm", "testdata", "golden", "dce_values")
	count := func(env []string) (int, string) {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Env: append(env, "XO_BACKEND_CACHE=off", "XO_BIN_CACHE=off")})
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed {
			t.Fatalf("build failed:\n%s", res.Output)
		}
		bin := res.Binaries[xotool.Host()]
		out, err := exec.Command("nm", bin).Output()
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, l := range strings.Split(string(out), "\n") {
			if strings.Contains(l, "xo_f_") {
				n++
			}
		}
		run, err := testtmp.Guard(exec.Command(bin)).Output()
		if err != nil {
			t.Fatalf("%s: %v", bin, err)
		}
		return n, string(run)
	}
	on, runOn := count(nil)
	off, runOff := count([]string{"XO_DCE=off"})
	if runOn != runOff {
		t.Errorf("the program prints differently with XO_DCE=off:\n%s\nagainst\n%s", runOff, runOn)
	}
	if on*10 > off*8 {
		t.Errorf("%d functions with dead function removal, %d without: want at most 80%%", on, off)
	}
	t.Logf("%d functions, %d with XO_DCE=off", on, off)
}

// hollowSrc routes one request in process: std/http's C ABI exports
// (cabi_http.xo) and the drop helpers only they call are in its link,
// and nothing calls them.
const hollowSrc = `use std/http

fn main(os: Os) {
  var r = http.router()
  r.handle("GET /hi", fn(req: http.Request) -> http.Response { http.text(200, "hi") })
  let resp = r.call(http.new_request("GET", "/hi"))
  os.stdio.println("status ${resp.status}")
}
`

// TestHollowUnreached: the dead code removal after code generation
// (compiler/arm64 hollow.xo, decision 0371) leaves an unreached C ABI
// export and an unreached drop helper one `brk #0x484f` each, and the
// program runs as it does with XO_PASSES=-hollow.
func TestHollowUnreached(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("the pass runs for darwin/arm64, built for the host")
	}
	if _, err := exec.LookPath("otool"); err != nil {
		t.Skip("no otool")
	}
	dir, err := testtmp.Dir("xo-hollow-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module hollow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(hollowSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// build returns the bodies of the functions whose body is one
	// instruction, by name, and what the program printed
	build := func(env []string) (map[string]string, string) {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Dir: dir, Env: append(env, "XO_BACKEND_CACHE=off", "XO_BIN_CACHE=off")})
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed {
			t.Fatalf("build failed:\n%s", res.Output)
		}
		defer os.RemoveAll(res.Dir)
		bin := res.Binaries[xotool.Host()]
		out, err := exec.Command("otool", "-tV", bin).Output()
		if err != nil {
			t.Fatal(err)
		}
		bodies := map[string][]string{}
		cur := ""
		for _, l := range strings.Split(string(out), "\n") {
			if l != "" && l[0] != ' ' && l[0] != '\t' && strings.HasSuffix(l, ":") && !strings.Contains(l, "(__TEXT") {
				cur = strings.TrimSuffix(l, ":")
				bodies[cur] = nil
			} else if _, ins, ok := strings.Cut(l, "\t"); ok && cur != "" {
				bodies[cur] = append(bodies[cur], strings.Join(strings.Fields(ins), " "))
			}
		}
		one := map[string]string{}
		for name, b := range bodies {
			if len(b) == 1 {
				one[name] = b[0]
			}
		}
		run, err := testtmp.Guard(exec.Command(bin)).Output()
		if err != nil {
			t.Fatalf("%s: %v", bin, err)
		}
		return one, string(run)
	}
	on, runOn := build(nil)
	off, runOff := build([]string{"XO_PASSES=-hollow"})
	if runOn != "status 200\n" || runOff != runOn {
		t.Errorf("printed %q, with XO_PASSES=-hollow %q; want %q", runOn, runOff, "status 200\n")
	}
	const brk = "brk #0x484f"
	var export, drop string
	for name, ins := range on {
		if ins != brk {
			continue
		}
		if off[name] == brk {
			t.Errorf("%s is a brk with XO_PASSES=-hollow too", name)
		}
		if strings.HasPrefix(name, "_xo_x_") {
			export = name
		}
		if strings.HasPrefix(name, "_xo_h") || strings.HasPrefix(name, "_xo_f_drop") {
			drop = name
		}
	}
	if export == "" {
		t.Error("no C ABI export (_xo_x_) is one brk")
	}
	if drop == "" {
		t.Error("no drop helper (_xo_h, _xo_f_drop) is one brk")
	}
	t.Logf("one brk: export %s, drop helper %s", export, drop)
}

// TestHollowUnreachedAmd64: the same pass on an amd64 build of the same
// program (compiler/arm64 hollow.xo, decision 0371's amd64 addendum)
// leaves the unreached export and drop helper one `ud2` each, and matches
// XO_PASSES=-hollow. The pass is on by default for darwin/amd64 too
// (2026-10-26, after its binaries ran on an Intel Mac), so the first build
// names no pass; the binary itself does not run on the development Mac (no
// Rosetta), and the test says so.
func TestHollowUnreachedAmd64(t *testing.T) {
	if testing.Short() {
		t.Skip("cross builds a program twice and disassembles it")
	}
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("the amd64 build for darwin is cross built here")
	}
	if _, err := exec.LookPath("otool"); err != nil {
		t.Skip("no otool")
	}
	dir, err := testtmp.Dir("xo-hollow-amd64-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module hollow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(hollowSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// build returns the bodies of the functions whose body is one
	// instruction, by name, and what the program printed ("" when the
	// host cannot run an x86_64 binary)
	build := func(env []string) (map[string]string, string) {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: "amd64", Targets: "darwin/amd64", Dir: dir, Env: append(env, "XO_BACKEND_CACHE=off", "XO_BIN_CACHE=off")})
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed {
			t.Fatalf("build failed:\n%s", res.Output)
		}
		defer os.RemoveAll(res.Dir)
		bin, ok := res.Binaries["darwin/amd64"]
		if !ok {
			t.Fatalf("no darwin/amd64 binary in %v", res.Binaries)
		}
		one := otoolOneInstr(t, bin)
		run := ""
		if out, err := exec.Command(bin).Output(); err == nil {
			run = string(out)
		} else {
			t.Logf("the x86_64 binary did not run here (%v); the bodies are still checked", err)
		}
		return one, run
	}
	on, runOn := build(nil)
	off, runOff := build([]string{"XO_PASSES=-hollow"})
	if runOn != "" && runOn != "status 200\n" {
		t.Errorf("printed %q, want %q", runOn, "status 200\n")
	}
	if runOff != "" && runOff != runOn {
		t.Errorf("printed %q with XO_PASSES=-hollow, want %q", runOff, runOn)
	}
	const ud2 = "ud2"
	var export, drop string
	for name, ins := range on {
		if ins != ud2 {
			continue
		}
		if off[name] == ud2 {
			t.Errorf("%s is ud2 with XO_PASSES=-hollow too", name)
		}
		if strings.HasPrefix(name, "_xo_x_") {
			export = name
		}
		if strings.HasPrefix(name, "_xo_h") || strings.HasPrefix(name, "_xo_f_drop") {
			drop = name
		}
	}
	if export == "" {
		t.Error("no C ABI export (_xo_x_) is one ud2")
	}
	if drop == "" {
		t.Error("no drop helper (_xo_h, _xo_f_drop) is one ud2")
	}
	t.Logf("one ud2: export %s, drop helper %s", export, drop)
}

// otoolOneInstr returns, from otool -tV of a Mach-O binary, the functions
// whose body is one instruction, by symbol. The assembler pads a hollowed
// 2 byte body to the function's alignment with nops, which are not the
// body's own instructions and are dropped here.
func otoolOneInstr(t *testing.T, bin string) map[string]string {
	t.Helper()
	if _, err := exec.LookPath("otool"); err != nil {
		t.Skip("no otool")
	}
	out, err := exec.Command("otool", "-tV", bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string][]string{}
	cur := ""
	for _, l := range strings.Split(string(out), "\n") {
		if l != "" && l[0] != ' ' && l[0] != '\t' && strings.HasSuffix(l, ":") && !strings.Contains(l, "(__TEXT") {
			cur = strings.TrimSuffix(l, ":")
			bodies[cur] = nil
		} else if _, ins, ok := strings.Cut(l, "\t"); ok && cur != "" {
			bodies[cur] = append(bodies[cur], strings.Join(strings.Fields(ins), " "))
		}
	}
	one := map[string]string{}
	for name, b := range bodies {
		var ins []string
		for _, x := range b {
			if !strings.HasPrefix(x, "nop") {
				ins = append(ins, x)
			}
		}
		if len(ins) == 1 {
			one[name] = ins[0]
		}
	}
	return one
}

// TestHollowPassesChangedCaches: a build with XO_PASSES=-hollow must not
// be served a plain build's cached results (nor the reverse), since the
// cache keys do not say which passes ran (compiler/xo main.xo
// passes_changed; compiler/xo bcache.xo). The plain build (hollowed by
// default on darwin/amd64 since 2026-10-26) fills a private cache, then
// the same program is built with -hollow in it: the second binary must
// carry no ud2 body where the plain one has.
func TestHollowPassesChangedCaches(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice and disassembles it")
	}
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("the amd64 build for darwin is cross built here")
	}
	dir, err := testtmp.Dir("xo-hollow-cache-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cache := filepath.Join(dir, "cache")
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module hollow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(hollowSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(env ...string) map[string]string {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: "amd64", Targets: "darwin/amd64", Dir: dir,
			Env: append([]string{"XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR=" + cache}, env...)})
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed {
			t.Fatalf("build failed:\n%s", res.Output)
		}
		defer os.RemoveAll(res.Dir)
		bin, ok := res.Binaries["darwin/amd64"]
		if !ok {
			t.Fatalf("no darwin/amd64 binary in %v", res.Binaries)
		}
		return otoolOneInstr(t, bin)
	}
	plain := build()
	noHollow := build("XO_PASSES=-hollow")
	const ud2 = "ud2"
	var export string
	for name, ins := range plain {
		if ins == ud2 && strings.HasPrefix(name, "_xo_x_") {
			if noHollow[name] == ud2 {
				t.Errorf("%s is ud2 with XO_PASSES=-hollow over the plain build's cache", name)
			}
			export = name
		}
	}
	if export == "" {
		t.Error("no C ABI export (_xo_x_) is one ud2 in the plain build, so the test compares nothing")
	}
	t.Logf("one ud2 in the plain build, a body with -hollow over its cache: export %s", export)
}

// TestMapLogStress runs the map_log_stress golden (decision 0372: copies
// of one table written by their tasks while others read it, and a Mutex
// store's writes and snapshots across tasks, every read checked) five
// times at each of XO_THREADS 1, 2, 4 and 8, and its --race build at 2
// and 8, which must report no race.
func TestMapLogStress(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice and runs it 22 times")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the arm64 backend builds for the host")
	}
	dir := filepath.Join("..", "llvm", "testdata", "golden", "map_log_stress")
	const want = "bad 0 base true small 40\n"
	// the --race build links clang's ThreadSanitizer runtime, found as
	// internal/llvm TestTSanSharedMaps finds it; with none the race
	// subtest says so and skips
	raceEnv, why := tsanRuntimeEnv()
	run := func(t *testing.T, race bool, env []string, threads []int, runs int) {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64, Race: race, Env: env})
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed {
			t.Fatalf("build (race %v) failed:\n%s", race, res.Output)
		}
		if res.Dir != "" {
			defer os.RemoveAll(res.Dir)
		}
		bin := res.Binaries[xotool.Host()]
		for _, n := range threads {
			for i := 0; i < runs; i++ {
				cmd := exec.Command(bin)
				cmd.Env = append(os.Environ(), fmt.Sprintf("XO_THREADS=%d", n))
				out, err := testtmp.Guard(cmd).CombinedOutput()
				if err != nil || string(out) != want {
					t.Fatalf("race %v XO_THREADS=%d run %d: %v\n%s", race, n, i, err, out)
				}
			}
		}
	}
	t.Run("plain", func(t *testing.T) { run(t, false, nil, []int{1, 2, 4, 8}, 5) })
	t.Run("race", func(t *testing.T) {
		if why != "" {
			t.Log("the --race half did not run: " + why)
			t.Skip(why)
		}
		run(t, true, raceEnv, []int{2, 8}, 1)
	})
}

// tsanRuntimeEnv is the environment of a darwin --race build
// (XO_RACE_RUNTIME, clang's ThreadSanitizer runtime), or why there is none.
func tsanRuntimeEnv() ([]string, string) {
	if runtime.GOOS != "darwin" {
		return nil, "the arm64 --race build links here on darwin only"
	}
	if os.Getenv("XO_RACE_RUNTIME") != "" {
		return nil, ""
	}
	cl, err := xotool.Clang()
	if err != nil {
		return nil, "no clang: " + err.Error()
	}
	out, err := exec.Command(cl, "-print-file-name=lib/darwin/libclang_rt.tsan_osx_dynamic.dylib").Output()
	if err != nil {
		return nil, "no ThreadSanitizer runtime: " + err.Error()
	}
	p := strings.TrimSpace(string(out))
	if _, err := os.Stat(p); err != nil || !filepath.IsAbs(p) {
		return nil, "no ThreadSanitizer runtime at " + p
	}
	return []string{"XO_RACE_RUNTIME=" + p}, ""
}
