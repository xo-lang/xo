package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultBackendAmd64 builds a program for linux/amd64 and
// windows/amd64 with no --backend (decision 0270: the native backend of
// the targets' architecture) and with --backend=amd64, to the same output
// path: the binaries must be the same. The build read the --backend flag
// rather than the backend it chose for the lowering flags (soft_cpu, the
// Windows link without a C runtime), so a default amd64 build differed
// (gotchas/default-backend-read-from-flag.md). cpu_prims uses cpu.xo's
// carry arithmetic, which soft_cpu lowers.
func TestDefaultBackendAmd64(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	t.Parallel()
	dir := filepath.Join(repoRoot, "internal", "llvm", "testdata", "golden", "cpu_prims")
	chdir(t, dir)
	for _, tg := range []string{"linux/amd64", "windows/amd64"} {
		out := filepath.Join(t.TempDir(), "prog")
		build := func(flags ...string) []byte {
			var stdout, stderr bytes.Buffer
			args := append(append([]string{"build"}, flags...), "--targets="+tg, "-o", out, ".")
			if code := run(t, args, &stdout, &stderr); code != 0 {
				t.Fatalf("xo %v: exit %d\n%s%s", args, code, stdout.String(), stderr.String())
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		def := build()
		amd := build("--backend=amd64")
		if !bytes.Equal(def, amd) {
			t.Errorf("%s: the default backend's binary (%d bytes) differs from --backend=amd64's (%d bytes)", tg, len(def), len(amd))
		}
	}
}
