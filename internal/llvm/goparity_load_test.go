package llvm

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// goParityLoadGoldens are the goldens whose Go backend output depended on
// timing (task #151): net/http keeps a connection only when its writer
// reports the request written within 50 ms of the response
// (maxWriteWaitBeforeConnReuse), and its writer goroutine records
// WroteHeaders and WroteRequest after the write, so a starved CPU dropped
// a connection the golden expected reused (http_max_idle: "at most 8
// connections: false") or sent before those events (http_client_trace,
// mostly "https: reused"). They failed in the 6 h tier on host 4 and
// passed alone; with every CPU busy http_client_trace differed in 5 of 20
// runs on host 1 before the fix (2026-11-16), 0 of 30 after.
var goParityLoadGoldens = []string{"http_client_trace", "http_max_idle"}

// TestGoParityUnderLoad runs goParityLoadGoldens on the Go backend
// XO_TEST_GO_PARITY_LOAD=N times each while busy goroutines hold every
// CPU, and requires want.txt each time. Skipped by default: it is the
// load reproduction of task #151, run on an offload host.
func TestGoParityUnderLoad(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("XO_TEST_GO_PARITY_LOAD"))
	if n <= 0 {
		t.Skip("XO_TEST_GO_PARITY_LOAD=N runs the pool-timing goldens N times under CPU load (task #151)")
	}
	var stop atomic.Bool
	defer stop.Store(true)
	sink := make(chan int, 1)
	for i := 0; i < 2*runtime.NumCPU(); i++ {
		go func() {
			x := 0
			for !stop.Load() {
				for j := 0; j < 1<<16; j++ {
					x += j
				}
			}
			select {
			case sink <- x:
			default:
			}
		}()
	}
	for _, name := range goParityLoadGoldens {
		dir := filepath.Join("testdata", "golden", name)
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		fails := 0
		for i := 0; i < n; i++ {
			res := buildAndRun(t, dir, xotool.Options{Backend: xotool.BackendGo})
			if string(want) != res.String() {
				fails++
				if fails <= 2 {
					t.Errorf("%s run %d of %d: the Go backend differs from want.txt\n--- want\n%s\n--- got\n%s", name, i+1, n, want, res)
				}
			}
		}
		if fails > 0 {
			t.Errorf("%s: %d of %d runs differ under load", name, fails, n)
		}
	}
}
