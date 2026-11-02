package llvm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// raceStdGoldens are goldens whose --race build (decision 0273 mode 2:
// the C library's heap, so ThreadSanitizer sees every free) showed a
// heap-use-after-free in std, each a raw address used after its value's
// last use (gotchas/raw-address-outlives-bytes.md), found by the
// 2026-11-02 race stress. A non-race build reads the freed bytes
// correctly until the block is reused, so only this build shows them.
var raceStdGoldens = []struct{ golden, site string }{
	// an h2 response's content-length string, freed before field copied it
	{"http_h2c_sniff", "h2w.field"},
	// a client frame's mask bytes, freed before they were read
	{"websocket", "WsConn.write_frame"},
}

// TestRaceStdNoUseAfterFree builds each of raceStdGoldens with --race on
// the arm64 backend and runs it once: no heap-use-after-free report. The
// runtime's known false reports (atomics on a record set up by another
// context, gotchas/tsan-fiber-switch-sync.md) are not checked here.
func TestRaceStdNoUseAfterFree(t *testing.T) {
	if runtime.GOOS != "linux" {
		// 2026-11-02: on darwin the h2w build passed without its fix too
		t.Skip("the use after free shows only in the Linux --race build (its heap is glibc's, which ThreadSanitizer intercepts)")
	}
	for _, g := range raceStdGoldens {
		g := g
		t.Run(g.golden, func(t *testing.T) {
			bin := raceBuild(t, filepath.Join("golden", g.golden), xotool.BackendArm64)
			dir := filepath.Join("testdata", "golden", g.golden)
			cmd := testtmp.Guard(exec.Command(bin))
			cmd.Dir = dir
			cmd.Env = append(append(os.Environ(), progEnv(dir)...), "TSAN_OPTIONS=halt_on_error=0:atexit_sleep_ms=0")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			_ = cmd.Run()
			if text := stderr.String(); strings.Contains(text, "heap-use-after-free") {
				if len(text) > 6000 {
					text = text[:6000]
				}
				t.Fatalf("a use after free (was in %s):\n%s", g.site, text)
			}
		})
	}
}
