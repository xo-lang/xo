package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// vetCostViewTrimCharsProg is the trim_chars shape: a trimmed view of a
// line stored in a field, which needs the three names in view_call.
const vetCostViewTrimCharsProg = `
struct Tok {
  name: Str
}

fn parse(text: Str) -> List[Tok] {
  var out: List[Tok] = []
  for line in text.lines() {
    out.push(Tok{name: line.trim_start_chars("x ")})
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${parse("x1 \nx2").len()}")
}
`

// TestVetCostViewStoreTrimChars: XO0911 is reported for a
// trim_start_chars view of a line stored in a field; without the three
// names in view_call the note disappears.
func TestVetCostViewStoreTrimChars(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"main.xo": vetCostViewTrimCharsProg})
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
	if len(lines) != 1 || lines[0] != 9 {
		t.Errorf("XO0911 at lines %v, want [9]\n%s", lines, out.String())
	}
}
