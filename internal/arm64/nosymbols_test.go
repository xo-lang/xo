package arm64

import (
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestTracebackNoSymbols: with -s (or a strip after the link) no function
// symbol is left to name a frame: the traceback says so instead of
// printing the image's header symbol for every frame, and no longer runs
// past main (2026-10-25).
func TestTracebackNoSymbols(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	for _, be := range tracebackBackends(t) {
		t.Run(be, func(t *testing.T) {
			res, err := xotool.Build([]string{filepath.Join("testdata", "traceback")}, xotool.Options{Backend: be, Flags: []string{"-s"}})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			got := runFault(t, res.Binaries[xotool.Host()])
			const report = "fault: index 7 out of range for List of length 3 at main.xo:5:12\n"
			const want = report + "traceback: (no function symbols: built with -s, or stripped)\n"
			if got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
