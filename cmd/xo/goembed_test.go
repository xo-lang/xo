package main

import (
	"bytes"
	"strings"
	"testing"
)

// The h1 hashes of testdata/goproxy's synthetic example.com/embed v1.0.0.
const (
	embedSum    = "h1:h2Prp2iZ1+MX1DRjknOE3ZitftgqkDeoc+MjJ7l2bAQ="
	embedModSum = "h1:ji4mw0hAiLuSAXE4dUIj0ZismZq31dwulUMcDVkwef0="
)

// TestUseGoEmbeddedFields (decision 0491 A, the owner's v0.36.0
// evaluation: zap's observer.LoggedEntry embeds zapcore.Entry, whose
// Message nothing could read): an embedded struct field gets a getter
// that is a handle of the outer value's field (a change through it is
// the outer value's), the fields it promotes get accessors on the outer
// handle (through an unexported embedded type, at depth two), an
// ambiguous name gets none, an embedded pointer is an option, and an
// embedded interface is its handle.
func TestUseGoEmbeddedFields(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module emb\n\nrequire go example.com/embed v1.0.0\n",
		"xo.lock": "go example.com/embed v1.0.0 " + embedSum + "\n" +
			"go example.com/embed v1.0.0/go.mod " + embedModSum + "\n",
		"main.xo": `use go "example.com/embed"

fn main(os: Os) {
  let le = embed.make("hi") ?? return
  os.stdio.println("${le.message()} ${le.level()}")
  le.set_message("set")
  os.stdio.println(embed.message(le))
  let e = le.entry()
  e.set_message("through entry")
  os.stdio.println(le.message())
  let o = embed.new_outer()
  os.stdio.println("${o.depth2()} ${o.name()} ${o.tag()}")
  let a = embed.Amb.new()
  a.set_y(3)
  os.stdio.println("${a.y()} ${a.a().x()}")
  let p = embed.Ptr.new()
  os.stdio.println("${p.entry().is_none()} ${p.n()}")
  let w = embed.WithIface.new()
  os.stdio.println("${w.k()}")
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "hi 2\nset\nthrough entry\ndeep mid top\n3 0\ntrue 0\n0"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
	// the ambiguous X: no accessor on Amb
	out.Reset()
	errOut.Reset()
	bad := writeFiles(t, map[string]string{
		"xo.mod": "module emb\n\nrequire go example.com/embed v1.0.0\n",
		"xo.lock": "go example.com/embed v1.0.0 " + embedSum + "\n" +
			"go example.com/embed v1.0.0/go.mod " + embedModSum + "\n",
		"main.xo": "use go \"example.com/embed\"\n\nfn main(os: Os) {\n  os.stdio.println(\"${embed.Amb.new().x()}\")\n}\n",
	})
	if code := run(t, []string{"check", "--backend=go", bad}, &out, &errOut); code == 0 {
		t.Errorf("check of Amb.x(): exit 0, want an error (X is ambiguous)\n%s%s", out.String(), errOut.String())
	}
}

// TestUseGoCopyWarning (decision 0491 B, option (c)): a List or Map
// given to a Go function as `any`, by `go.any(xs)` or as an element of a
// `...any` list, is XO1107 in `xo check`, a warning: Go changes a copy.
// Not for a function compiler/check/goreads.xo lists (json.marshal,
// fmt.sprintf), nor with `// xo:allow XO1107 <reason>`.
func TestUseGoCopyWarning(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module cp\n",
		"main.xo": `use go "encoding/json"
use go "fmt"
use std/go

fn main(os: Os) {
  var xs = [1, 2]
  let m = {"a": 1}
  json.unmarshal(b"[3]", go.any(xs)).ignore_err("only the warning")
  let _ = fmt.sscan("4", [m]).ignore_err("only the warning")
  let _ = json.marshal(go.any(xs)).ignore_err("allowlisted")
  let _ = fmt.sprintf("%v %v", [xs, m])
  // xo:allow XO1107 the test's silenced call
  json.unmarshal(b"[5]", go.any(xs)).ignore_err("only the warning")
  xs.push(3)
  os.stdio.println("${xs.len()}")
}
`,
	})
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--backend=go", dir}, &out, &errOut)
	all := out.String() + errOut.String()
	if code != 0 {
		t.Fatalf("check: exit %d, want 0 (XO1107 is a warning)\n%s", code, all)
	}
	var lines []string
	for _, l := range strings.Split(all, "\n") {
		if strings.Contains(l, "warning XO1107") {
			lines = append(lines, l)
		}
	}
	if len(lines) != 2 || !strings.Contains(lines[0], "main.xo:8:") || !strings.Contains(lines[0], "copy of this List") || !strings.Contains(lines[1], "main.xo:9:") || !strings.Contains(lines[1], "copy of this Map") {
		t.Errorf("XO1107 lines %q, want one at 8 (List, json.unmarshal) and one at 9 (Map, fmt.sscan)\n%s", lines, all)
	}
}
