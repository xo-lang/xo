package selfhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/gotool"
)

// hatchesFixture is a module tree with its own module cache: a dependency
// (example.com/gox) with `use go` escape hatches, and two main modules
// that require it, one without the vouch for its version, with a
// malformed vouch, a narrower effect lock, an old edition name, and calls
// of names the Go binding skipped (hatch), and one with the vouch and the
// effect lock the API needs (hatchok). moduleDirs leaves it out: the Go
// checker of a run of the cached dependency itself would write its `use
// go` bindings into the cached tree and change its content hash.
const hatchesFixture = "internal/selfhost/testdata/hatches"

// TestSelfhostCheckerHatches compares the Xo and Go checkers on the
// escape hatches of decision 0156 (XO1316 with a stale vouch's note,
// XO1301 for a malformed vouch, XO1317 from the effect lock), XO1315,
// and XO1102 on a copy of hatchesFixture. The Go checker runs first: it
// writes the `use go` bindings the Xo checker reads.
func TestSelfhostCheckerHatches(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("binds Go packages with go list")
	}
	root := repoRoot(t)
	bin, err := selfhostBinary("check", "go")
	if err != nil {
		t.Fatal(err)
	}
	stdDir, err := filepath.Abs(filepath.Join(root, "std"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	copyDir(t, filepath.Join(root, hatchesFixture), tmp)
	t.Chdir(tmp)
	t.Setenv("XO_MODCACHE", filepath.Join(tmp, "cache"))
	t.Setenv("XO_OFFLINE", "1")
	t.Setenv("XO_WORK", "")
	dirs := []string{"hatch", "hatchok"}
	wants := map[string]*goRun{}
	for _, d := range dirs {
		wants[d] = goCheck(d)
	}
	skipRecording(t)
	text := runXoBinding(t, bin, tmp, append([]string{"--each", "--std=" + stdDir, "--wd=" + workDir(t)}, dirs...), dirs)
	parts := strings.Split(text, "=== check ")
	if len(parts) != len(dirs)+1 {
		t.Fatalf("Xo checker printed %d runs for %d directories", len(parts)-1, len(dirs))
	}
	var st checkStats
	codes := map[string]int{}
	for j, d := range dirs {
		got := strings.TrimPrefix(parts[j+1], d+"\n")
		before := st.stagedOK
		if msg := compareRun(d, wants[d], got, &st); msg != "" {
			t.Error(msg)
		}
		if st.stagedOK > before {
			for _, l := range wants[d].stagedLines(got) {
				codes[l[len(`{"code":"`):len(`{"code":"XO0000`)]]++
			}
		}
	}
	t.Logf("%d runs: %d accepted by Go (%d match), %d rejected only with diagnostics of the stages done (%d match), %d rejected otherwise, %d unsupported, %d not loadable; diagnostics %v",
		len(dirs), st.accepted, st.acceptedOK, st.staged, st.stagedOK, st.otherRejected, st.unsupported, st.errors, codes)
	if st.acceptedOK+st.stagedOK != len(dirs) {
		t.Errorf("want every run compared and identical, got %d of %d", st.acceptedOK+st.stagedOK, len(dirs))
	}
	if _, err := os.Stat(filepath.Join(tmp, "hatch", ".xo", "gobind")); err != nil {
		t.Errorf("the dependency's binding is not in the main module's cache: %v", err)
	}
}

// unboundRE matches the Xo checker's output for a run that lacks the
// binding of a `use go` package (selfhost/check goimport.xo, go_miss).
var unboundRE = regexp.MustCompile(`^== unsupported use go without a current binding of (\S+) in `)

// runXoBinding runs the Xo checker bin with args from dir, binding first
// what it lacks, as xo1's checker does (check_dirs_bound,
// selfhost/check/bindmiss.xo): every `use go` package a run reports
// unbound is bound into that run's module with the helper xo-gobind
// (decision 0178 stage 6, Go since it reads Go's export data), and the
// runs are checked again. The Go checker writes the same bindings as a
// side effect of its check; a replayed Go side (gogoldens_test.go)
// writes none, so replay needs this (live, the first run finds them).
func runXoBinding(t *testing.T, bin, dir string, args, runs []string) string {
	t.Helper()
	gobind := ""
	bound := map[string]bool{}
	for round := 0; ; round++ {
		text := runXo(t, bin, dir, args)
		parts := strings.Split(text, "=== check ")
		var todo [][2]string
		for j, d := range runs {
			if j+1 >= len(parts) {
				break
			}
			got := strings.TrimPrefix(parts[j+1], d+"\n")
			if m := unboundRE.FindStringSubmatch(firstLine(got)); m != nil && !bound[d+" "+m[1]] {
				bound[d+" "+m[1]] = true
				todo = append(todo, [2]string{d, m[1]})
			}
		}
		if len(todo) == 0 || round == 8 {
			return text
		}
		if gobind == "" {
			gobind = filepath.Join(t.TempDir(), "xo-gobind")
			cmd := exec.Command("go", "build", "-o", gobind, "github.com/xo-lang/xo/cmd/xo-gobind")
			cmd.Dir = pkgDir
			cmd.Env = append(os.Environ(), gotool.CacheEnv())
			release := gotool.HoldCache()
			out, err := cmd.CombinedOutput()
			release()
			if err != nil {
				t.Fatalf("building xo-gobind: %v\n%s", err, out)
			}
		}
		for _, b := range todo {
			cmd := exec.Command(gobind, "--dir", b[0], b[1])
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Logf("xo-gobind --dir %s %s: %v\n%s", b[0], b[1], err, out)
			}
		}
	}
}
