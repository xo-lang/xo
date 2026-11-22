package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// TestReleaseVsGoRmw checks scripts/release-vs-go.sh's rmw (task #645): a
// directory a background std build still writes in for a few seconds
// after the timed build (a detached `xo dev stdcache`) is removed once the
// writer stops, and rmw returns 0. A plain rm -rf there ended the compile
// section's part 3 with 1 (host 1, 2026-11-21: "Directory not empty").
func TestReleaseVsGoRmw(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	src, err := os.ReadFile(filepath.Join(root, "scripts", "release-vs-go.sh"))
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?ms)^rmw\(\) \{\n.*?^\}\n`).Find(src)
	if fn == nil {
		t.Fatal("release-vs-go.sh has no rmw function")
	}
	dir := filepath.Join(t.TempDir(), "cache")
	// the writer makes files in nested directories for about 3 s, as the
	// background build's objects, then exits; rmw starts while it runs
	script := string(fn) + `
d=$1
mkdir -p "$d/a/b"
( end=$((SECONDS + 3)); i=0; while [ $SECONDS -lt $end ]; do mkdir -p "$d/a/b/$i" 2>/dev/null && : > "$d/a/b/$i/o"; i=$((i + 1)); done ) &
sleep 0.2
rmw "$d"; rc=$?
wait
[ -e "$d" ] && { echo "still there"; exit 3; }
exit $rc
`
	out, err := exec.Command("bash", "-c", script, "bash", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("rmw: %v\n%s", err, out)
	}
}
