package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLLRTCUnknownName requires a build to fail on an XO_LLRT_C name
// with no std/native/llrt/<name>_c.xo (compiler/xo/main.xo
// llrt_c_names): decision 0198 deleted most C paths, and a deleted name
// that was silently ignored built the Xo path while a test claimed to
// compare it with the C one
// (knowledgebase/gotchas/deleted-llrt-switch-still-set.md). No C path
// is left since B22 deleted net_c.xo (2026-10-14), so every name fails;
// the error names it. Both the native driver and the Go backend's check
// the names.
func TestLLRTCUnknownName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/llrtc\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	setenv(t, "XO_LLRT_C", "rc")
	for _, be := range []string{"go", "arm64", "llvm"} {
		var out, errOut bytes.Buffer
		code := run(t, []string{"build", "--backend=" + be, "-o", filepath.Join(dir, "bin-"+be), dir}, &out, &errOut)
		msg := errOut.String()
		if code != 1 || !strings.Contains(msg, "XO_LLRT_C=rc: no C path named rc") {
			t.Errorf("--backend=%s: exit %d, want 1 and an error naming rc\n%s%s", be, code, out.String(), msg)
		}
	}
}
