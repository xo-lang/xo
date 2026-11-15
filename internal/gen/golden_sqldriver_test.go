package gen

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sqlOpenMysql matches a `sql.open(net, "mysql", ...)` call.
var sqlOpenMysql = regexp.MustCompile(`sql\.open\([^,]*,\s*"mysql"`)

// TestGoldenSQLDriverParity keeps the goldens here, which run on the Go
// backend and natively (TestGenGoldensNative) against one want.txt, off
// the "mysql" driver: native builds have std/sql's Xo driver and the Go
// backend has none (decision 0648 item 13), so the two print different
// errors. A golden that wants an unknown driver names one no backend has.
func TestGoldenSQLDriverParity(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "golden", "*", "*.xo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no goldens found")
	}
	for _, f := range files {
		// a `use go` golden run leaves its binding cache, a `.xo`
		// directory, in the golden (gotcha go-bind-cache-in-source-tree),
		// and Glob's * matches a leading dot: regular source files only
		if strings.HasPrefix(filepath.Base(f), ".") {
			continue
		}
		if fi, err := os.Lstat(f); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := sqlOpenMysql.FindIndex(src); loc != nil {
			t.Errorf("%s opens the \"mysql\" driver, which only native builds have: use a driver name no backend has (decision 0648 item 13)", f)
		}
	}
}
