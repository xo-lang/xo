package llvm

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

// TestRuntimeRandChaCha8 runs testdata/rt/randchacha on both native
// backends (decision 0669): the native Rand's ChaCha8 generator seeded
// with Go's internal/chacha8rand test seed gives Go's TestOutput vector
// (372 words, three key erasures), and a forked child's next Rand word
// differs from its parent's (Linux MADV_WIPEONFORK, FreeBSD INHERIT_ZERO,
// macOS's pthread_atfork child handler; a child that inherits the state
// repeats the parent's word and fails here).
func TestRuntimeRandChaCha8(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no fork on Windows")
	}
	dir := filepath.Join("testdata", "rt", "randchacha")
	// the native emitter's x86-64 lowering (ssa x86chacha.xo
	// chacha8rand_x86_lines, task #87) runs as the amd64 backend
	for _, be := range []string{xotool.BackendArm64, "amd64", xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
			}
			if be == "amd64" && runtime.GOARCH != "amd64" {
				t.Skip("the amd64 backend runs on amd64 hosts")
			}
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, Env: libcEnv()})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if res.Dir != "" {
				defer os.RemoveAll(res.Dir)
			}
			if res.Failed {
				t.Fatalf("build failed:\n%s", res.Output)
			}
			cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			for _, name := range []string{"kat", "fork", "bytes"} {
				if !strings.Contains(string(out), name+": ok\n") {
					t.Errorf("no %q:\n%s", name+": ok", out)
				}
			}
		})
	}
}
