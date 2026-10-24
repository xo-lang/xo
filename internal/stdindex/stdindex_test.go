package stdindex

import (
	"strings"
	"testing"
)

func TestIndex(t *testing.T) {
	src := "struct Str {}\n" +
		"interface Eq {}\n" +
		"interface Display {\n  /// doc\n  fn display(self) -> Str\n}\n" +
		"fn (s: Str) len() -> Int\n" +
		"fn (var b: StrBuilder) write(s: Str)\n" +
		"fn fault(msg: Str) -> Never\n" +
		"fn (s: Str) find(sub: Str) -> Int?\n"
	got := Index(src)
	for _, want := range []string{
		"5 declarations.",
		"### Display\n\n```\nfn display(self) -> Str\n```",
		"### Str\n\n```\nfn (s: Str) len() -> Int\nfn (s: Str) find(sub: Str) -> Int?\n```",
		"### StrBuilder\n\n```\nfn (var b: StrBuilder) write(s: Str)\n```",
		"### Functions\n\n```\nfn fault(msg: Str) -> Never\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Index lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "### Eq") {
		t.Errorf("an empty interface has a section:\n%s", got)
	}
	spec := "# std\n\ntext\n"
	once := Splice(spec, got)
	if Current(once) != got || Splice(once, got) != once {
		t.Errorf("Splice is not idempotent:\n%s", once)
	}
}

// TestSpliceMarkerReworded: a spec whose Begin line still names an older
// command holds one block, and splicing replaces it rather than appending a
// second one (the marker named scripts/stdindex until 2026-10-24).
func TestSpliceMarkerReworded(t *testing.T) {
	index := Index("fn (s: Str) len() -> Int\n")
	spec := "# std\n\n<!-- BEGIN prelude index: generated from std/prelude.xo by `go run ./scripts/stdindex`; do not edit -->\n\nold text\n\n" + End + "\n\nprose after\n"
	if !strings.Contains(Begin, "scripts/stdsync") {
		t.Fatal("the marker does not name scripts/stdsync")
	}
	if Current(spec) == "" {
		t.Fatal("Current does not find a block with the older marker")
	}
	once := Splice(spec, index)
	if got := strings.Count(once, End); got != 1 {
		t.Fatalf("Splice left %d blocks:\n%s", got, once)
	}
	if Current(once) != index {
		t.Errorf("the block is not the index:\n%s", once)
	}
	if !strings.HasSuffix(once, "prose after\n") {
		t.Errorf("Splice lost the text after the block:\n%s", once)
	}
}
