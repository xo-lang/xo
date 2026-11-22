package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rvgFakes writes fake xo and go into DIR/bin: build -o X makes X, and
// bench --ab prints "fake-ab ARGS" with the arguments after "--".
func rvgFakes(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeXo := `#!/bin/bash
case "$1" in
version) echo 'xo 0.0.0-fake' ;;
build) while [ $# -gt 0 ]; do [ "$1" = -o ] && { printf '#!/bin/sh\n' >"$2"; chmod +x "$2"; }; shift; done ;;
bench) while [ $# -gt 0 ] && [ "$1" != -- ]; do shift; done; shift; echo "fake-ab $*" ;;
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
	return bin
}

// envNoSlot is os.Environ() without heavy.sh's XO_HEAVY_SLOT, which the
// run section refuses unless it is measure (task #686): a test run under
// a gate would otherwise see the refusal.
func envNoSlot() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "XO_HEAVY_SLOT=") {
			env = append(env, e)
		}
	}
	return env
}

func rvgRun(t *testing.T, tmp, bin string, env ...string) (string, []byte, error) {
	out := filepath.Join(tmp, "out")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release-vs-go.sh"), filepath.Join(bin, "xo"), out)
	cmd.Env = append(envNoSlot(), "PATH="+bin+":"+os.Getenv("PATH"), "SECTIONS=run", "WORKLOADS=fib putget branch spawn", "MAX_LOAD=1000", "ROUNDS=1")
	cmd.Env = append(cmd.Env, env...)
	o, err := cmd.CombinedOutput()
	return out, o, err
}

// TestReleaseVsGoRepeat checks that the run section passes each workload a
// repeat count (task #686, owner 2026-11-22: "run them longer so we can
// compare them"): 0.55.0's linux/arm64 report had 12 of 20 rows "too short"
// for a wall verdict. RVG_REPEAT=N gives every row N.
func TestReleaseVsGoRepeat(t *testing.T) {
	for _, tc := range []struct {
		env  []string
		want map[string]string
	}{
		{nil, map[string]string{"fib": "5", "putget": "23", "branch": "90", "spawn": "58"}},
		{[]string{"RVG_REPEAT=1"}, map[string]string{"fib": "1", "putget": "1", "branch": "1", "spawn": "1"}},
	} {
		tmp := t.TempDir()
		bin := rvgFakes(t, tmp)
		out, o, err := rvgRun(t, tmp, bin, tc.env...)
		if err != nil {
			t.Fatalf("%v: release-vs-go.sh: %v\n%s", tc.env, err, o)
		}
		rep, err := os.ReadFile(filepath.Join(out, "report.md"))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, m := range regexp.MustCompile(`(?m)^fake-ab (\w+) ?(\S*)$`).FindAllStringSubmatch(string(rep), -1) {
			got[m[1]] = m[2]
		}
		for w, n := range tc.want {
			if got[w] != n {
				t.Errorf("%v: %s ran with repeat %q, want %q\n%s", tc.env, w, got[w], n, rep)
			}
			if !strings.Contains(string(rep), " "+w+", repeat "+n+" (load ") {
				t.Errorf("%v: no heading naming %s's repeat %s", tc.env, w, n)
			}
		}
	}
}

// TestReleaseVsGoRunRefusesSlot checks that the run section refuses to
// start under a heavy.sh gate or slot (task #686, bug fix): 0.55.0's Mac
// report came from a run under heavy.2, where xo bench --ab refuses, and
// all 20 run rows read "(failed)". A measure job still runs.
func TestReleaseVsGoRunRefusesSlot(t *testing.T) {
	tmp := t.TempDir()
	bin := rvgFakes(t, tmp)
	_, o, err := rvgRun(t, tmp, bin, "XO_HEAVY_SLOT=heavy.2")
	if err == nil || !strings.Contains(string(o), "needs its own HEAVY_CLASS=measure job") {
		t.Fatalf("under heavy.2: want a refusal, got err %v\n%s", err, o)
	}
	tmp = t.TempDir()
	bin = rvgFakes(t, tmp)
	if _, o, err := rvgRun(t, tmp, bin, "XO_HEAVY_SLOT=measure"); err != nil {
		t.Fatalf("under measure: %v\n%s", err, o)
	}
}
