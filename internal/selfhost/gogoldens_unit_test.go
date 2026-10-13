package selfhost

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestGoldPieces requires a unit's pieces to give the same verdicts
// live and replayed from its recorded lines (written and read back),
// with names and texts holding blanks, quotes, newlines, and the
// repository root (stored as a placeholder); and a replayed family whose
// Xo side differs to name the member that differs, through the pool.
func TestGoldPieces(t *testing.T) {
	cx := newGoldenCtx()
	root := cx.root
	live := newPieces()
	live.kind = "testkind"
	live.setMeta("none", "error at "+root+"/x.xo: \"bad\"\nsecond line")
	live.addMember("f", "main", "func main\n  ret\n")
	live.addMember("f", "dup List[Int]", "helper text "+root)
	live.add("empty", "")
	live.addList("types", []string{"type b", "type a"})
	live.addSet("lits", "\"x\"", "y y")
	live.addSet("none")
	live.id, live.cx = "0123456789abcdef", cx
	lines, pool := live.recordLines()
	dir := t.TempDir()
	t.Setenv("XO_SELFHOST_GOLDENS_DIR", dir)
	if err := writeGoldKind(dir, "testkind", map[string][]goldLine{live.id: lines}); err != nil {
		t.Fatal(err)
	}
	pm := map[string]bool{}
	for _, h := range pool {
		pm[h] = true
	}
	if err := writeGoldPool(dir, "testkind", pm); err != nil {
		t.Fatal(err)
	}
	read, err := readGoldKind(dir, "testkind")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "testkind", "*.txt"))
	if len(files) < 2 || len(read[live.id]) != len(lines) {
		t.Fatalf("%d files, %d lines read of %d", len(files), len(read[live.id]), len(lines))
	}
	rep := newPieces()
	rep.kind, rep.cx = "testkind", cx
	piecesFromLines(rep, read[live.id])
	xf := map[string]string{"main": "func main\n  ret\n", "dup List[Int]": "helper text " + root}
	for _, p := range []*goldPieces{live, rep} {
		p = p.resolved("f", xf, nil).resolvedSet("lits", []string{"\"x\"", "y y"}).resolvedSet("none", nil)
		check := func(what string, got, want bool) {
			t.Helper()
			if got != want {
				t.Errorf("hashed=%v: %s: got %v, want %v", p.hashed, what, got, want)
			}
		}
		check("meta", p.m("none") == live.m("none"), true)
		check("eq main", p.eq("f:main", "func main\n  ret\n"), true)
		check("eq main other", p.eq("f:main", "func main\n"), false)
		check("eq helper", p.eq("f:dup List[Int]", "helper text "+root), true)
		check("eq empty", p.eq("empty", ""), true)
		check("eq absent", p.eq("absent", ""), true)
		check("has absent", p.has("f:absent"), false)
		check("list", p.eqList("types", []string{"type a", "type b"}), true)
		check("list other", p.eqList("types", []string{"type a"}), false)
		check("set", p.inSet("lits", "y y") && p.inSet("lits", "\"x\""), true)
		check("set other", p.inSet("lits", "z"), false)
		check("set empty", p.inSet("none", ""), false)
		check("names", strings.Join(p.names("f:"), ",") == "dup List[Int],main", true)
	}
	// the Xo side differs in main and skips the helper
	q := rep.resolved("f", map[string]string{"main": "func main\n"}, []string{"dup List[Int]"})
	if q.eq("f:main", "func main\n") || !q.has("f:dup List[Int]") || q.eq("f:dup List[Int]", "") {
		t.Errorf("differing family: %v", q.parts)
	}
	if strings.Join(q.names("f:"), ",") != "dup List[Int],main" {
		t.Errorf("differing family: names %v", q.names("f:"))
	}
	// a set with an element the Go side lacks
	s := rep.resolvedSet("lits", []string{"y y", "z"})
	if !s.inSet("lits", "y y") || s.inSet("lits", "z") {
		t.Errorf("differing set: %v", s.parts)
	}
}

// TestGoldSlot requires the units of one input recorded with different
// contents (of the file, the run's tree, std) to share a slot, and the
// units of different paths, options, or hosts not to.
func TestGoldSlot(t *testing.T) {
	cx := newGoldenCtx()
	a := cx.fileKey("x/a.xo", []byte("one"))
	b := cx.fileKey("x/a.xo", []byte("two"))
	if goldID("lexdump", []string{a}) == goldID("lexdump", []string{b}) {
		t.Fatal("different contents share a unit")
	}
	if goldSlot("lexdump", []string{a}) != goldSlot("lexdump", []string{b}) {
		t.Error("one path's contents have different slots")
	}
	if goldSlot("lexdump", []string{a}) == goldSlot("lexdump", []string{cx.fileKey("x/b.xo", []byte("one"))}) {
		t.Error("two paths share a slot")
	}
	if goldSlot("lexdump", []string{a}) == goldSlot("astdump", []string{a}) {
		t.Error("two kinds share a slot")
	}
	sum := strings.Repeat("ab", 32)
	run := func(tree, std, host string) []string {
		return []string{"tests=false", "@ROOT0@\x00examples/hello\x00" + tree + "\x00std:" + std + "\x00" + host}
	}
	other := strings.Repeat("cd", 32)
	if goldSlot("ssadump", run(sum, sum, "darwin/arm64")) != goldSlot("ssadump", run(other, other, "darwin/arm64")) {
		t.Error("one run's trees have different slots")
	}
	if goldSlot("ssadump", run(sum, sum, "darwin/arm64")) == goldSlot("ssadump", run(sum, sum, "linux/arm64")) {
		t.Error("two hosts share a slot")
	}
}
