package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// iselCertType is x509cert.Cert's LLVM type (59 fields), which an
// fn-value call passes by value (decision 0145 makes only direct calls
// pass it by reference).
const iselCertType = `{ %xo.str, %xo.str, i64, %xo.str, i64, %xo.str, %xo.str, %xo.str, %xo.str, i64, i64, i64, %xo.str, %xo.str, ptr, ptr, i1, i1, i1, i64, i64, ptr, ptr, i1, %xo.str, %xo.str, ptr, ptr, i64, i64, i64, %xo.str, %xo.str, i1, %xo.str, i1, ptr, ptr, ptr, ptr, ptr, ptr, ptr, ptr, ptr, ptr, ptr, i1, i64, i1, i64, i1, i64, ptr, ptr, ptr, ptr, i1, ptr }`

const iselCallee = `%xo.str = type { ptr, i64 }
%Cert = type CERT
define ptr @callee({ ptr, ptr } %env, i64 %k, %Cert %c, ptr %chain) noinline {
  ret ptr %chain
}
`

const iselCaller = `%xo.str = type { ptr, i64 }
%Cert = type CERT
@fp = global ptr null
@marker = global i64 0
declare ptr @callee({ ptr, ptr }, i64, %Cert, ptr)
define i32 @main() {
  store volatile ptr @callee, ptr @fp
  %f = load volatile ptr, ptr @fp
  %c0 = insertvalue %Cert zeroinitializer, i64 7, 2
  %c1 = insertvalue %Cert %c0, ptr @fp, 58
  %r = call ptr %f({ ptr, ptr } zeroinitializer, i64 3, %Cert %c1, ptr @marker)
  %ok = icmp eq ptr %r, @marker
  %bad = select i1 %ok, i32 0, i32 1
  ret i32 %bad
}
`

// TestISelMixedLevels: a -O0 module and a -O2 module of xo's IR, as a test
// build's split makes them (decision 0619 compiles a module with a
// constant time function at -O2), agree on a large aggregate argument
// with xo's -O0 flags (compiler/xo split.xo isel_flags,
// -fno-global-isel). With clang's arm64 -O0 default (GlobalISel) Apple
// clang 21 passed the argument after it from another slot
// (gotchas/llvm-globalisel-aggregate-args.md).
func TestISelMixedLevels(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("GlobalISel is clang's -O0 default on arm64 only")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("no clang")
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(strings.ReplaceAll(text, "CERT", iselCertType)), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	caller := write("caller.ll", iselCaller)
	callee := write("callee.ll", iselCallee)
	// run links the caller compiled with a and the callee with b; whether
	// the callee saw the caller's last argument
	run := func(a, b []string) bool {
		t.Helper()
		for _, c := range []struct {
			src   string
			flags []string
		}{{caller, a}, {callee, b}} {
			args := append(append([]string{}, c.flags...), "-Wno-override-module", "-c", c.src, "-o", c.src+".o")
			if out, err := exec.Command(clang, args...).CombinedOutput(); err != nil {
				t.Fatalf("clang %v: %v\n%s", args, err, out)
			}
		}
		bin := filepath.Join(dir, "t")
		if out, err := exec.Command(clang, caller+".o", callee+".o", "-o", bin).CombinedOutput(); err != nil {
			t.Fatalf("link: %v\n%s", err, out)
		}
		return exec.Command(bin).Run() == nil
	}
	o0 := []string{"-O0", "-fno-global-isel"}
	o2 := []string{"-O2"}
	if !run(o0, o2) || !run(o2, o0) {
		t.Errorf("xo's -O0 flags %v and -O2 disagree on a large aggregate argument", o0)
	}
	if run([]string{"-O0"}, o2) && run(o2, []string{"-O0"}) {
		t.Logf("this clang's GlobalISel and SelectionDAG agree (the gotcha may be fixed in it)")
	}
}
