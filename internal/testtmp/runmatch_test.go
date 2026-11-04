package testtmp

import (
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	RequireRunMatch(m)
	os.Exit(m.Run())
}

// TestRequireRunMatch (decision 0437): this test binary run with a -run
// that names no test exits 1 and says so; with one that names a test,
// or ^$, it passes.
func TestRequireRunMatch(t *testing.T) {
	if os.Getenv("XO_RUNMATCH_CHILD") == "1" {
		return
	}
	for _, c := range []struct {
		run  string
		skip string
		code int
	}{{"^TestNoSuchTestAnywhere$", "", 1}, {"^TestRequireRunMatch$", "", 0}, {"^$", "", 0}, {"TestRequireRunMatch/sub", "", 0},
		// -skip of a subtest leaves its test running (a TSan round with
		// -skip TestGoldenTasks/debug_routes ran nothing, 2026-11-04)
		{"^TestRequireRunMatch$", "TestRequireRunMatch/sub", 0}, {"^TestRequireRunMatch$", "^TestRequireRunMatch$", 1}} {
		args := []string{"-test.run=" + c.run}
		if c.skip != "" {
			args = append(args, "-test.skip="+c.skip)
		}
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "XO_RUNMATCH_CHILD=1")
		out, _ := cmd.CombinedOutput()
		code := cmd.ProcessState.ExitCode()
		if code != c.code || (c.code == 1 && !strings.Contains(string(out), "matches no test")) {
			t.Errorf("-run=%s -skip=%s: exit %d, want %d\n%s", c.run, c.skip, code, c.code, out)
		}
	}
}
