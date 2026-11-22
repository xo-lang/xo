package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseVsGoRunAlone checks that scripts/release-vs-go.sh's run
// section builds the pairs it times when the compile section did not
// (task #645, bug fix): SECTIONS=run alone on host 1 found no binaries and
// every --ab failed "executable file not found". Fake xo and go record
// whether both sides of each --ab exist.
func TestReleaseVsGoRunAlone(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// build -o X: makes X; bench --ab ... A B -- W: says whether A and B run
	fakeXo := `#!/bin/bash
case "$1" in
version) echo 'xo 0.0.0-fake' ;;
build) while [ $# -gt 0 ]; do [ "$1" = -o ] && { printf '#!/bin/sh\n' >"$2"; chmod +x "$2"; }; shift; done ;;
bench) a= b=; for x in "$@"; do case "$x" in --*|bench) ;; *) if [ -z "$a" ]; then a=$x; elif [ -z "$b" ]; then b=$x; fi ;; esac; done
	if [ -x "$a" ] && [ -x "$b" ]; then echo "fake-ab ok"; else echo "fake-ab missing $a $b"; fi ;;
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
	cmd.Env = append(envNoSlot(), "PATH="+bin+":"+os.Getenv("PATH"), "SECTIONS=run", "WORKLOADS=fib putget spawn", "MAX_LOAD=1000", "ROUNDS=1")
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release-vs-go.sh: %v\n%s", err, o)
	}
	rep, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(rep), "fake-ab ok"); n != 3 || strings.Contains(string(rep), "fake-ab missing") {
		t.Errorf("want 3 runs with both sides built, got %d:\n%s", n, rep)
	}
}
