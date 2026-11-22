package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseVsGoHeapProfileDies checks that a profiled build that dies in
// scripts/release-vs-go.sh's warm-memory notes is reported in the report and
// the run goes on (task #645, bug fix): xo 0.55.0's heap profile dies of
// SIGSEGV (task #658), and under set -e that ended the Mac run before its
// run and test sections. A fake xo kills itself with SIGSEGV when XO_PROF is
// set; RVG_PART=1/4 is the part that holds the notes (TestReleaseVsGoParts).
func TestReleaseVsGoHeapProfileDies(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeXo := `#!/bin/bash
case "$1" in
version) echo 'xo 0.0.0-fake' ;;
prof) exit 0 ;;
build) [ -n "${XO_PROF:-}" ] && kill -SEGV $$
	while [ $# -gt 0 ]; do [ "$1" = -o ] && { printf '#!/bin/sh\n' >"$2"; chmod +x "$2"; }; shift; done ;;
bench) echo "fake-ab" ;;
esac
`
	fakeGo := `#!/bin/bash
case "$1" in
env) echo go0.0-fake ;;
version) echo 'go version go0.0-fake' ;;
build) while [ $# -gt 0 ]; do [ "$1" = -o ] && { printf '#!/bin/sh\n' >"$2"; chmod +x "$2"; }; shift; done ;;
esac
`
	for name, body := range map[string]string{"xo": fakeXo, "go": fakeGo} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(tmp, "out")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release-vs-go.sh"), filepath.Join(bin, "xo"), out)
	cmd.Env = append(envNoSlot(), "PATH="+bin+":"+os.Getenv("PATH"), "SECTIONS=compile run", "RVG_PART=1/4", "WORKLOADS=fib", "MAX_LOAD=1000", "ROUNDS=1")
	o, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release-vs-go.sh: %v\n%s", err, o)
	}
	rep, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"(no heap profile: the profiled build exited 139)", "## Run (", "fake-ab"} {
		if !strings.Contains(string(rep), want) {
			t.Errorf("report lacks %q:\n%s", want, rep)
		}
	}
}
