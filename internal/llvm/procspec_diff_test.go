package llvm_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// procSpecProgram runs env(1) with the environments and directories
// whose building std/native/llrt/procspec.xo does (decision 0193 step 6)
// and prints what the child saw, in its order.
const procSpecProgram = `fn show(r: Result[ProcOutput, ProcErr]) -> Str {
  match r {
    Ok(o) => "${o.code}\n${o.stdout.to_str() ?? "?"}"
    Err(e) => "error ${e}"
  }
}

fn main(os: Os) ! {
  let p = os.proc
  let out = os.stdio
  out.println("inherited:\n" + show(p.cmd("env").output()))
  out.println("dir .:\n" + show(p.cmd("env").dir(".").output()))
  out.println("dir sub/../:\n" + show(p.cmd("env").dir("sub/../").output()))
  out.println("dir absolute:\n" + show(p.cmd("env").dir("/").output()))
  out.println("added:\n" + show(p.cmd("env").set_env("XO_SPEC_Z", "1").set_env("PATH", "/usr/bin:/bin").set_env("XO_SPEC_Z", "2").set_env("XO_SPEC_A", "a b").output()))
  out.println("added with a dir:\n" + show(p.cmd("env").dir(".").set_env("XO_SPEC_B", "").output()))
  out.println("set:\n" + show(p.cmd("env").env(os.env.only(["PATH", "XO_SPEC_D", "XO_SPEC_C"])).output()))
  out.println("set and added:\n" + show(p.cmd("env").env(os.env.only(["PATH", "XO_SPEC_C"])).set_env("XO_SPEC_C", "over").set_env("XO_SPEC_E", "=eq").dir("sub").output()))
  out.println("empty:\n" + show(p.cmd("/usr/bin/env").env(os.env.only([])).output()))
  out.println("args:\n" + show(p.cmd("printf", ["[%s]", "", "a b", "q\"uote", "back\\\\slash", "tab\tx"]).output()))
}
`

// TestProcSpecDiff runs procSpecProgram built with the Go backend (Go's
// os/exec) and the native backends and requires the same output: the
// children's environments in their order (dedupEnv, PWD, set and added
// variables) and their arguments. XO_LLRT_C=procspec checks the C path.
func TestProcSpecDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with three backends")
	}
	if runtime.GOOS == "windows" {
		t.Skip("runs env(1)")
	}
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(procSpecProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	run := t.TempDir()
	if err := os.Mkdir(filepath.Join(run, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	backends := []string{build.BackendGo, build.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, build.BackendArm64)
	}
	outs := make([]string, len(backends))
	for i, be := range backends {
		res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Diagnostics)
		}
		cmd := exec.Command(res.Binaries[build.Host()])
		cmd.Dir = run
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "PWD=/stale", "XO_SPEC_C=c", "XO_SPEC_D=d", "XO_SPEC_Z=0", "LANG=C"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", be, err, out)
		}
		outs[i] = string(out)
	}
	t.Logf("go backend:\n%s", outs[0])
	for i, be := range backends[1:] {
		if outs[i+1] != outs[0] {
			t.Errorf("%s backend differs from the Go backend\n%s", be, lineDiff(outs[0], outs[i+1]))
		}
	}
}
