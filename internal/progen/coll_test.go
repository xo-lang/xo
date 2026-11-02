package progen

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestCollDump writes programs FIRST..FIRST+COUNT-1 of Coll as module
// directories under XO_COLL_DUMP (skipped without it): the input of the
// differential runner (scripts/coll-diff.sh).
func TestCollDump(t *testing.T) {
	gen := Coll
	if os.Getenv("XO_COLL_MODE") == "alias" {
		gen = Alias
	}
	dir := os.Getenv("XO_COLL_DUMP")
	if dir == "" {
		t.Skip("set XO_COLL_DUMP=DIR [XO_COLL_FIRST=1 XO_COLL_COUNT=100]")
	}
	first, count := int64(1), 100
	if v := os.Getenv("XO_COLL_FIRST"); v != "" {
		first, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := os.Getenv("XO_COLL_COUNT"); v != "" {
		count, _ = strconv.Atoi(v)
	}
	for i := 0; i < count; i++ {
		seed := first + int64(i)
		d := filepath.Join(dir, fmt.Sprint(seed))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "xo.mod"), []byte(fmt.Sprintf("module example.com/coll%d\n", seed)), 0o644)
		os.WriteFile(filepath.Join(d, "main.xo"), []byte(gen(seed)), 0o644)
	}
}
