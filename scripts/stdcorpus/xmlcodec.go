package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
)

// The xmlcodec corpus (std/xmlcodec, decisions 0315 and 0312): the helpers
// `derive Xml` generates call, which the derive corpus reaches only through
// a struct. Each helper is checked against Go's encoding/xml on the same
// input: parse_root, attr_of and direct_text against a Decoder token walk,
// int_of, float_of, bool_of and float_text against strconv, and build's
// pieces (open_path, leaf, chardata, comment) against the Encoder over the
// token stream Go's marshal printer would write. Go's encoding/xml and
// strconv are Copyright 2009 The Go Authors, BSD-style license.

// xcFixedDocs are the hand picked documents of the root table: elements,
// attributes, chardata with entities, comments, nested and empty elements,
// CDATA, a namespace prefix, numbers and booleans in text, and malformed
// input.
var xcFixedDocs = []string{
	`<root id="7" class="a&amp;b">text</root>`,
	`<r><a><b>deep</b></a><a/><empty></empty></r>`,
	`<r>a &amp; b &lt;x&gt; &#233; &quot;q&quot; &apos;p&apos;</r>`,
	`<r><!-- c -->x<!-- d --></r>`,
	`<r><![CDATA[<raw> & ]]></r>`,
	`<ns:r xmlns:ns="urn:x" ns:id="1"><ns:c>x</ns:c></ns:r>`,
	`<v> 42 </v>`,
	`<v>-3</v>`,
	`<v>1e3</v>`,
	`<v>0.1</v>`,
	`<v>99999999999999999999</v>`,
	`<b>true</b>`,
	`<b> FALSE </b>`,
	`<b>1</b>`,
	`<b>yes</b>`,
	`<r/>`,
	`<r id="1" id="2">x</r>`,
	`<r a="&#x42;">&#x1F600;</r>`,
	`<r>line1
line2</r>`,
	// malformed and rootless
	`<r>`,
	`<r></x>`,
	`<r a=1>`,
	`<r>&bogus;</r>`,
	`<a/><b/>`,
	`<r>text</r>tail`,
	`text`,
	``,
	`<!-- c -->`,
	`<?pi x?>`,
	`<!DOCTYPE html>`,
	`
`,
}

// xcAttrNames are the local attribute names attr_of is asked for.
var xcAttrNames = []string{"id", "class", "lang", "x"}

// xcRoot is Go's answers for one document: parse_root's element encoded
// with two-space indent (or the error), attr_of for xcAttrNames,
// direct_text, and int_of, float_of and bool_of of that direct text.
func xcRoot(doc string) (root, attrs, direct, parsed string) {
	d := xml.NewDecoder(strings.NewReader(doc))
	depth, roots := 0, 0
	var inside []xml.Token
	var start *xml.StartElement
	var text strings.Builder
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlErrText(err), "", "", ""
		}
		switch v := t.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return "err 0 xml: more than one root element", "", "", ""
				}
				vv := v
				start = &vv
			}
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				inside = append(inside, xml.CopyToken(t))
				continue
			}
		case xml.CharData:
			if depth == 0 {
				if strings.TrimLeft(string(v), " \t\r\n") != "" {
					return "err 0 xml: text outside the root element", "", "", ""
				}
				continue
			}
			if depth == 1 {
				text.WriteString(string(v))
			}
		}
		if depth > 0 {
			inside = append(inside, xml.CopyToken(t))
		}
	}
	if roots == 0 {
		// parse_root maps std's "xml: no root element" to EOF
		return "err 0 EOF", "", "", ""
	}
	var as []string
	for _, want := range xcAttrNames {
		got := "none"
		for _, a := range start.Attr {
			if a.Name.Local == want {
				got = "some:" + textOf(a.Value)
				break
			}
		}
		as = append(as, want+"="+got)
	}
	dt := textOf(text.String())
	pr := "i=" + xcInt(dt) + " f=" + xcFloat(dt) + " b=" + xcBool(dt)
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	for _, t := range inside {
		if err := enc.EncodeToken(t); err != nil {
			// parse_root still returns the element; only its encoding fails
			return xmlErrText(err), strings.Join(as, " "), dt, pr
		}
	}
	if err := enc.Flush(); err != nil {
		return xmlErrText(err), strings.Join(as, " "), dt, pr
	}
	return "ok " + textOf(b.String()), strings.Join(as, " "), dt, pr
}

// xcInt is int_of's contract: Int.parse of the trimmed text, 0 when empty.
func xcInt(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return "ok 0"
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + strconv.FormatInt(n, 10)
}

// xcFloat is float_of's contract: Float.parse of the trimmed text, 0 when
// empty; the value's text is float_text's.
func xcFloat(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return "ok 0"
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + strconv.FormatFloat(f, 'g', -1, 64)
}

// xcBool is bool_of's contract: strconv.ParseBool of the trimmed text,
// false when empty.
func xcBool(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return "ok false"
	}
	b, err := strconv.ParseBool(t)
	if err != nil {
		return "err " + err.Error()
	}
	if b {
		return "ok true"
	}
	return "ok false"
}

// xcPiece is one Piece of a build row: kind 0 opens the parents, 1 is a
// leaf holding text, 3 character data and 4 a comment (kind 2, a nested
// element, has its own hand written rows).
type xcPiece struct {
	kind    int
	parents []string
	name    string
	text    string
}

// xcAttr is an attribute of a build row's root.
type xcAttr struct{ name, value string }

type xcRow struct {
	root  string
	attrs []xcAttr
	ps    []xcPiece
}

func xcParents(ps ...string) []string { return ps }

// xcRowExpr is the row as the corpus's Xo tuple.
func xcRowExpr(r xcRow) string {
	var as []string
	for _, a := range r.attrs {
		as = append(as, fmt.Sprintf("xmlcodec.attr(%s, %s)", xq(a.name), xq(a.value)))
	}
	var ps []string
	for _, p := range r.ps {
		par := "[" + strings.Join(xqList(p.parents), ", ") + "]"
		switch p.kind {
		case 0:
			ps = append(ps, "xmlcodec.open_path("+par+")")
		case 1:
			ps = append(ps, fmt.Sprintf("xmlcodec.leaf(%s, %s, %s)", par, xq(p.name), xq(p.text)))
		case 3:
			ps = append(ps, "xmlcodec.chardata("+xq(p.text)+")")
		case 4:
			ps = append(ps, "xmlcodec.comment("+xq(p.text)+")")
		}
	}
	return fmt.Sprintf("(%s, [%s], [%s]", xq(r.root), strings.Join(as, ", "), strings.Join(ps, ", "))
}

func xqList(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = xq(x)
	}
	return out
}

// xcBuild is build's contract, from Go's marshal.go printer: the parents
// shared with the open stack stay open, the rest close, then the new ones
// open, and the leaf is written. The answer is Go's token stream in the
// corpus's canonical text (xmlTokText), not its bytes: build's element is
// encoded without indent and read back, which leaves escaping and the
// shared-parents order as the things that must match (the derive corpus
// holds the byte-for-byte Marshal). Text escapes with EscapeText like
// Go's marshaler, so a newline is &#xA; there and a newline here; the
// decoded tokens are the same.
func xcBuild(r xcRow) string {
	root := xml.StartElement{Name: xml.Name{Local: r.root}}
	for _, a := range r.attrs {
		root.Attr = append(root.Attr, xml.Attr{Name: xml.Name{Local: a.name}, Value: a.value})
	}
	toks := []xml.Token{root}
	var open []string
	closeTo := func(n int) {
		for i := len(open) - 1; i >= n; i-- {
			toks = append(toks, xml.EndElement{Name: xml.Name{Local: open[i]}})
		}
		open = open[:n]
	}
	addText := func(text string) {
		// the decoder reads one text token per segment, and an empty one
		// leaves none
		if text == "" {
			return
		}
		if n := len(toks); n > 0 {
			if last, ok := toks[n-1].(xml.CharData); ok {
				toks[n-1] = xml.CharData(string(last) + text)
				return
			}
		}
		toks = append(toks, xml.CharData(text))
	}
	for _, p := range r.ps {
		switch p.kind {
		case 3:
			addText(p.text)
			continue
		case 4:
			toks = append(toks, xml.Comment(p.text))
			continue
		}
		keep := 0
		for keep < len(p.parents) && keep < len(open) && p.parents[keep] == open[keep] {
			keep++
		}
		closeTo(keep)
		for _, n := range p.parents[keep:] {
			toks = append(toks, xml.StartElement{Name: xml.Name{Local: n}})
			open = append(open, n)
		}
		if p.kind == 1 {
			toks = append(toks, xml.StartElement{Name: xml.Name{Local: p.name}})
			addText(p.text)
			toks = append(toks, xml.EndElement{Name: xml.Name{Local: p.name}})
		}
	}
	closeTo(0)
	toks = append(toks, xml.EndElement{Name: xml.Name{Local: r.root}})
	// an encoder error is the row's answer: the elements above are valid,
	// but a comment cannot hold "-->"
	enc := xml.NewEncoder(io.Discard)
	for _, t := range toks {
		if err := enc.EncodeToken(t); err != nil {
			return xmlErrText(err)
		}
	}
	var texts []string
	for _, t := range toks {
		texts = append(texts, xmlTokText(t))
	}
	return strings.Join(texts, " ")
}

func xcLeafText(r *rand.Rand) string {
	return []string{"", "hi", "a & b", "<x>", "é日", "\n  ", `a"b`, "]]>", "x\ty"}[r.Intn(9)]
}

func xcLeafName(r *rand.Rand) string {
	return []string{"v", "w", "text", "a.b-c", "é", "item"}[r.Intn(6)]
}

// xcRandRow is a random build row: parents picked from paths that share
// and drop prefixes, leaves, character data and comments between them.
func xcRandRow(r *rand.Rand) xcRow {
	row := xcRow{root: []string{"Root", "r", "a.b-c"}[r.Intn(3)]}
	for i, n := 0, r.Intn(3); i < n; i++ {
		row.attrs = append(row.attrs, xcAttr{
			name:  []string{"id", "class", "x"}[r.Intn(3)],
			value: []string{"7", "a&amp;b", "", `a"b`, "é"}[r.Intn(5)],
		})
	}
	paths := [][]string{nil, {"a"}, {"a", "b"}, {"a", "b", "c"}, {"x"}, {"x", "y"}, {"a", "z"}}
	for i, n := 0, 1+r.Intn(8); i < n; i++ {
		switch k := r.Intn(12); {
		case k < 5:
			row.ps = append(row.ps, xcPiece{kind: 0, parents: paths[r.Intn(len(paths))]})
		case k < 11:
			row.ps = append(row.ps, xcPiece{kind: 1, parents: paths[r.Intn(len(paths))],
				name: xcLeafName(r), text: xcLeafText(r)})
		case k == 11:
			if r.Intn(2) == 0 {
				row.ps = append(row.ps, xcPiece{kind: 3, text: xcLeafText(r)})
			} else {
				row.ps = append(row.ps, xcPiece{kind: 4, text: []string{"c", " ", "two words"}[r.Intn(3)]})
			}
		}
	}
	return row
}

// xcFixedRows are the hand written build rows: a comment the Encoder
// rejects, an attribute the Encoder escapes, a leaf reopened below a
// reused parent, and a chardata only element.
var xcFixedRows = []xcRow{
	{root: "r", attrs: []xcAttr{{"id", "7"}},
		ps: []xcPiece{{kind: 1, parents: xcParents("a"), name: "b", text: "t"}}},
	{root: "r", ps: []xcPiece{
		{kind: 3, text: "lead"},
		{kind: 1, parents: nil, name: "v", text: "x"},
		{kind: 3, text: "a & b"},
	}},
	{root: "r", ps: []xcPiece{{kind: 4, text: "-->"}}},
	{root: "r", ps: []xcPiece{{kind: 4, text: "two words"}}},
	{root: "r", attrs: []xcAttr{{"a", `x"y&z<`}, {"b", "é"}}, ps: []xcPiece{
		{kind: 1, parents: xcParents("p", "q"), name: "v", text: "<&>"},
		{kind: 1, parents: xcParents("p"), name: "w", text: ""},
		{kind: 1, parents: xcParents("p", "q"), name: "v", text: "again"},
	}},
	{root: "r", ps: []xcPiece{
		{kind: 0, parents: xcParents("a", "b")},
		{kind: 0, parents: xcParents("a")},
		{kind: 1, parents: xcParents("a", "b"), name: "v", text: "deep"},
	}},
}

// xcStrings are int_of, float_of and bool_of's inputs: the values Go
// accepts, the ones it rejects, and the edge shapes (signs, zeros,
// underscores, non-ASCII digits, spaces, the range limits, and the
// floating point forms: decimal, hexadecimal, inf and nan).
var xcStrings = []string{
	"", " ", "\t\n", "12", " 42 ", "-3", "+3", "007", "-0", "0",
	"1_000", "12a", "abc", "1.5", "0x10", "١٢",
	"9223372036854775807", "-9223372036854775808", "9223372036854775808",
	"-9223372036854775809", "99999999999999999999", "123456789012345678",
	"1e3", "0.1", ".5", "5.", "1e-400", "Inf", "NaN",
	"infinity", "+Inf", "-Inf", "0x1p-2", "1,5", "1 000",
	// float64's edges: just inside, just past, and the exponent and
	// mantissa forms that overflow
	"1e308", "1e309", "-1e309", " 1e309 ", "1.7976931348623157e308",
	"1.7976931348623159e308", "1e-309", "1e100000", "1e-100000",
	"0x1p1023", "0x1p1024", "0x1.fffffffffffff8p1023", strings.Repeat("9", 400),
	"0.1" + strings.Repeat("0", 320) + "1",
	"1e999999999999999999999", "-1e999999999999999999999",
	"1e-999999999999999999999", "0x1p99999999999999999999",
	// syntax Go rejects: a bad underscore, a missing exponent, a hex
	// float without p, and a special word with more after it
	"1__0", "1_", "_1", "1_.5", "1._5", "1_0.5", "1e", "1e+", "1e-",
	"1e_1", "1e1_", "1e1_0", "1E+3", "0x", "0x1", "0X1P", "0x1p", "0x1p+",
	"0x_1p0", "0x1.8p1", "0x1P-2", "0x1p1_0", "infinityy", "in", "nana",
	"-nan", "+nan", "1e309x", "++1", "1.2.3", "1e 3", "nan(1)",
	"true", "TRUE", " True ", "t", "T", "1", "0", "yes", "false", "F",
	"FALSE", "TRUEs", "2", " 42 ", "\r\n7\r\n", "１２",
}

// xcFloats are float_text's inputs: integral, fractional, negative,
// exponent, and the shortest-text edges.
var xcFloats = []float64{
	0, 1, -1, 1.5, 2.25, -0.5, 100, 1234567, 1e6, 1e21, 1e-7, 0.1,
	1.0 / 3.0, 123456789.5, 3, 2e-10, 1.7976931348623157e308, 5e-324,
	-0.0, 1e20, 1e-5, 42,
}

func xmlcodecCorpus() string {
	r := rand.New(rand.NewSource(21))
	var o strings.Builder
	o.WriteString(header("encoding/xml's Decoder token walk and Encoder, and strconv, as std/xmlcodec's helpers contract them: parse_root, attr_of, direct_text, build with open_path, leaf, chardata and comment, float_text, int_of, float_of and bool_of, on hand picked and seeded random documents, pieces and strings"))
	o.WriteString("use std/xml\nuse std/xmlcodec\n\n")
	o.WriteString("/// (document, parse_root's element encoded with two-space indent or err LINE MSG, attr_of id class lang x, direct_text, int_of, float_of and bool_of of that text)\nconst ROOT_DOCS: List[(Str, Str, Str, Str, Str)] = [\n")
	docs := append([]string{}, xcFixedDocs...)
	for i := 0; i < 500; i++ {
		docs = append(docs, xmlDoc(r))
	}
	for _, d := range docs {
		root, attrs, direct, parsed := xcRoot(d)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(d), xq(root), xq(attrs), xq(direct), xq(parsed))
	}
	o.WriteString("]\n\n/// (root, attributes, pieces, Go's token stream as the corpus's canonical text, or err LINE MSG when the Encoder rejects a comment)\nconst PIECE_ROWS: List[(Str, List[xml.Attr], List[xmlcodec.Piece], Str)] = [\n")
	rows := append([]xcRow{}, xcFixedRows...)
	for i := 0; i < 600; i++ {
		rows = append(rows, xcRandRow(r))
	}
	for _, row := range rows {
		fmt.Fprintf(&o, "  %s, %s),\n", xcRowExpr(row), xq(xcBuild(row)))
	}
	o.WriteString("]\n\n/// (float, Go's FormatFloat 'g' -1 64)\nconst FLOAT_TEXTS: List[(Float, Str)] = [\n")
	for _, f := range xcFloats {
		fmt.Fprintf(&o, "  (%s, %s),\n", xoFloat(f), xq(strconv.FormatFloat(f, 'g', -1, 64)))
	}
	o.WriteString("]\n\n/// (text, int_of, float_of and bool_of of it)\nconst PARSE_TEXTS: List[(Str, Str, Str, Str)] = [\n")
	for _, s := range xcStrings {
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(textOf(s)), xq(xcInt(s)), xq(xcFloat(s)), xq(xcBool(s)))
	}
	o.WriteString("]\n")
	o.WriteString(xmlcodecTests)
	return o.String()
}

const xmlcodecTests = `
fn first_bad3[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn err_text(e: xml.XmlErr) -> Str { "err ${e.line} ${e.msg}" }

fn show_opt(v: Str?) -> Str {
  match v {
    Some(s) => "some:${s}"
    None => "none"
  }
}

fn root_text(r: Result[xml.Element, xml.XmlErr]) -> Str {
  match r {
    Ok(e) => match e.encode(indent: "  ") {
      Ok(s) => "ok ${s}"
      Err(x) => err_text(x)
    }
    Err(x) => err_text(x)
  }
}

fn attr_report(e: xml.Element) -> Str {
  ["id", "class", "lang", "x"].map(fn(n) { "${n}=${show_opt(xmlcodec.attr_of(e, n))}" }).join(" ")
}

fn parse3(s: Str) -> Str {
  let i = match xmlcodec.int_of(s) {
    Ok(n) => "ok ${n}"
    Err(e) => "err ${e.msg}"
  }
  let f = match xmlcodec.float_of(s) {
    Ok(x) => "ok ${xmlcodec.float_text(x)}"
    Err(e) => "err ${e.msg}"
  }
  let b = match xmlcodec.bool_of(s) {
    Ok(v) => "ok ${v}"
    Err(e) => "err ${e.msg}"
  }
  "i=${i} f=${f} b=${b}"
}

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

fn built_text(e: xml.Element) -> Str {
  match e.encode() {
    Ok(s) => match xml.tokens(s) {
      Ok(ts) => ts.map(tok_text).join(" ")
      Err(x) => err_text(x)
    }
    Err(x) => err_text(x)
  }
}

test "parse_root, attr_of and direct_text are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (doc, want_root, _attrs, _direct, _parsed) in ROOT_DOCS {
    let r = xmlcodec.parse_root(doc)
    let got = root_text(r)
    if got != want_root {
      bad.push((doc, got, want_root))
      continue
    }
    match r {
      Ok(e) => {
        let got2 = attr_report(e)
        if got2 != _attrs {
          bad.push((doc, got2, _attrs))
        }
        let got3 = xmlcodec.direct_text(e)
        if got3 != _direct {
          bad.push((doc, got3, _direct))
        }
      }
      Err(..) => {}
    }
  }
  expect(first_bad3(bad)) == []
}

test "the text in numbers and booleans is read as Go's strconv does" {
  var bad: List[(Str, Str, Str)] = []
  for (doc, _want_root, _attrs, _direct, want) in ROOT_DOCS {
    match xmlcodec.parse_root(doc) {
      Ok(e) => {
        let got = parse3(xmlcodec.direct_text(e))
        if got != want {
          bad.push((doc, got, want))
        }
      }
      Err(..) => {
        if want != "" {
          bad.push((doc, "err", want))
        }
      }
    }
  }
  expect(first_bad3(bad)) == []
}

test "build with open_path, leaf, chardata and comment is Go's printer" {
  var bad: List[(Str, Str, Str)] = []
  for (name, attrs, pieces, want) in PIECE_ROWS {
    let got = built_text(xmlcodec.build(name, attrs, pieces))
    if got != want {
      bad.push((name, got, want))
    }
  }
  expect(first_bad3(bad)) == []
}

test "a Writer given the pieces writes the text build's tree encodes to" {
  var bad: List[(Str, Str, Str)] = []
  for (name, attrs, pieces, _want) in PIECE_ROWS {
    var comment = false
    for p in pieces {
      if p.kind == 4 {
        comment = true
      }
    }
    if comment {
      continue
    }
    for indent in ["", "  "] {
      let want = match xmlcodec.build(name, attrs, pieces).encode(indent) {
        Ok(s) => s
        Err(x) => err_text(x)
      }
      var w = xmlcodec.writer(indent)
      xmlcodec.write_start(w, name, attrs)
      for p in pieces {
        if p.kind == 0 {
          xmlcodec.write_open(w, p.parents)
        } else if p.kind == 1 {
          xmlcodec.write_leaf(w, p.parents, p.name, p.text)
        } else if p.kind == 3 {
          xmlcodec.write_chardata(w, p.text)
        }
      }
      xmlcodec.write_end(w, name)
      let got = xmlcodec.written(w)
      if got != want {
        bad.push((name, got, want))
      }
    }
    for p in pieces {
      var b = StrBuilder.new()
      xml.escape_to(b, p.text)
      if b.freeze() != xml.escape(p.text) {
        bad.push((name, "escape_to ${p.text}", "escape"))
      }
    }
  }
  expect(first_bad3(bad)) == []
}

test "float_text is Go's FormatFloat 'g' -1 64" {
  var bad: List[(Str, Str, Str)] = []
  for (f, want) in FLOAT_TEXTS {
    let got = xmlcodec.float_text(f)
    if got != want {
      bad.push(("${f}", got, want))
    }
  }
  expect(first_bad3(bad)) == []
}

test "int_of, float_of and bool_of are Go's strconv" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want_i, want_f, want_b) in PARSE_TEXTS {
    let i = match xmlcodec.int_of(s) {
      Ok(n) => "ok ${n}"
      Err(e) => "err ${e.msg}"
    }
    if i != want_i {
      bad.push((s, "int " + i, want_i))
    }
    let f = match xmlcodec.float_of(s) {
      Ok(x) => "ok ${xmlcodec.float_text(x)}"
      Err(e) => "err ${e.msg}"
    }
    if f != want_f {
      bad.push((s, "float " + f, want_f))
    }
    let b = match xmlcodec.bool_of(s) {
      Ok(v) => "ok ${v}"
      Err(e) => "err ${e.msg}"
    }
    if b != want_b {
      bad.push((s, "bool " + b, want_b))
    }
  }
  expect(first_bad3(bad)) == []
}
`
