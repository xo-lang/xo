package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestXoTestNoTests: `xo test` on a package with no tests says so on
// standard error and exits 0, and with --fail-empty exits 1 (2026-10-24:
// a gate's `xo test --std-module=time`, a module with no tests, printed
// "ok: 0 passed" and read as a pass).
func TestXoTestNoTests(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) uses (stdio) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/notests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run(t, []string{"test", dir}, &out, &errb); code != 0 || !strings.Contains(errb.String(), "xo test: no tests in") {
		t.Fatalf("xo test: want exit 0 and a no tests line, got %d:\n%s%s", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run(t, []string{"test", "--fail-empty", dir}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "(--fail-empty)") {
		t.Fatalf("xo test --fail-empty: want exit 1, got %d:\n%s%s", code, out.String(), errb.String())
	}
}

// TestXoTestStdModuleNames (decision 0437): `xo test --std-module` takes
// a bare name for the module with tests (base64 is native/base64; it ran
// std/base64, the Go backend's, with no tests, and passed), fails a run
// that tests nothing, and rejects an unknown name with exit 2 and the
// nearest tested modules; `xo test` on a path in std names the flag.
func TestXoTestStdModuleNames(t *testing.T) {
	t.Parallel()
	xt := func(args ...string) (int, string, string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, append([]string{"test"}, args...), &out, &errb)
		return code, out.String(), errb.String()
	}
	if code, out, errs := xt("--std-module=base64"); code != 0 || !strings.Contains(out, "ok: ") || strings.Contains(out, "ok: 0 passed") {
		t.Errorf("--std-module=base64: want native/base64's tests run, exit %d:\n%s%s", code, out, errs)
	}
	if code, out, errs := xt("--std-module=bas64"); code != 2 || !strings.Contains(errs, "did you mean native/base64") {
		t.Errorf("--std-module=bas64: want exit 2 and a did-you-mean, exit %d:\n%s%s", code, out, errs)
	}
	if code, out, errs := xt("--std-module=backoff"); code != 2 || !strings.Contains(errs, "has no tests") {
		t.Errorf("--std-module=backoff (no tests): want exit 2, exit %d:\n%s%s", code, out, errs)
	}
	if code, out, errs := xt(filepath.Join(xoStd(), "native", "base64.xo")); code != 2 || !strings.Contains(errs, "--std-module=native/base64") {
		t.Errorf("xo test on std's base64.xo: want exit 2 naming --std-module=native/base64, exit %d:\n%s%s", code, out, errs)
	}
}
