package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLLVMClangTooOld: a clang that reports LLVM 15 stops an LLVM build
// with XO1505 before code generation (the generated IR calls
// llvm.threadlocal.address, LLVM 16; clang 15 linked it as an undefined
// function, CI 2026-10-20, gotchas/llvm-thread-local-constant.md).
func TestLLVMClangTooOld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake clang is a shell script")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "clang")
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'clang version 15.0.7'; echo 'Target: aarch64-unknown-linux-gnu'; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(dir, "p")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module example.com/p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(xoBin(t), "build", "--backend=llvm", "-o", filepath.Join(dir, "out"), ".")
	cmd.Dir = prog
	cmd.Env = append(os.Environ(), "XO_CLANG="+fake)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err == nil {
		t.Fatalf("xo build with clang 15 succeeded:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "XO1505") || !strings.Contains(out.String(), "clang 15") {
		t.Fatalf("want XO1505 naming clang 15, got:\n%s", out.String())
	}
}
