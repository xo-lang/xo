package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// vetcostBytesCutPrefixProg: a Bytes parameter's cut_prefix piece stored in a
// struct inside a loop (XO0911, line 9).
const vetcostBytesCutPrefixProg = `
struct Tok {
  name: Bytes
}

fn parse(data: Bytes) -> List[Tok] {
  var out: List[Tok] = []
  for line in data.split(b"\n") {
    out.push(Tok{name: line.cut_prefix(b"x") ?? b""})
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${parse(b"x1\ny2").len()}")
}
`

// vetcostBytesCutProg: the same with bytes.cut, whose tuple of views goes
// into a returned collection through the Option's payload (XO0911, line 5,
// the push).
// A cut destructured into a struct field is not noted: the tuple
// destructure is not a chain the analysis follows (a gap, not a wrong note).
const vetcostBytesCutProg = `
fn parse(data: Bytes) -> List[(Bytes, Bytes)] {
  var out: List[(Bytes, Bytes)] = []
  for line in data.split(b"\n") {
    out.push(line.cut(b"=") ?? (b"", b""))
  }
  out
}

fn main(os: Os) {
  os.stdio.println("${parse(b"a=1\nb=2")[0].0.len()}")
}
`

// vetcostBytesCutConsumedProg: the control, where every cut is consumed in
// the loop and nothing of the parameter outlives it, so no note is right.
const vetcostBytesCutConsumedProg = `
fn width(data: Bytes) -> Int {
  var n = 0
  for line in data.split(b"\n") {
    let (name, rest) = line.cut(b"=") ?? (line, b"")
    n += name.len() + rest.len() + (line.cut_suffix(b"!") ?? b"").len()
  }
  n
}

fn main(os: Os) {
  os.stdio.println("${width(b"a=1!\nb=2")}")
}
`

// vetcostBytesLines runs xo vet --cost on prog and returns the line of
// every XO0911 note.
func vetcostBytesLines(t *testing.T, prog string) []int {
	t.Helper()
	dir := writeFiles(t, map[string]string{"main.xo": prog})
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

// TestVetCostViewStoreBytesCut: XO0911 names the Bytes cut family
// (compiler/ssa/cost.xo view_call: cut, cut_prefix, cut_suffix) beside the
// Str forms, through the tuple and the Option a cut returns.
func TestVetCostViewStoreBytesCut(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		prog string
		line int
	}{
		{"cut_prefix", vetcostBytesCutPrefixProg, 9},
		{"cut", vetcostBytesCutProg, 5},
	} {
		if lines := vetcostBytesLines(t, tc.prog); len(lines) != 1 || lines[0] != tc.line {
			t.Errorf("%s: XO0911 at lines %v, want [%d]", tc.name, lines, tc.line)
		}
	}
}

// TestVetCostViewStoreBytesCutControl: the negative control, a cut whose
// pieces are consumed in the loop and never stored, which must not be noted
// (without the guard it would make the test above pass on any note).
func TestVetCostViewStoreBytesCutControl(t *testing.T) {
	t.Parallel()
	if lines := vetcostBytesLines(t, vetcostBytesCutConsumedProg); len(lines) != 0 {
		t.Errorf("the control noted XO0911 at lines %v, want none", lines)
	}
}
