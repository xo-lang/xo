package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// TestTuplesGenCurrent: std/rt/tuples_gen.go is what scripts/gentuples
// writes, and compiler/gen's GO_TUPLE_MAX (the widest tuple the Go
// backend accepts) equals the generator's Max, so a tuple the generator
// declares is never reported as too wide, and one it does not is never
// emitted (2026-10-21: a 17-tuple failed go build with "undefined:
// rt.Tuple17").
func TestTuplesGenCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	maxOf := func(p, pattern string) string {
		t.Helper()
		m := regexp.MustCompile(pattern).FindStringSubmatch(read(p))
		if m == nil {
			t.Fatalf("%s: no %s", p, pattern)
		}
		return m[1]
	}
	genMax := maxOf("scripts/gentuples/main.go", `(?m)^const Max = (\d+)$`)
	goMax := maxOf("compiler/gen/gen.xo", `(?m)^const GO_TUPLE_MAX = (\d+)$`)
	if genMax != goMax {
		t.Errorf("scripts/gentuples Max %s, compiler/gen GO_TUPLE_MAX %s: keep them equal", genMax, goMax)
	}
	src := read("std/rt/tuples_gen.go")
	if !regexp.MustCompile(`(?m)^type Tuple` + genMax + `\[`).MatchString(src) {
		t.Errorf("std/rt/tuples_gen.go has no Tuple%s: go run ./scripts/gentuples", genMax)
	}
	if testing.Short() {
		return
	}
	cmd := exec.Command("go", "run", "./scripts/gentuples", "-check")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("%v: %s", err, out)
	}
}
