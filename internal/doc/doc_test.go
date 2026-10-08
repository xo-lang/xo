package doc

import (
	"strings"
	"testing"
)

func TestExamples(t *testing.T) {
	d := "Intro.\n\n```\nlet a = 1\n```\n\n```json\n{}\n```\n\n```xo\nlet b = 2\nb\n```\n"
	got := Examples(d)
	if len(got) != 2 || got[0] != "let a = 1\n" || got[1] != "let b = 2\nb\n" {
		t.Errorf("Examples = %q", got)
	}
	n := normalizeFences(d)
	if strings.Count(n, "```xo") != 2 || !strings.Contains(n, "```json") {
		t.Errorf("normalizeFences:\n%s", n)
	}
}

func TestAnchor(t *testing.T) {
	if a := anchor("ValidationErr.display"); a != "validationerrdisplay" {
		t.Errorf("anchor = %q", a)
	}
}
