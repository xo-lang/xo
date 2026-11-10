package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// hoverProg uses std/charset (a free fn, a method, a type), a prelude
// method with a doc comment (Str.split_where) and one without
// (List.sort_by), and a fn of its own.
const hoverProg = `use std/charset

fn helper() -> Int {
  1
}

fn main(os: Os) {
  let cs = charset.Charset.Utf16Le
  match charset.decode("hi".bytes(), cs) {
    Ok(s) => os.stdio.println(s.split_where(fn(r: Rune) -> Bool { r == ' ' }).join(","))
    Err(e) => os.stdio.println(e.display())
  }
  let xs = [3, 1, 2].sort_by(fn(x: Int) -> Int { x })
  os.stdio.println("${helper()} ${xs}")
}
`

// TestLSPHoverDocLinks: hover on a std declaration ends with a link to
// its page of the std reference, as gopls links pkg.go.dev: the id is the
// declaration's name ("#CharsetErr.display"). The prelude, documented in
// the spec, and the program's own code get none; a client turns the links
// off with initializationOptions.hoverDocLinks false.
func TestLSPHoverDocLinks(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{"xo.mod": "module example.com/hov\nedition preview\n", "main.xo": hoverProg})
	uri := "file://" + filepath.Join(dir, "main.xo")
	session := func(opts map[string]any) *lspClient {
		c := startLSP(t, dir)
		c.request("initialize", map[string]any{"processId": nil, "rootUri": "file://" + dir, "capabilities": map[string]any{}, "initializationOptions": opts})
		c.notify("initialized", map[string]any{})
		c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "xo", "version": 1, "text": hoverProg}})
		return c
	}
	hover := func(c *lspClient, mark string) string {
		l, ch := lspAt(t, hoverProg, mark)
		h, _ := c.request("textDocument/hover", lspPos(uri, l, ch-1)).(map[string]any)
		return fmt.Sprint(h["contents"])
	}
	const site = "https://xolang.dev/docs/stdlib/"
	c := session(map[string]any{})
	for _, k := range []struct{ mark, sig, link string }{
		{"charset.decode", "fn decode", "[charset.decode](" + site + "charset/#decode) on xolang.dev"},
		{"e.display", "display", "[charset.CharsetErr.display](" + site + "charset/#CharsetErr.display) on xolang.dev"},
		{"charset.Charset", "Charset", "[charset.Charset](" + site + "charset/#Charset) on xolang.dev"},
	} {
		if got := hover(c, k.mark); !strings.Contains(got, k.sig) || !strings.Contains(got, k.link) {
			t.Errorf("hover on %s = %s\nwant %q and the link %s", k.mark, got, k.sig, k.link)
		}
	}
	// the prelude: its doc comment, no link
	if got := hover(c, "s.split_where"); !strings.Contains(got, "strings.FieldsFunc") || strings.Contains(got, "xolang.dev") {
		t.Errorf("hover on Str.split_where = %s\nwant its doc comment (strings.FieldsFunc) and no link", got)
	}
	if got := hover(c, "].sort_by"); !strings.Contains(got, "sort_by") || strings.Contains(got, "xolang.dev") {
		t.Errorf("hover on List.sort_by = %s\nwant its signature and no link", got)
	}
	if got := hover(c, "${helper"); !strings.Contains(got, "fn helper") || strings.Contains(got, "xolang.dev") {
		t.Errorf("hover on the program's helper = %s\nwant its signature and no link", got)
	}
	off := session(map[string]any{"hoverDocLinks": false})
	if got := hover(off, "charset.decode"); !strings.Contains(got, "fn decode") || strings.Contains(got, "xolang.dev") {
		t.Errorf("hoverDocLinks false: hover on charset.decode = %s\nwant no link", got)
	}
}
