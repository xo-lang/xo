package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestWindowsArm64Assembly: decision 0569 stage 1. windows/arm64 builds
// only with --backend=arm64 named; xo writes the assembly and says it
// does not link yet (exit 1). Where clang is on PATH, the assembly
// assembles for aarch64-windows-msvc, with .seh unwind data and no
// write of x18 (the TEB). The default build still names --backend=go
// (TestTargetsAllNative).
func TestWindowsArm64Assembly(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/hello\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hi ${40 + 2}\")\n}\n",
	})
	chdir(t, dir)
	r := xo(t, "build", "--backend=arm64", "--targets=windows/arm64", "-o", filepath.Join(dir, "hello"), ".").want(1)
	m := regexp.MustCompile(`does not link it yet: (\S+\.s)`).FindStringSubmatch(r.err)
	if m == nil {
		t.Fatalf("no assembly named:\n%s", r.err)
	}
	asm, err := os.ReadFile(m[1])
	if err != nil {
		t.Fatal(err)
	}
	text := string(asm)
	for _, want := range []string{".seh_proc xo_f_main", ".seh_endprologue", "[x18, #88]", ".section .tls$"} {
		if !strings.Contains(text, want) {
			t.Errorf("assembly has no %q", want)
		}
	}
	for _, bad := range []string{".cfi_", "tpidr_el0", ":tprel", "@progbits"} {
		if strings.Contains(text, bad) {
			t.Errorf("assembly has %q", bad)
		}
	}
	if regexp.MustCompile(`(?m)^\t[a-z0-9.]+ [xw]18,|^\tldp [xw][0-9]+, [xw]18,`).MatchString(text) {
		t.Errorf("an instruction writes x18")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("no clang to assemble it")
	}
	obj := filepath.Join(dir, "prog.obj")
	if out, err := exec.Command(clang, "-target", "aarch64-windows-msvc", "-c", "-o", obj, m[1]).CombinedOutput(); err != nil {
		t.Fatalf("clang -target aarch64-windows-msvc: %v\n%s", err, out)
	}
	b, err := os.ReadFile(obj)
	if err != nil || len(b) < 2 || b[0] != 0x64 || b[1] != 0xaa {
		t.Errorf("not an ARM64 COFF object (machine 0xAA64): %x", b[:2])
	}
}
