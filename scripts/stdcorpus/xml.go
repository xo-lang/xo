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

// xmlErrDisplay is std/xml's XmlErr.display: Go's SyntaxError.Error() when
// the decoder reports a line ("XML syntax error on line N: MSG"), and the
// message alone otherwise.
func xmlErrDisplay(err error) string {
	var se *xml.SyntaxError
	if errors.As(err, &se) && se.Line > 0 {
		return fmt.Sprintf("XML syntax error on line %d: %s", se.Line, textOf(se.Msg))
	}
	return textOf(err.Error())
}

// xmlFirstErr is the first error the strict Decoder reports on text, nil
// when it reads the text to EOF.
func xmlFirstErr(text string) error {
	d := xml.NewDecoder(strings.NewReader(text))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
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

// xmlBadDocs are documents the strict decoder rejects, one per error shape
// Go reports (an unexpected end, a bad entity, a bad character, a second
// root, a mismatched end tag, and errors after the first line).
var xmlBadDocs = []string{
	"<a>",
	"<a><b></a>",
	"<a>&nope;</a>",
	"<a>\x01</a>",
	"<a>x</a><b/>",
	"<a>\n<b>\n<c>&#xD800;</c>\n</b>\n</a>",
	"<a b='1' b='2'/>",
	"</a>",
	"<a><![CDATA[x</a>",
	"<!-- a -- b --><a/>",
	"<1a/>",
	"<a><b></b></a></a>",
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
	o.WriteString("]\n\n/// (document, the text std/xml's XmlErr.display writes for the decoder's error)\nconst ERRTEXTS: List[(Str, Str)] = [\n")
	count := 0
	for _, s := range xmlBadDocs {
		if err := xmlFirstErr(s); err != nil {
			fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(xmlErrDisplay(err)))
			count++
		}
	}
	for i, n := 0, 0; i < 20000 && n < 60; i++ {
		s := xmlDoc(r)
		if err := xmlFirstErr(s); err != nil {
			fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(xmlErrDisplay(err)))
			n++
			count++
		}
	}
	if count == 0 {
		panic("stdcorpus: xml wrote no ERRTEXTS rows, so the corpus would test nothing")
	}
	fmt.Fprintf(&o, "]\n\n/// The ERRTEXTS rows the generator wrote; the corpus checks the list against it, so an empty or truncated list fails instead of passing\nconst ERRTEXT_COUNT: Int = %d\n", count)
	o.WriteString(xmlTests)
	o.WriteString(xmlWriterCorpus())
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

/// The tokens a Reader gives, as toks shows tokens' result: the list, or
/// the error (twice: it comes again).
fn reader_toks(r: xml.Reader) -> Str {
  var rd = r
  var out: List[Str] = []
  while true {
    match rd.next() {
      Ok(Some(t)) => out.push(tok_text(t))
      Ok(None) => return out.join(" ")
      Err(e) => {
        let again = match rd.next() {
          Err(e2) => e2.line == e.line && e2.msg == e.msg
          _ => false
        }
        return if again { "err ${e.line} ${e.msg}" } else { "error not repeated" }
      }
    }
  }
  ""
}

test "a Reader gives the tokens of tokens and raw_tokens" {
  var bad: List[(Str, Str, Str)] = []
  for (doc, strict, lenient, raw, _enc) in DOCS {
    let got = reader_toks(xml.reader(doc))
    if got != strict {
      bad.push((doc, got, strict))
    }
    let got2 = reader_toks(xml.reader(doc, strict: false))
    if got2 != lenient {
      bad.push((doc, got2, lenient))
    }
    let got3 = reader_toks(xml.raw_reader(doc))
    if got3 != raw {
      bad.push((doc, got3, raw))
    }
  }
  expect(first_bad(bad)) == []
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

test "an XmlErr displays as Go's error text (std/xml XmlErr)" {
  // the count is written by the generator beside the list, so an empty or
  // short list fails here instead of looping zero times and passing
  expect(ERRTEXT_COUNT) > 0
  expect(ERRTEXTS.len()) == ERRTEXT_COUNT
  var bad: List[(Str, Str, Str)] = []
  for (doc, want) in ERRTEXTS {
    let got = match xml.tokens(doc) {
      Ok(_) => "the decoder accepted it"
      Err(e) => e.display()
    }
    if got != want {
      bad.push((doc, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`

// xmlWriterCorpus checks xml.writer, Writer.token, element, take and close
// against Go's Encoder (EncodeToken, Indent, Flush, Close) on seeded
// random step lists: start and end tags (some mismatched, some in
// namespaces), text, comments, processing instructions and directives
// with Go's bad cases, takes between steps, and steps after a close.
func xmlWriterCorpus() string {
	r := rand.New(rand.NewSource(1826))
	type step struct {
		k     int
		a, b  string
		attrs [][3]string
	}
	spaces := []string{"", "", "", "urn:x", "urn:y"}
	locals := []string{"a", "b", "c", "item", ""}
	texts := []string{"", "hi", "a<b&c", "x\ny\r", "-->", "é\t"}
	pis := [][2]string{{"xml", "version=\"1.0\""}, {"go", "fmt"}, {"bad name", ""}, {"go", "a?>b"}, {"t", ""}}
	dirs := []string{"DOCTYPE x", "DOCTYPE x [<!ENTITY y \"z\">]", "bad>", "<!-- c -->x", "x<"}
	var o strings.Builder
	o.WriteString("\n/// (indent, steps (kind, a, b, attrs): 0 start, 1 end, 2 text, 3 comment, 4 procinst, 5 directive, 6 take, 7 close, 8 element of the steps' own open tag; Go's result of each step)\nconst WRITES: List[(Str, List[(Int, Str, Str, List[(Str, Str, Str)])], List[Str])] = [\n")
	for i := 0; i < 400; i++ {
		indent := []string{"", "", "  ", "\t"}[r.Intn(4)]
		var steps []step
		var open []step
		for j, n := 0, 1+r.Intn(12); j < n; j++ {
			switch k := r.Intn(10); {
			case k < 3:
				s := step{k: 0, a: spaces[r.Intn(len(spaces))], b: locals[r.Intn(len(locals))]}
				for m := r.Intn(3); m > 0; m-- {
					s.attrs = append(s.attrs, [3]string{spaces[r.Intn(len(spaces))], locals[r.Intn(4)], texts[r.Intn(len(texts))]})
				}
				steps = append(steps, s)
				open = append(open, s)
			case k < 6:
				if len(open) > 0 && r.Intn(5) != 0 {
					t := open[len(open)-1]
					open = open[:len(open)-1]
					steps = append(steps, step{k: 1, a: t.a, b: t.b})
				} else {
					steps = append(steps, step{k: 1, a: spaces[r.Intn(len(spaces))], b: locals[r.Intn(len(locals))]})
				}
			case k == 6:
				steps = append(steps, step{k: 2, b: texts[r.Intn(len(texts))]})
			case k == 7:
				x := r.Intn(4)
				switch x {
				case 0:
					steps = append(steps, step{k: 3, b: texts[r.Intn(len(texts))]})
				case 1:
					p := pis[r.Intn(len(pis))]
					steps = append(steps, step{k: 4, a: p[0], b: p[1]})
				default:
					steps = append(steps, step{k: 5, b: dirs[r.Intn(len(dirs))]})
				}
			case k == 8:
				steps = append(steps, step{k: 6})
			default:
				if r.Intn(4) == 0 {
					steps = append(steps, step{k: 7})
				} else {
					steps = append(steps, step{k: 8, a: spaces[r.Intn(len(spaces))], b: locals[r.Intn(4)]})
				}
			}
		}
		steps = append(steps, step{k: 6}, step{k: 7}, step{k: 6})
		var b bytes.Buffer
		enc := xml.NewEncoder(&b)
		if indent != "" {
			enc.Indent("", indent)
		}
		var res []string
		res1 := func(err error) {
			if err != nil {
				res = append(res, "err "+err.Error())
			} else {
				res = append(res, "ok")
			}
		}
		var xs []string
		for _, s := range steps {
			var attrs []string
			var gattrs []xml.Attr
			for _, a := range s.attrs {
				attrs = append(attrs, fmt.Sprintf("(%s, %s, %s)", xq(a[0]), xq(a[1]), xq(a[2])))
				gattrs = append(gattrs, xml.Attr{Name: xml.Name{Space: a[0], Local: a[1]}, Value: a[2]})
			}
			xs = append(xs, fmt.Sprintf("(%d, %s, %s, [%s])", s.k, xq(s.a), xq(s.b), strings.Join(attrs, ", ")))
			name := xml.Name{Space: s.a, Local: s.b}
			switch s.k {
			case 0:
				res1(enc.EncodeToken(xml.StartElement{Name: name, Attr: gattrs}))
			case 1:
				res1(enc.EncodeToken(xml.EndElement{Name: name}))
			case 2:
				res1(enc.EncodeToken(xml.CharData(s.b)))
			case 3:
				res1(enc.EncodeToken(xml.Comment(s.b)))
			case 4:
				res1(enc.EncodeToken(xml.ProcInst{Target: s.a, Inst: []byte(s.b)}))
			case 5:
				res1(enc.EncodeToken(xml.Directive(s.b)))
			case 6:
				enc.Flush()
				res = append(res, "take "+b.String())
				b.Reset()
			case 7:
				res1(enc.Close())
			case 8:
				// an element <name>hi<!--c--><kid/></name>, as std/rt's xmlEncode writes a tree
				var err error
				start := xml.StartElement{Name: name}
				if err = enc.EncodeToken(start); err == nil {
					if err = enc.EncodeToken(xml.CharData("hi")); err == nil {
						if err = enc.EncodeToken(xml.Comment("c")); err == nil {
							kid := xml.StartElement{Name: xml.Name{Local: "kid"}}
							if err = enc.EncodeToken(kid); err == nil {
								if err = enc.EncodeToken(kid.End()); err == nil {
									err = enc.EncodeToken(start.End())
								}
							}
						}
					}
				}
				res1(err)
			}
		}
		var rq []string
		for _, x := range res {
			rq = append(rq, xq(x))
		}
		fmt.Fprintf(&o, "  (%s, [%s], [%s]),\n", xq(indent), strings.Join(xs, ", "), strings.Join(rq, ", "))
	}
	o.WriteString("]\n")
	o.WriteString(xmlWriterTest)
	return o.String()
}

const xmlWriterTest = `
fn writer_step(var w: xml.Writer, k: Int, a: Str, b: Str, attrs: List[(Str, Str, Str)]) -> Str {
  let name = xml.Name{space: a, local: b}
  if k == 6 {
    return "take ${w.take()}"
  }
  let r = match k {
    0 => w.token(xml.Token.Start(name: name, attrs: attrs.map(fn(x) { xml.Attr{name: xml.Name{space: x.0, local: x.1}, value: x.2} })))
    1 => w.token(xml.Token.End(name: name))
    2 => w.token(xml.Token.Text(s: b))
    3 => w.token(xml.Token.Comment(s: b))
    4 => w.token(xml.Token.ProcInst(target: a, inst: b))
    5 => w.token(xml.Token.Directive(s: b))
    7 => w.close()
    _ => w.element(xml.Element{name: name, attrs: [], children: [xml.Node.Text(s: "hi"), xml.Node.Comment(s: "c"), xml.Node.Elem(e: xml.Element{name: xml.Name{space: "", local: "kid"}, attrs: [], children: []})]})
  }
  match r {
    Ok(_) => "ok"
    Err(e) => "err ${e}"
  }
}

test "writer is Go's Encoder: EncodeToken, Indent, Flush and Close" {
  var bad: List[(Str, List[Str], List[Str])] = []
  for (indent, steps, want) in WRITES {
    var w = xml.writer(indent: indent)
    var got: List[Str] = []
    for (k, a, b, attrs) in steps {
      got.push(writer_step(w, k, a, b, attrs))
    }
    if got != want {
      bad.push((indent, got, want))
    }
  }
  expect(bad[..bad.len().min(3)]) == []
}
`
