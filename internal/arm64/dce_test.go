package arm64

import (
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
