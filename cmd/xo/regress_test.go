package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegressionWritten checks that a shrunk contract fuzz failure becomes a
// regression test in the module (core 9.2), once, and that it fails.
func TestRegressionWritten(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a test binary")
	}
	dir := filepath.Join(t.TempDir(), "halver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "fn half(n: Int) -> Int\n  ensures result * 2 == n\n{\n  n / 2\n}\n\npub fn twice(n: Int) -> Int { half(n) * 4 }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	// contract fuzzing is the Go backend's (decision 0248: named, since
	// the default is native, where it is skipped)
	if code := runTest(t, []string{"--backend=go", dir}, &out, &errb); code == 0 {
		t.Fatalf("want the contract fuzz to fail\n%s%s", out.String(), errb.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, regressFile))
	if err != nil {
		t.Fatalf("no regression file: %v\n%s%s", err, out.String(), errb.String())
	}
	if !strings.Contains(string(got), `test "regression: halver.half (seed `) || !strings.Contains(string(got), "half(arg_n)") {
		t.Fatalf("unexpected regression file:\n%s", got)
	}
	out.Reset()
	if code := runTest(t, []string{dir}, &out, &errb); code == 0 || !strings.Contains(out.String(), "--- FAIL: regression: halver.half") {
		t.Fatalf("want the regression test to fail\n%s", out.String())
	}
	again, _ := os.ReadFile(filepath.Join(dir, regressFile))
	if string(again) != string(got) {
		t.Fatalf("regression written twice:\n%s", again)
	}
}
