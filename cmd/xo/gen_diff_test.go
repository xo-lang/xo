package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// genDiffFile is the test file of TestGenDiff: for each seed, a test
// whose expectation fails and prints the values testing.gen makes of
// every scalar type (and of collections of them, which draw sizes and
// nest), so the report holds every value.
func genDiffFile() string {
	var b strings.Builder
	b.WriteString("use std/testing\n")
	types := []string{
		"(Int, Int8, Int16, Int32)",
		"(UInt8, UInt16, UInt32, UInt64)",
		"(Float, Float32, Rune, Str)",
		"(Bytes, Duration, Time, Bool)",
		"List[(Int8, UInt64, Option[Rune])]",
		"Map[Str, List[Float32]]",
	}
	for seed := 1; seed <= 100; seed++ {
		for k, t := range types {
			fmt.Fprintf(&b, "\ntest \"gen %d %d\" {\n  expect(testing.gen[%s](%d)) == testing.gen[%s](%d)\n}\n", seed, k, t, seed, t, seed+100000)
		}
	}
	return b.String()
}

// TestGenDiff compares the values testing.gen makes on the Go backend
// and on both native backends, with the scalar values in Xo
// (std/native/llrt/gen.xo, decision 0195; decision 0198 deleted the C
// path): every test fails, printing its values, and the reports are the
// same.
func TestGenDiff(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds test binaries on every backend")
	}
	backends := nativeBackends(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/gen\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gen_test.xo"), []byte(genDiffFile()), 0o644); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	goCode, goOut := xoTest(t, "-v", dir)
	// a few pairs are equal (two empty lists)
	if goCode == 0 || strings.Count(goOut, "--- FAIL") < 550 {
		t.Fatalf("go backend: exit %d, %d failures, want most of 600\n%s", goCode, strings.Count(goOut, "--- FAIL"), goOut)
	}
	for _, be := range backends {
		code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
		if code != goCode || out != goOut {
			t.Errorf("%s differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", be, goCode, code, goOut, be, out)
		}
	}
}
