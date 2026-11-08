package main

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestWindowsArm64Assembly: decision 0569 stage 1. windows/arm64 builds
// only with --backend=arm64 named. Its assembly has .seh unwind data
// and no write of x18 (the TEB); where clang is on PATH (its assembler),
// xo build links an ARM64 PE (machine 0xAA64) with Xo's PE linker. The
// default build still names --backend=go (TestTargetsAllNative).
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
	text := xo(t, "asm", "--backend=arm64", "--targets=windows/arm64", ".").want(0).out
	for _, want := range []string{".seh_proc xo_f_main", ".seh_endprologue", "[x18, #88]", ".section .tls$", "xo_win_start"} {
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("no clang to assemble it")
	}
	exe := filepath.Join(dir, "hello.exe")
	xo(t, "build", "--backend=arm64", "--targets=windows/arm64", "-o", exe, ".").want(0)
	b, err := os.ReadFile(exe)
	if err != nil || len(b) < 0x40 || b[0] != 'M' || b[1] != 'Z' {
		t.Fatalf("not a PE image: %v", err)
	}
	pe := int(binary.LittleEndian.Uint32(b[0x3c:]))
	if pe+6 > len(b) || string(b[pe:pe+4]) != "PE\x00\x00" || binary.LittleEndian.Uint16(b[pe+4:]) != 0xaa64 {
		t.Errorf("not an ARM64 PE image (machine 0xAA64)")
	}
}

// TestWindowsArm64UseC: decision 0569 stage 1. A windows/arm64 program
// with `use c` links with Xo's PE linker: clang compiles the adapter and
// the link file against XO_WINSYSROOT's headers, and `link "user32"`
// reads the sysroot's arm64 User32.Lib, so the image imports
// GetSystemMetrics from USER32.dll. Needs XO_WINSYSROOT (an xwin
// sysroot with arm64) and clang; nothing runs the image.
func TestWindowsArm64UseC(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	if os.Getenv("XO_WINSYSROOT") == "" {
		t.Skip("XO_WINSYSROOT names no Windows sysroot")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("no clang")
	}
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/tiny\n",
		"tiny.h": "int tiny_add(int a, int b);\ndouble tiny_half(double x);\nint tiny_metrics(void);\n",
		"tiny.c": "#include <string.h>\n#include \"tiny.h\"\n__declspec(dllimport) int __stdcall GetSystemMetrics(int index);\n" +
			"int tiny_add(int a, int b) { return a + b; }\ndouble tiny_half(double x) { return x / 2; }\n" +
			"int tiny_metrics(void) { return GetSystemMetrics(0) > 0; }\n",
		"main.xo": "use c \"tiny.h\" link \"tiny.c\" link \"user32\"\n\nfn main(os: Os) {\n" +
			"  os.stdio.println(\"${tiny.tiny_add(40, 2)} ${tiny.tiny_half(3.0)} ${tiny.tiny_metrics()}\")\n}\n",
	})
	chdir(t, dir)
	exe := filepath.Join(dir, "tiny.exe")
	xo(t, "build", "--backend=arm64", "--targets=windows/arm64", "-o", exe, ".").want(0)
	b, err := os.ReadFile(exe)
	if err != nil || len(b) < 0x40 || b[0] != 'M' || b[1] != 'Z' {
		t.Fatalf("not a PE image: %v", err)
	}
	pe := int(binary.LittleEndian.Uint32(b[0x3c:]))
	if pe+6 > len(b) || binary.LittleEndian.Uint16(b[pe+4:]) != 0xaa64 {
		t.Errorf("not an ARM64 PE image (machine 0xAA64)")
	}
	for _, want := range []string{"USER32.dll", "GetSystemMetrics", "tiny_add"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the image has no %q", want)
		}
	}
}
