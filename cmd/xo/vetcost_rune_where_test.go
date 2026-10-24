package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// vetCostViewWhereTrimProg is the trim_where shape: a trimmed view of a
// line stored in a field, which needs the trim_where names in view_call.
const vetCostViewWhereTrimProg = `
struct Tok {
  name: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    out.push(Tok{name: line.trim_start_where(fn(r) { r == Rune(32) })})
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${parse("  a\n  b").len()}")
}
`

// vetCostViewWherePieceProg is the split_where shape of the split_n test: a
// piece of a split of a line stored in a field, which needs str_split_where
// in param_view_at beside split and fields. The call's name carries its
// generic instantiation (`prelude.str_split_where[()]`), so the match strips
// the suffix.
const vetCostViewWherePieceProg = `
struct Tok {
  name: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    out.push(Tok{name: line.split_where(fn(r) { r.is_space() })[0]})
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${parse("a b").len()}")
}
`

// vetCostViewWhereUsedProg is the negative control: the same view used and
// dropped, never stored, so no XO0911.
const vetCostViewWhereUsedProg = `
fn emit(os: Os, text: Str) {
  os.stdio.println("[${text.trim_start_where(fn(r) { r == Rune(32) })}]")
}

fn main(os: Os) {
  emit(os, "  a")
  for piece in "a b".split_where(fn(r) { r.is_space() }) {
    os.stdio.println(piece)
  }
}
`

// vetCostViewWhereLines is the lines of the XO0911 diagnostics of a vet
// --json --cost run on the program src.
func vetCostViewWhereLines(t *testing.T, src string) []int {
	t.Helper()
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", "--cost", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	var lines []int
	for _, d := range res.Diagnostics {
		if d.Code == "XO0911" {
			lines = append(lines, d.Span.Line)
		}
	}
	return lines
}

// TestVetCostViewStoreWhereTrim: XO0911 is reported for a trim_start_where
// view of a line stored in a field; without the trim_where names in
// view_call the note disappears.
func TestVetCostViewStoreWhereTrim(t *testing.T) {
	t.Parallel()
	if lines := vetCostViewWhereLines(t, vetCostViewWhereTrimProg); len(lines) != 1 || lines[0] != 9 {
		t.Errorf("XO0911 at lines %v, want [9]", lines)
	}
}

// TestVetCostViewStoreWherePiece: XO0911 is reported for a piece of
// split_where stored in a field; without str_split_where in param_view_at
// the note disappears.
func TestVetCostViewStoreWherePiece(t *testing.T) {
	t.Parallel()
	if lines := vetCostViewWhereLines(t, vetCostViewWherePieceProg); len(lines) != 1 || lines[0] != 9 {
		t.Errorf("XO0911 at lines %v, want [9]", lines)
	}
}

// TestVetCostViewWhereUsedNoNote: the negative control, a view of a
// parameter's text used and dropped on the spot, draws no XO0911, so the
// two tests above fail for the stored view and not for any use.
func TestVetCostViewWhereUsedNoNote(t *testing.T) {
	t.Parallel()
	if lines := vetCostViewWhereLines(t, vetCostViewWhereUsedProg); len(lines) != 0 {
		t.Errorf("XO0911 at lines %v, want none", lines)
	}
}
