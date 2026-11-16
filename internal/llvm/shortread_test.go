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

// TestRuntimeShortRead runs testdata/rt/shortread on both native backends
// (task #86): a 300 byte reply read with c.read(65536) keeps an object of
// at most twice its length, where netconn's read handed back the whole
// 64 KiB buffer (aws-43's sql stress: Xo RSS 93 to 163 MB, Go flat 25 MB).
func TestRuntimeShortRead(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	dir := filepath.Join("testdata", "rt", "shortread")
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
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
			if !strings.Contains(string(out), "short read: ok\n") {
				t.Errorf("no \"short read: ok\":\n%s", out)
			}
		})
	}
}
