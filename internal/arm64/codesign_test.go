package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestCodesignAcceptsLinkerOutput: a program compiler/macho links, for
// darwin/arm64 and darwin/amd64, passes `codesign --verify --strict` with
// the linker's own ad hoc signature, and again after codesign replaces it
// with a hardened runtime signature (ad hoc here; scripts/sign-macos.sh
// signs with a Developer ID the same way). codesign rewrites
// LC_CODE_SIGNATURE and __LINKEDIT, so it rejects a file whose signature
// is not last or whose __LINKEDIT does not end the file
// (knowledgebase/infrastructure/code-signing.md).
func TestCodesignAcceptsLinkerOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("codesign runs on macOS")
	}
	codesign, err := exec.LookPath("codesign")
	if err != nil {
		t.Skip("no codesign")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-codesign-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	const want = "hello, signed"
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\""+want+"\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verify := func(t *testing.T, f string) {
		t.Helper()
		if out, err := exec.Command(codesign, "--verify", "--strict", "--verbose=2", f).CombinedOutput(); err != nil {
			t.Fatalf("codesign --verify --strict %s: %v\n%s", f, err, out)
		}
	}
	for _, tc := range []struct{ arch, backend, target string }{
		{"arm64", "arm64", "darwin/arm64"},
		{"amd64", "amd64", "darwin/amd64"},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			bin := filepath.Join(work, tc.arch, "hello")
			cmd := exec.Command(xo, "build", "--backend="+tc.backend, "--target="+tc.target, "--ld=macho", "-o", bin, ".")
			cmd.Dir = src
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, out)
			}
			verify(t, bin)
			if out, err := exec.Command(codesign, "--force", "--sign", "-", "--options", "runtime", bin).CombinedOutput(); err != nil {
				t.Fatalf("codesign --sign -: %v\n%s", err, out)
			}
			verify(t, bin)
			if tc.arch != runtime.GOARCH {
				return
			}
			out, err := exec.Command(bin).CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != want {
				t.Fatalf("the re-signed program: %v, output %q, want %q", err, out, want)
			}
		})
	}
}
