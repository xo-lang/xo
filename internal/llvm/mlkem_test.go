package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestMLKEMAccumulated runs C2SP CCTV's accumulated ML-KEM-768 test of
// 10,000 iterations (std/native/llrt/mlkem.xo through internal.crypto_op;
// Go's crypto/mlkem TestAccumulated has the same digest) on the native
// backends: the golden crypto_mlkem runs 100.
func TestMLKEMAccumulated(t *testing.T) {
	if testing.Short() {
		t.Skip("10,000 key generations and encapsulations on two backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	prog := "use std/internal\n\nfn main(os: Os) {\n  os.stdio.println(internal.crypto_op(\"mlkem.accumulated\", \"10000\".bytes(), b\"\", b\"\", b\"\").to_hex())\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/mlkemacc\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	const want = "8a518cc63da366322a8e7a818c7a0d63483cb3528d34a4cf42f35d5ad73f22fc"
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res.Output)
		}
		start := time.Now()
		out, err := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()])).Output()
		if err != nil {
			t.Fatalf("%s: %v", be, err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s: %s, want %s", be, got, want)
		}
		t.Logf("%s: %v", be, time.Since(start))
	}
}
