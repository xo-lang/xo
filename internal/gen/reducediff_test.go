package gen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/gotool"
	"github.com/xo-lang/xo/internal/xotool"
)

// The tests of scripts/reduce-diff.py, the reducer of backend
// differences. A backend named `bad` stands for a miscompile: its builds
// print one more line when the program has an xor (` ^ `), so the
// reducer must keep an xor and drop everything else it can.

// realXo wraps a real xo: `bad` builds with the Go backend, and the
// binary it writes runs that build and then prints the extra line.
const realXo = `#!/bin/sh
shift
be=go
while [ $# -gt 0 ]; do
	case $1 in
	--backend=*) be=${1#--backend=} ;;
	-o) out=$2; shift ;;
	esac
	shift
done
if [ "$be" != bad ]; then exec "$XO_REAL" build -o "$out" .; fi
"$XO_REAL" build -o "$out.real" . || exit 1
extra=
if grep -q ' ^ ' main.xo; then extra='echo xor'; fi
printf '#!/bin/sh\n"%s"\ns=$?\n%s\nexit $s\n' "$out.real" "$extra" >"$out"
chmod +x "$out"
`

func reduceDiff(t *testing.T, xo, in string, env ...string) string {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	script, err := filepath.Abs("../../scripts/reduce-diff.py")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "min.xo")
	cmd := exec.Command(py, "-I", script, "-q", xo, "go", "bad", in, out)
	cmd.Env = append(os.Environ(), env...)
	start := time.Now()
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reduce-diff: %v\n%s", err, b)
	}
	t.Logf("reduced in %v", time.Since(start).Round(time.Millisecond))
	min, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(min) != string(b) && string(min)+"\n" != string(b) {
		t.Errorf("stdout is not the result written:\n%s\n--- written\n%s", b, min)
	}
	return string(min)
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestReduceDiffFake runs the reducer's passes on a p197 style program
// with builds that cost nothing (testdata/reduce/fakexo). Also under
// -short (about 2 s).
func TestReduceDiffFake(t *testing.T) {
	xo := filepath.Join(t.TempDir(), "fakexo")
	gob := exec.Command("go", "build", "-o", xo, "./testdata/reduce/fakexo")
	gob.Env = goVetEnv()
	release := gotool.HoldCache()
	out, err := gob.CombinedOutput()
	release()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	min := reduceDiff(t, xo, "testdata/reduce/p197.xo")
	lines := strings.Split(strings.TrimRight(min, "\n"), "\n")
	if !strings.Contains(min, " ^ ") || !strings.Contains(min, "fn main") || len(lines) > 4 {
		t.Errorf("not reduced to a main and an xor:\n%s", min)
	}
}

// TestReduceDiffReal reduces the same program with real builds (the Go
// backend, and `bad`, the Go backend plus a line): the result must
// still check, keep its xor, and be much smaller. Skipped with -short.
func TestReduceDiffReal(t *testing.T) {
	if testing.Short() {
		t.Skip("about a minute of builds; run without -short")
	}
	// the xo the tests run (internal/xotool: $XO_CLI, else the
	// checkout's bin/xo)
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	min := reduceDiff(t, writeScript(t, "xo-wrap", realXo), "testdata/reduce/p197.xo", "XO_REAL="+xo)
	if !strings.Contains(min, " ^ ") || strings.Count(min, "\n") > 12 {
		t.Errorf("not reduced to a small program with an xor:\n%s", min)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/min\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.xo"), []byte(min), 0o644)
	ds, err := xotool.Check([]string{dir})
	if err != nil || len(ds) > 0 {
		t.Errorf("the result does not check: %v %v\n%s", err, ds, min)
	}
}
