package arm64

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
		run, err := exec.Command(bin).Output()
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
