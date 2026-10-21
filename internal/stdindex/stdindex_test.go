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
