package main

import "testing"

// TestVetScriptJSON covers XO0922 (task #36): std/json's encode output
// in a function that writes a "<script" literal, not passed through
// json.html_escape (gotchas/json-encode-no-html-escape.md: a value holding
// "</script>" closes the element). Concatenated, interpolated, or written
// to a builder after the tag, in any case of the tag, and encoded before
// the literal into a local, it reports; escaped, or in a function with no
// script literal, it does not.
func TestVetScriptJSON(t *testing.T) {
	src := `use std/json

fn page(s: Str) -> Str {
  let a = "<script>var s = " + json.encode_str(s) + ";</script>"
  let b = "<SCRIPT type=\"application/json\">${json.encode_str(s)}</SCRIPT>"
  let c = "<script>var s = ${json.html_escape(json.encode_str(s))};</script>"
  a + b + c
}

fn later(s: Str) -> Str {
  let j = json.encode_str(s)
  var sb = StrBuilder.new()
  sb.write("<script>")
  sb.write(j)
  sb.freeze()
}

fn api(s: Str) -> Str { json.encode_str(s) }

fn main(os: Os) {
  os.stdio.println(page("x") + later("y") + api("z"))
}
`
	got := vetCodes(t, src)
	g := got["XO0922"]
	if len(g) != 3 || g[0] != 4 || g[1] != 5 || g[2] != 11 {
		t.Errorf("XO0922 at %v, want [4 5 11]: %v", g, got)
	}
}
