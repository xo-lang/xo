package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRtProtoMarked: every XoEq, XoHash and XoGenAny method of the
// runtime's generic types in std/rt is marked `//xo:proto BIT TYPE` with
// the right bit and type, so the Go backend can leave it out of a program
// no root of that protocol reaches (decision 0551). An unmarked one is
// kept in every program, and a wrong mark leaves out a method a root
// needs. Box is not marked: the generator boxes a recursive field, which
// the protocol walk sees as the field's own type.
func TestRtProtoMarked(t *testing.T) {
	dir := filepath.Join("..", "..", "std", "rt")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	method := regexp.MustCompile(`^func \(\w* ?\*?(List|Set|Map|Option|Result|Secret|Box|Tuple\d+)\[[^]]*\]\) (XoEq|XoHash|XoGenAny)\(`)
	bits := map[string]string{"XoEq": "eq", "XoHash": "hash", "XoGenAny": "gen"}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i, l := range lines {
			m := method.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			prev := ""
			if i > 0 {
				prev = lines[i-1]
			}
			if m[1] == "Box" {
				if strings.HasPrefix(prev, "//xo:proto ") {
					t.Errorf("%s:%d: Box's %s is marked; Box keeps every method (decision 0551)", filepath.Base(f), i+1, m[2])
				}
				continue
			}
			ty := m[1]
			if strings.HasPrefix(ty, "Tuple") {
				ty = "Tuple"
			}
			want := "//xo:proto " + bits[m[2]] + " " + ty
			if prev != want {
				t.Errorf("%s:%d: %s.%s needs the line %q before it (decision 0551), found %q", filepath.Base(f), i+1, m[1], m[2], want, prev)
			}
			n++
		}
	}
	if n == 0 {
		t.Fatal("no protocol methods of rt's generic types found in std/rt")
	}
}
