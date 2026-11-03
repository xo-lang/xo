// A fuzz harness for std/xml against Go's encoding/xml (2026-11-02,
// decision 0461: a parser of untrusted input is Early on testing until it
// is fuzzed).
//
// std/xml is Go's Decoder (decision 0314): the same tokens and the same
// error texts. Each case is a document from pieces a hostile body
// reaches: prologs (another version, an encoding with no CharsetReader),
// DOCTYPEs with an internal subset, names with prefixes and the bytes a
// name cannot start with, attributes quoted, unquoted, bare, duplicated
// and namespaced, entities known, unknown, numeric (a surrogate, zero)
// and HTML's, CDATA open and closed, comments with "--", processing
// instructions, control bytes, mismatched and unclosed elements, several
// roots, and each cut short or with a byte changed. Every case is UTF-8
// (tokens takes a Str).
//
// Each case is compared three ways, as Go answers them: xml.tokens (Go's
// Decoder.Token to EOF), xml.tokens with strict false (Strict = false,
// AutoClose = HTMLAutoClose, Entity = HTMLEntity), and xml.raw_tokens
// (RawToken). The answer is "err" and the error's text exactly, or "ok"
// and every token rendered (names, attributes, text as hex).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/xmlfuzz/xmlfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestXmlFuzzCorpusCurrent checks it is what this
// generator writes (XO_XMLFUZZ_WRITE=1 rewrites it). TestXmlFuzz runs a
// fresh seed (XO_XMLFUZZ_SEED, default 2; XO_XMLFUZZ_N cases, default
// 2000) on XO_XMLFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// xfCase is one document and Go's three answers for it.
type xfCase struct {
	doc               string
	strict, html, raw string
}

var xfProlog = []string{
	"", "", "", `<?xml version="1.0"?>`, `<?xml version="1.0" encoding="UTF-8"?>` + "\n",
	`<?xml version="1.0" encoding="latin1"?>`, `<?xml version="2.0"?>`, `<?xml?>`,
	"<!DOCTYPE html>", `<!DOCTYPE a [<!ENTITY x "y">]>`, `<!DOCTYPE a [<!ELEMENT a ANY>`,
}

var xfNames = []string{"a", "b", "a", "b", "x:y", "p:a", "_a", "a-b", "a.b", "\xc3\xa9"}
var xfBadNames = []string{"1a", "xmlns", "", "-a", "a:b:c", ":a"}

var xfAttrs = []string{
	`k="v"`, `k='v'`, `k="1" j="2"`, `xmlns="urn:x"`, `xmlns:p="urn:p"`, `p:k="1"`, `k="&amp;"`,
	`k="a&#10;b"`, "k=\"\t\n\"", `k=""`, `k="&lt;&gt;"`,
}
var xfBadAttrs = []string{`k=v`, `k`, `k="&bogus;"`, `k="<"`, `k="1" k="2"`, `k="&"`, `="v"`}

var xfText = []string{
	"t", "hello world", " ", "\n", "\r\n", "&amp;", "&lt;", "&gt;", "&quot;", "&apos;", "&#65;",
	"&#x41;", "<![CDATA[x<y]]>", "<!-- c -->", "<!---->", "<?pi x?>", "\t", "\xc3\xa9", "]]",
}
var xfBadText = []string{
	"&#xD800;", "&#0;", "&#x110000;", "&nbsp;", "&copy;", "&bogus;", "&", "&#;", "&amp", "]]>",
	"<![CDATA[", "<!-- a--b -->", "<?xml x?>", "\x01", "<", "<!x>", "<!DOCTYPE a>",
}

// xfMix picks from good, and from bad one time in n.
func xfMix(r *rand.Rand, good, bad []string, n int) string {
	if r.Intn(n) == 0 {
		return xfPick(r, bad)
	}
	return xfPick(r, good)
}

func xfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// xfElem is an element of up to depth levels.
func xfElem(r *rand.Rand, depth int) string {
	name := xfMix(r, xfNames, xfBadNames, 20)
	var b strings.Builder
	b.WriteString("<" + name)
	for i, n := 0, r.Intn(3); i < n; i++ {
		b.WriteString(" " + xfMix(r, xfAttrs, xfBadAttrs, 12))
	}
	if r.Intn(5) == 0 {
		b.WriteString(xfPick(r, []string{"/>", " />"}))
		return b.String()
	}
	b.WriteString(">")
	for i, n := 0, r.Intn(4); i < n; i++ {
		if depth > 0 && r.Intn(2) == 0 {
			b.WriteString(xfElem(r, depth-1))
		} else {
			b.WriteString(xfMix(r, xfText, xfBadText, 10))
		}
	}
	switch r.Intn(30) {
	case 0: // unclosed
	case 1:
		b.WriteString("</" + xfMix(r, xfNames, xfBadNames, 20) + ">")
	case 2:
		b.WriteString("</" + name + " >")
	default:
		b.WriteString("</" + name + ">")
	}
	return b.String()
}

// xfCases builds the seeded cases: a prolog and a root, sometimes a second
// root or text after it, one in ten cut short, one in ten with a byte
// changed (a case that stops being UTF-8 is drawn again).
func xfCases(r *rand.Rand, n int) []xfCase {
	cases := make([]xfCase, 0, n)
	for len(cases) < n {
		s := xfPick(r, xfProlog) + xfElem(r, 3)
		if r.Intn(10) == 0 {
			s += xfPick(r, []string{xfElem(r, 1), "t", " ", "\n", "<!-- c -->", "<?pi?>"})
		}
		raw := []byte(s)
		switch r.Intn(10) {
		case 0:
			if len(raw) > 0 {
				raw = raw[:r.Intn(len(raw))]
			}
		case 1:
			if len(raw) > 0 {
				mutate := "<>/=\"'&;!?[]- a:"
				raw[r.Intn(len(raw))] = mutate[r.Intn(len(mutate))]
			}
		}
		if !utf8.Valid(raw) {
			continue
		}
		c := xfCase{doc: string(raw)}
		c.strict, c.html, c.raw = xfGo(c.doc, 0), xfGo(c.doc, 1), xfGo(c.doc, 2)
		cases = append(cases, c)
	}
	return cases
}

// xfGo is Go's answer for doc: mode 0 Token, 1 Token not strict with
// HTML's autoclose and entities, 2 RawToken.
func xfGo(doc string, mode int) string {
	d := xml.NewDecoder(strings.NewReader(doc))
	if mode == 1 {
		d.Strict = false
		d.AutoClose = xml.HTMLAutoClose
		d.Entity = xml.HTMLEntity
	}
	var toks []string
	for {
		var t xml.Token
		var err error
		if mode == 2 {
			t, err = d.RawToken()
		} else {
			t, err = d.Token()
		}
		if err == io.EOF {
			return "ok " + strings.Join(toks, ",")
		}
		if err != nil {
			return "err " + err.Error()
		}
		toks = append(toks, xfRender(t))
	}
}

func xfName(n xml.Name) string { return fmt.Sprintf("%x:%x", n.Space, n.Local) }

// xfRender is one token in the shape the generated test's xf_tok writes.
func xfRender(t xml.Token) string {
	switch t := t.(type) {
	case xml.StartElement:
		s := "S" + xfName(t.Name)
		for _, a := range t.Attr {
			s += fmt.Sprintf(" %s=%x", xfName(a.Name), a.Value)
		}
		return s
	case xml.EndElement:
		return "E" + xfName(t.Name)
	case xml.CharData:
		return fmt.Sprintf("T%x", []byte(t))
	case xml.Comment:
		return fmt.Sprintf("C%x", []byte(t))
	case xml.ProcInst:
		return fmt.Sprintf("P%x|%x", t.Target, t.Inst)
	case xml.Directive:
		return fmt.Sprintf("D%x", []byte(t))
	}
	panic(fmt.Sprintf("xfRender: %T", t))
}

const xfRenderXo = `use std/xml

fn xf_n(n: xml.Name) -> Str {
  "${n.space.bytes().to_hex()}:${n.local.bytes().to_hex()}"
}

fn xf_tok(t: xml.Token) -> Str {
  match t {
    Start(name, attrs) => {
      var s = "S" + xf_n(name)
      for a in attrs {
        s = s + " " + xf_n(a.name) + "=" + a.value.bytes().to_hex()
      }
      s
    }
    End(name) => "E" + xf_n(name)
    Text(s) => "T" + s.bytes().to_hex()
    Comment(s) => "C" + s.bytes().to_hex()
    ProcInst(target, inst) => "P" + target.bytes().to_hex() + "|" + inst.bytes().to_hex()
    Directive(s) => "D" + s.bytes().to_hex()
  }
}

/// mode 0 tokens, 1 tokens not strict, 2 raw_tokens
fn xf_line(doc: Str, mode: Int) -> Str {
  let r = if mode == 0 { xml.tokens(doc) } else if mode == 1 { xml.tokens(doc, strict: false) } else { xml.raw_tokens(doc) }
  match r {
    Ok(ts) => {
      var parts: List[Str] = []
      for t in ts {
        parts.push(xf_tok(t))
      }
      "ok " + parts.join(",")
    }
    Err(e) => "err " + e.display()
  }
}

`

// xfXoFile is the generated test file: the renderer and one test per case.
func xfXoFile(seed int64, cases []xfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/xmlfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// encoding/xml; do not edit: XO_XMLFUZZ_WRITE=1 go test ./internal/stdtest -run TestXmlFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(xfRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"xml fuzz %d\" {\n  let doc = %s\n", i, xoStr(c.doc))
		for _, m := range []struct{ name, call, want string }{
			{"strict", "xf_line(doc, 0)", c.strict},
			{"html", "xf_line(doc, 1)", c.html},
			{"raw", "xf_line(doc, 2)", c.raw},
		} {
			fmt.Fprintf(&b, "  let %s = %s\n  if %s != %s {\n    fault(\"%s: got \" + %s)\n  }\n", m.name, m.call, m.name, xoStr(m.want), m.name, m.name)
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// xfCoverage fails when the cases stop reaching the decoder's parts.
func xfCoverage(t *testing.T, cases []xfCase) {
	t.Helper()
	oks, errs, htmlOks, attrs, texts, ends := 0, 0, 0, 0, 0, 0
	msgs := map[string]bool{}
	for _, c := range cases {
		if strings.HasPrefix(c.strict, "err ") {
			errs++
			msgs[c.strict[strings.LastIndex(c.strict, ":")+1:]] = true
		} else {
			oks++
			if strings.Contains(c.strict, "=") {
				attrs++
			}
			if strings.Contains(c.strict, ",T") {
				texts++
			}
			if strings.Contains(c.strict, ",E") {
				ends++
			}
		}
		if strings.HasPrefix(c.html, "ok ") {
			htmlOks++
		}
	}
	t.Logf("answers: strict %d ok, %d err (%d distinct messages); not strict %d ok; ok with attributes %d, text %d, an end %d", oks, errs, len(msgs), htmlOks, attrs, texts, ends)
	if oks < len(cases)/5 || errs < len(cases)/5 || len(msgs) < 10 || htmlOks < len(cases)/4 || attrs < 100 || texts < 100 || ends < 100 {
		t.Fatalf("the cases stopped covering the decoder: %d ok, %d err, %d messages, %d not strict ok, %d attrs, %d text, %d ends", oks, errs, len(msgs), htmlOks, attrs, texts, ends)
	}
}

const xfCorpus = "testdata/corpus/xmlfuzz/xmlfuzz_test.xo"

// TestXmlFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestXmlFuzzCorpusCurrent(t *testing.T) {
	cases := xfCases(rand.New(rand.NewSource(1)), 2000)
	xfCoverage(t, cases)
	text := xfXoFile(1, cases)
	if os.Getenv("XO_XMLFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(xfCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(xfCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(xfCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's encoding/xml or the generator changed): XO_XMLFUZZ_WRITE=1 go test ./internal/stdtest -run TestXmlFuzzCorpusCurrent", xfCorpus)
	}
}

// TestXmlFuzz runs a fresh seed's cases.
func TestXmlFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the xml fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_XMLFUZZ_SEED", 2))
	n := envInt(t, "XO_XMLFUZZ_N", 2000)
	cases := xfCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d %q", i, c.doc)
	}
	runFuzzDir(t, "xmlfuzz_test.xo", xfXoFile(seed, cases), envBackends("XO_XMLFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
