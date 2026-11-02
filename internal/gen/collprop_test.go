package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/progen"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestCollChecks holds progen.Coll to its promise: every program checks
// without a diagnostic (one xo check of 100 modules). The differential
// run is scripts/coll-diff.sh.
func TestCollChecks(t *testing.T) {
	t.Parallel() // temporary directories only
	root := t.TempDir()
	var dirs []string
	for seed := int64(1); seed <= 100; seed++ {
		d := filepath.Join(root, fmt.Sprint(seed))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "xo.mod"), []byte(fmt.Sprintf("module example.com/coll%d\n", seed)), 0o644)
		os.WriteFile(filepath.Join(d, "main.xo"), []byte(progen.Coll(seed)), 0o644)
		dirs = append(dirs, d)
	}
	ds, err := xotool.Check(dirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) > 0 {
		t.Errorf("a Coll program does not check:\n%s", diagText(&xotool.Result{Diagnostics: ds}))
	}
}
