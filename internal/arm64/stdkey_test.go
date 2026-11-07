package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdVariantKeyTouched: std's separately compiled variant keys on
// its files' contents, so touching every file with its bytes unchanged
// (a fresh checkout, a copy, an editor's save) finds the variant xo dev
// stdcache built before instead of building it again. The key held each
// file's modification time until 2026-11-07, when a tier's
// TestSepcompDefault missed the variant built seconds before.
func TestStdVariantKeyTouched(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's variant")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdkey-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	std := filepath.Join(work, "std")
	copyTreeTimes(t, filepath.Join(root, "std"), std)
	stdcache := func() (string, string) {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
		cmd.Env = append(os.Environ(), "XO_STD="+std, "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"))
		var errb strings.Builder
		cmd.Stderr = &errb
		out, err := stdRun(cmd.Output)
		if err != nil {
			t.Fatalf("xo dev stdcache: %v\n%s", err, errb.String())
		}
		return strings.TrimSpace(string(out)), errb.String()
	}
	first, _ := stdcache()
	now := time.Now()
	err = filepath.WalkDir(std, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(p, now, now)
	})
	if err != nil {
		t.Fatal(err)
	}
	again, msgs := stdcache()
	if again != first {
		t.Fatalf("after a touch of std's files the variant is another:\nbefore %s\nafter  %s\n%s", first, again, msgs)
	}
	if strings.Contains(msgs, " built in ") {
		t.Fatalf("after a touch of std's files the variant was built again:\n%s", msgs)
	}
}
