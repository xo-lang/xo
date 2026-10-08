package gen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// jsonFuncRE matches one generated JSON codec method, through its closing
// brace at the start of a line.
var jsonFuncRE = regexp.MustCompile(`(?ms)^func \([^)]*\) XoJson(Enc|Read)\(.*?^}\n`)

// TestJSONDirectCodegen pins the direct `derive Json` codec
// (internal/gen/json.go): precomputed keys and typed writers in XoJsonEnc,
// the key switch of XoJsonRead. Run with -golden-update to rewrite the golden
// file.
func TestJSONDirectCodegen(t *testing.T) {
	src := `use std/json

struct User {
  id: Int
  name: Str
  admin: Bool
  nickname: Str? = None
  tags: List[Str] = []
  score: Float = 0.5
} derive Json

struct Edit {
  name: json.Patch[Str]
  id: Int
} derive Json

struct Empty {} derive Json

fn main(os: Os) {}
`
	got := strings.Join(jsonFuncRE.FindAllString(generateArenas(t, src, false), -1), "\n")
	got = regexp.MustCompile(`(?m)^//line .*\n`).ReplaceAllString(got, "")
	path := filepath.Join("testdata", "json_direct.golden")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("generated codec differs from %s (run with -golden-update):\n%s", path, got)
	}
}
