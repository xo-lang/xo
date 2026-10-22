package llvm

import (
	"bytes"
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

// TestTSanSharedMaps: Maps shared between tasks under a Mutex (read with
// with_read, written with with, values and whole snapshots used outside
// the lock; testdata/tsan_maps) under ThreadSanitizer, in both Map forms
// (XO_MAP_LOG=0, the lists; 1, decision 0291's hybrid; branch, its
// branching prototype, where the 1,600-entry recs Map forks its lists
// under the snapshots). The log form once
// stored new entries into chunks already marked shared, so other threads
// counted their strings without atomics and the heap broke under
// concurrent HTTP load (2026-10-21,
// gotchas/map-log-store-into-shared-chunk.md); serial goldens never saw it.
func TestTSanSharedMaps(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with ThreadSanitizer")
	}
	if runtime.GOOS != "darwin" || !xotool.Arm64Host() {
		t.Skip("xo build --race links here on darwin/arm64 (decision 0273 part 3)")
	}
	if why := sanitizerRuns("-fsanitize=thread"); why != "" {
		t.Skipf("ThreadSanitizer does not run here: %s", why)
	}
	var env []string
	if os.Getenv("XO_RACE_RUNTIME") == "" {
		cl, err := xotool.Clang()
		if err != nil {
			t.Skip("clang not found")
		}
		out, err := exec.Command(cl, "-print-file-name=lib/darwin/libclang_rt.tsan_osx_dynamic.dylib").Output()
		if err != nil {
			t.Skipf("no ThreadSanitizer runtime: %v", err)
		}
		env = append(env, "XO_RACE_RUNTIME="+strings.TrimSpace(string(out)))
	}
	for _, mode := range []string{"0", "1", "branch"} {
		t.Run("map_log="+mode, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "prog")
			r, err := xotool.Build([]string{filepath.Join("testdata", "tsan_maps")}, xotool.Options{Backend: xotool.BackendArm64, Race: true, Out: bin, Env: append(env, "XO_MAP_LOG="+mode)})
			if err != nil {
				t.Fatal(err)
			}
			if r.Failed {
				t.Fatalf("xo build --race failed:\n%s", r.Output)
			}
			cmd := testtmp.Guard(exec.Command(bin))
			cmd.Env = append(os.Environ(), tsanOptions())
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			testtmp.Lifeline(t, cmd, 2*time.Minute)
			err = cmd.Run()
			if strings.Contains(stderr.String(), "ThreadSanitizer") || err != nil {
				t.Fatalf("exit %v; stdout %q:\n%s", err, stdout.String(), stderr.String())
			}
			if got := strings.TrimSpace(stdout.String()); got != "done 1200 1200 8 8" {
				t.Fatalf("stdout %q, want done 1200 1200 8 8", got)
			}
		})
	}
}
