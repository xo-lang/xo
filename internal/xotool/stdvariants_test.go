package xotool

import (
	"os/exec"
	"testing"
)

// TestStdVariantsCacheOff: with the backend cache off xo keeps no std
// variant (`xo dev stdcache` exits 1 by design), so stdVariants must not
// run it and fail every later build (2026-11-08: gate.sh's
// XO_BACKEND_CACHE=off failed every internal/arm64 build). The xo here is
// `false`, which exits 1 as xo does: run, it would be the error.
func TestStdVariantsCacheOff(t *testing.T) {
	f, err := exec.LookPath("false")
	if err != nil {
		t.Skip("no false command")
	}
	t.Setenv("XO_TEST_SEPCOMP", "")
	for _, env := range [][2]string{{"XO_BACKEND_CACHE", "off"}, {"XO_DUMP_FN", "main"}} {
		t.Run(env[0], func(t *testing.T) {
			t.Setenv("XO_BACKEND_CACHE", "")
			t.Setenv("XO_DUMP_FN", "")
			t.Setenv(env[0], env[1])
			if err := stdVariants([]string{f}); err != nil {
				t.Fatalf("%s=%s: %v", env[0], env[1], err)
			}
		})
	}
}
