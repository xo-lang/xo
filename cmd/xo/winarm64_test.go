package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
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
	checkArm64Unwind(t, exe, b)
}

// checkArm64Unwind: decision 0569 stage 2. The ARM64 PE's .pdata and
// .xdata pass xo dev debug --unwind-check (every unwind code matches the
// instruction it stands for, each frame aligned with its saves inside it
// and freed by every epilog), with no llvm-readobj; with each function's
// first instruction made a nop, the check finds the prologs that no
// longer match.
func checkArm64Unwind(t *testing.T, exe string, b []byte) {
	t.Helper()
	r := xo(t, "dev", "debug", "--unwind-check", exe)
	var n, problems int
	if _, err := fmt.Sscanf(r.out, "%d functions, %d problems", &n, &problems); err != nil || n < 100 || r.code != 0 {
		t.Fatalf("xo dev debug --unwind-check: exit %d, want at least 100 functions and no problems:\n%s%s", r.code, r.out, r.err)
	}
	f, err := pe.NewFile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	pd := f.Section(".pdata")
	if pd == nil {
		t.Fatal("no .pdata")
	}
	data, err := pd.Data()
	if err != nil {
		t.Fatalf(".pdata: %v", err)
	}
	bad := append([]byte(nil), b...)
	for i := 0; i+8 <= len(data); i += 8 {
		rva := binary.LittleEndian.Uint32(data[i:])
		for _, sc := range f.Sections {
			if rva >= sc.VirtualAddress && rva+4 <= sc.VirtualAddress+sc.Size {
				binary.LittleEndian.PutUint32(bad[sc.Offset+rva-sc.VirtualAddress:], 0xd503201f)
			}
		}
	}
	mut := exe + ".nop.exe"
	if err := os.WriteFile(mut, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	r = xo(t, "dev", "debug", "--unwind-check", mut).want(1)
	if !strings.Contains(r.err, "prolog instruction at +0x0 (0xd503201f)") {
		t.Errorf("the nop prologs were not found:\n%s%s", r.out, r.err)
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
