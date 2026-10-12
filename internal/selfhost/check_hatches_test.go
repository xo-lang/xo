package selfhost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	text := runXo(t, bin, tmp, append([]string{"--each", "--std=" + stdDir, "--wd=" + workDir(t)}, dirs...))
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
			for _, l := range wants[d].staged {
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
