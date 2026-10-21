package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
)

// xmlTokText is the corpus's text of a token, as the Xo test writes it.
func xmlTokText(t xml.Token) string {
	n := func(x xml.Name) string { return textOf(x.Space) + ":" + textOf(x.Local) }
	switch v := t.(type) {
	case xml.StartElement:
		var as []string
		for _, a := range v.Attr {
			as = append(as, n(a.Name)+"="+textOf(a.Value))
		}
		return "S(" + n(v.Name) + "[" + strings.Join(as, ",") + "])"
	case xml.EndElement:
		return "E(" + n(v.Name) + ")"
	case xml.CharData:
		return "T(" + textOf(string(v)) + ")"
	case xml.Comment:
		return "C(" + textOf(string(v)) + ")"
	case xml.ProcInst:
		return "P(" + textOf(v.Target) + "," + textOf(string(v.Inst)) + ")"
	case xml.Directive:
		return "D(" + textOf(string(v)) + ")"
	}
	return "?"
}

func xmlErrText(err error) string {
	var se *xml.SyntaxError
	if errors.As(err, &se) {
		return fmt.Sprintf("err %d %s", se.Line, textOf(se.Msg))
	}
	return "err 0 " + textOf(err.Error())
}

// xmlToks is std/rt's tokens or raw_tokens: the tokens' texts joined, or
// err LINE MSG.
func xmlToks(text string, strict, raw bool) string {
	d := xml.NewDecoder(strings.NewReader(text))
	if !strict {
		d.Strict = false
		d.AutoClose = xml.HTMLAutoClose
		d.Entity = xml.HTMLEntity
	}
	var out []string
	for {
		var t xml.Token
		var err error
		if raw {
			t, err = d.RawToken()
		} else {
			t, err = d.Token()
		}
		if err == io.EOF {
			return strings.Join(out, " ")
		}
		if err != nil {
			return xmlErrText(err)
		}
		out = append(out, xmlTokText(t))
	}
}

// xmlEncodeOf is std/rt's parse then encode with two-space indent: the
// encoding, or err LINE MSG (the tree's own errors are line 0).
func xmlEncodeOf(text string) string {
	d := xml.NewDecoder(strings.NewReader(text))
	var toks []xml.Token
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlErrText(err)
		}
		toks = append(toks, xml.CopyToken(t))
	}
	// the root: one element, only blank text outside it
	depth, roots := 0, 0
	var inside []xml.Token
	for _, t := range toks {
		switch v := t.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return "err 0 xml: more than one root element"
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				inside = append(inside, t)
				continue
			}
		case xml.CharData:
			if depth == 0 {
				if strings.TrimLeft(string(v), " \t\r\n") != "" {
					return "err 0 xml: text outside the root element"
				}
				continue
			}
		}
		if depth > 0 {
			inside = append(inside, t)
		}
	}
	if roots == 0 {
		return "err 0 xml: no root element"
	}
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	for _, t := range inside {
		if err := enc.EncodeToken(t); err != nil {
			return xmlErrText(err)
		}
	}
	if err := enc.Flush(); err != nil {
		return xmlErrText(err)
	}
	return "ok " + textOf(b.String())
}

func xmlName(r *rand.Rand) string {
	if r.Intn(12) == 0 {
		return []string{"1bad", "", "a b", "xmlns", "p:q:r"}[r.Intn(5)]
	}
	return []string{"a", "b", "item", "x:y", "ns:el", "p:q", "Root", "é", "_n", "a.b-c", "xml:lang", "list", "row"}[r.Intn(13)]
}

func xmlGenElem(r *rand.Rand, b *strings.Builder, depth int) {
	name := xmlName(r)
	b.WriteString("<" + name)
	for i, n := 0, r.Intn(3); i < n; i++ {
		an := []string{"id", "x:y", "xmlns", "xmlns:x", "xmlns:ns", "xmlns:p", "a", "b", "class", "id", "xml:lang"}[r.Intn(11)]
		q := []string{`"`, `"`, `"`, `'`, `'`, `"`, `"`, `"`, `"`, ``}[r.Intn(10)]
		v := []string{"1", "http://e.com/ns", "urn:x", "a&amp;b", "&lt;", "x\ty\nz", "", "é", "&#65;", "&#x42;", "v", "2", "http://e.com/ns"}[r.Intn(13)]
		if r.Intn(10) == 0 {
			v = []string{"a&b", "&#xZZ;", "&nbsp;", "a\"b", "a<b"}[r.Intn(5)]
		}
		b.WriteString(" " + an + "=" + q + v + q)
	}
	switch r.Intn(6) {
	case 0:
		b.WriteString("/>")
		return
	case 1:
		b.WriteString(" />")
		return
	}
	b.WriteString(">")
	for i, n := 0, r.Intn(4); i < n; i++ {
		switch k := r.Intn(10); {
		case k < 3:
			b.WriteString([]string{"text", " ", "\n  ", "a &amp; b", "&lt;x&gt;", "&quot;&apos;", "&#233;", "&#x1F600;", "é日", "\r\n", "word", "]]>"}[r.Intn(12)])
			if r.Intn(12) == 0 {
				b.WriteString([]string{"&bogus;", "a & b", "&#0;", "&nbsp;"}[r.Intn(4)])
			}
		case k < 6 && depth > 0:
			xmlGenElem(r, b, depth-1)
		case k == 6:
			b.WriteString([]string{"<!-- c -->", "<!---->", "<!-- a--b -->", "<!-- x -- -->"}[r.Intn(4)])
		case k == 7:
			b.WriteString([]string{"<![CDATA[<raw> & ]]>", "<![CDATA[]]>", "<![CDATA[a]]b]]>", "<![CDATA[x"}[r.Intn(4)])
		case k == 8:
			b.WriteString([]string{"<?pi data?>", "<?target?>", "<?xml-stylesheet href='a'?>", "<?xml version=\"1.0\"?>"}[r.Intn(4)])
		default:
			if r.Intn(3) == 0 {
				b.WriteString([]string{"<br>", "</x>", "<", "&", "<p>unclosed"}[r.Intn(5)])
			}
		}
	}
	if r.Intn(25) > 0 {
		b.WriteString("</" + name + ">")
	} else {
		b.WriteString("</" + xmlName(r) + ">")
	}
}

func xmlDoc(r *rand.Rand) string {
	var b strings.Builder
	if r.Intn(3) == 0 {
		b.WriteString([]string{`<?xml version="1.0" encoding="UTF-8"?>`, `<?xml version="1.0"?>` + "\n", `<?xml version="1.0" encoding="utf-8"?>`, "<!DOCTYPE html>\n", "<!-- head -->", `<?xml version="1.1"?>`, `<?xml version="1.0" encoding="latin1"?>`}[r.Intn(7)])
	}
	xmlGenElem(r, &b, 3)
	if r.Intn(5) == 0 {
		b.WriteString([]string{"\n", " <x/>", "tail", "<!-- t -->"}[r.Intn(4)])
	}
	s := b.String()
	if r.Intn(12) == 0 && len(s) > 0 {
		s = s[:r.Intn(len(s))]
	}
	return textOf(s)
}

func xmlCorpus() string {
	r := rand.New(rand.NewSource(13))
	var o strings.Builder
	o.WriteString(header("encoding/xml's Decoder (Token strict and HTML-lenient, RawToken) and Encoder as std/rt/xml.go calls them, on seeded random documents: namespaces, entities, CDATA, comments, processing instructions, bad names, cuts"))
	o.WriteString("use std/xml\n\n/// (document, Go's tokens strict, lenient, raw; Go's parse and two-space encode)\nconst DOCS: List[(Str, Str, Str, Str, Str)] = [\n")
	for i := 0; i < 2000; i++ {
		s := xmlDoc(r)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(s), xq(xmlToks(s, true, false)), xq(xmlToks(s, false, false)), xq(xmlToks(s, true, true)), xq(xmlEncodeOf(s)))
	}
	o.WriteString("]\n\n/// (text, Go's EscapeText)\nconst ESCAPES: List[(Str, Str)] = [\n")
	for i := 0; i < 200; i++ {
		var b strings.Builder
		for j, n := 0, r.Intn(8); j < n; j++ {
			b.WriteString([]string{"a", "<", ">", "&", "\"", "'", "\t", "\n", "\r", "é", "\x01", "￾", "日"}[r.Intn(13)])
		}
		var e bytes.Buffer
		xml.EscapeText(&e, []byte(b.String()))
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(textOf(b.String())), xq(textOf(e.String())))
	}
	o.WriteString("]\n")
	o.WriteString(xmlTests)
	xmlDeriveCorpus(&o)
	return o.String()
}

const xmlTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn nm(n: xml.Name) -> Str { "${n.space}:${n.local}" }

fn tok_text(t: xml.Token) -> Str {
  match t {
    xml.Token.Start(name, attrs) => "S(${nm(name)}[${attrs.map(fn(a) { "${nm(a.name)}=${a.value}" }).join(",")}])"
    xml.Token.End(name) => "E(${nm(name)})"
    xml.Token.Text(s) => "T(${s})"
    xml.Token.Comment(s) => "C(${s})"
    xml.Token.ProcInst(target, inst) => "P(${target},${inst})"
    xml.Token.Directive(s) => "D(${s})"
  }
}

fn toks(r: Result[List[xml.Token], xml.XmlErr]) -> Str {
  match r {
    Ok(ts) => ts.map(tok_text).join(" ")
    Err(e) => "err ${e.line} ${e.msg}"
  }
}

test "tokens and raw_tokens are Go's Decoder's" {
  var bad: List[(Str, Str, Str)] = []
  for (doc, strict, lenient, raw, _enc) in DOCS {
    let got = toks(xml.tokens(doc))
    if got != strict {
      bad.push((doc, got, strict))
    }
    let got2 = toks(xml.tokens(doc, strict: false))
    if got2 != lenient {
      bad.push((doc, got2, lenient))
    }
    let got3 = toks(xml.raw_tokens(doc))
    if got3 != raw {
      bad.push((doc, got3, raw))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse and encode are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (doc, _s, _l, _r, want) in DOCS {
    let got = match xml.parse(doc) {
      Ok(e) => match e.encode(indent: "  ") {
        Ok(s) => "ok ${s}"
        Err(x) => "err ${x.line} ${x.msg}"
      }
      Err(x) => "err ${x.line} ${x.msg}"
    }
    if got != want {
      bad.push((doc, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "escape is Go's EscapeText" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in ESCAPES {
    let got = xml.escape(s)
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
