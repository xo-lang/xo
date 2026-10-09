package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func apiOf(t *testing.T, files map[string]string) []Item {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	items, err := ModuleAPI(dir, "example.com/m")
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestDiff(t *testing.T) {
	old := apiOf(t, map[string]string{
		"a/a.xo":      "pub fn keep(n: Int) -> Int { n }\npub fn gone() {}\npub fn grows() -> Int { 1 }\npub fn shrinks() -> Int uses (clock, env) { 1 }\npub fn retyped(n: Int) -> Int { n }\nfn private() {}\npub struct P { pub x: Int }\npub fn (p: P) get() -> Int uses fs.read { p.x }\n",
		"a/a_test.xo": "pub fn helper() {}\n",
		"sub/xo.mod":  "module example.com/sub\n",
		"sub/s.xo":    "pub fn other() {}\n",
	})
	if len(old) != 7 {
		t.Fatalf("old API: %+v", old)
	}
	new := apiOf(t, map[string]string{
		"a/a.xo": "pub fn keep(n: Int) -> Int { n + 1 }\npub fn grows() -> Int uses net { 1 }\npub fn shrinks() -> Int uses clock { 1 }\npub fn retyped(n: Str) -> Int { 1 }\npub fn added() {}\npub struct P { pub x: Int }\npub fn (p: P) get() -> Int uses fs { p.x }\n",
	})
	var got []string
	for _, c := range Diff(old, new) {
		s := c.Kind + " " + c.Name
		if c.Breaking {
			s += "!"
		}
		got = append(got, s)
	}
	want := "effects_added P.get!,added added,removed gone!,effects_added grows!,changed retyped!,effects_removed shrinks"
	if strings.Join(got, ",") != want {
		t.Errorf("diff:\n got %s\nwant %s", strings.Join(got, ","), want)
	}
	if b := Bump(Diff(old, new)); b != "major" {
		t.Errorf("bump %s", b)
	}
	if b := Bump(Diff(old, old)); b != "patch" {
		t.Errorf("bump of no change %s", b)
	}
}

func TestNext(t *testing.T) {
	for _, tc := range [][3]string{{"v1.2.3", "patch", "v1.2.4"}, {"v1.2.3", "minor", "v1.3.0"}, {"v1.2.3", "major", "v2.0.0"}, {"v1.3.0-rc.1", "patch", "v1.3.0"}, {"", "minor", "v0.1.0"}} {
		if got := next(tc[0], tc[1]); got != tc[2] {
			t.Errorf("next(%s, %s) = %s, want %s", tc[0], tc[1], got, tc[2])
		}
	}
	if !allowsBreaking("v0.3.1", "v0.4.0") || allowsBreaking("v0.3.1", "v0.3.2") || allowsBreaking("v1.3.1", "v1.4.0") || !allowsBreaking("v1.3.1", "v2.0.0") {
		t.Errorf("allowsBreaking")
	}
}
