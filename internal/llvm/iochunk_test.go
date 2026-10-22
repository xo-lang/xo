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

// TestRuntimeIOChunk runs testdata/rt/iochunk on both native backends:
// with the most bytes one read(2) or write(2) asks for set to 7
// (std/native/llrt/raw.xo io_len, std/internal rt_probe "io_chunk"),
// whole files, file handles, standard output, and a child's standard
// input move in many chunks, and a write to a child that stops reading
// fails partway. XO_IO_CHUNK_BIG=1 (passed on) adds a real file of 2^31
// + 16 bytes, written and read back with the default chunk (macOS refused
// a write above INT_MAX before the chunks; slow, 2 GB of disk and about 6
// GB of memory).
func TestRuntimeIOChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	dir := filepath.Join("testdata", "rt", "iochunk")
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
			}
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
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
			for _, name := range []string{"whole file", "file handle", "child stdin", "reader gone midway"} {
				if !strings.Contains(string(out), name+": ok\n") {
					t.Errorf("no %q:\n%s", name+": ok", out)
				}
			}
			if os.Getenv("XO_IO_CHUNK_BIG") == "1" && !strings.Contains(string(out), "past INT_MAX: ok\n") {
				t.Errorf("no \"past INT_MAX: ok\":\n%s", out)
			}
		})
	}
}
