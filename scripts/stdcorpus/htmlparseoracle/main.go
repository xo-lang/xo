// Command htmlparseoracle prints the std/htmlparse corpus: golang.org/x/net
// v0.62.0's html.Parse and html.ParseFragment (std/htmlparse's reference) on
// the html5lib tree-construction tests that ship with x/net, Go's own
// parser test data, and seeded random documents built from markup pieces.
// scripts/stdcorpus runs it; it is its own module so the repository's go.mod
// has no third-party requirement.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// A testCase is one input: the text, the fragment context (namespace and
// tag, empty for a document) and the scripting flag.
type testCase struct {
	name      string
	text      string
	ns, tag   string
	scripting bool
}

func readParseTest(r *bufio.Reader) (*testCase, error) {
	ta := &testCase{scripting: true}
	line, err := r.ReadSlice('\n')
	if err != nil {
		return nil, err
	}
	var b []byte
	if string(line) != "#data\n" {
		return nil, fmt.Errorf(`got %q want "#data\n"`, line)
	}
	for {
		line, err = r.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		if line[0] == '#' {
			break
		}
		b = append(b, line...)
	}
	ta.text = strings.TrimSuffix(string(b), "\n")
	if string(line) != "#errors\n" {
		return nil, fmt.Errorf(`got %q want "#errors\n"`, line)
	}
	for {
		line, err = r.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		if line[0] == '#' {
			break
		}
	}
	if string(line) == "#new-errors\n" {
		for {
			line, err = r.ReadSlice('\n')
			if err != nil {
				return nil, err
			}
			if line[0] == '#' {
				break
			}
		}
	}
	if ls := string(line); strings.HasPrefix(ls, "#script-") {
		switch {
		case strings.HasSuffix(ls, "-on\n"):
			ta.scripting = true
		case strings.HasSuffix(ls, "-off\n"):
			ta.scripting = false
		default:
			return nil, fmt.Errorf(`got %q, want "#script-on" or "#script-off"`, line)
		}
		for {
			line, err = r.ReadSlice('\n')
			if err != nil {
				return nil, err
			}
			if line[0] == '#' {
				break
			}
		}
	}
	if string(line) == "#document-fragment\n" {
		line, err = r.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		ctx := strings.TrimSpace(string(line))
		if i := strings.IndexByte(ctx, ' '); i >= 0 {
			ta.ns, ctx = ctx[:i], ctx[i+1:]
		}
		ta.tag = ctx
		line, err = r.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
	}
	if string(line) != "#document\n" {
		return nil, fmt.Errorf(`got %q want "#document\n"`, line)
	}
	inQuote := false
	for {
		line, err = r.ReadSlice('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		trimmed := bytes.Trim(line, "| \n")
		if len(trimmed) > 0 {
			if line[0] == '|' && trimmed[0] == '"' {
				inQuote = true
			}
			if trimmed[len(trimmed)-1] == '"' && !(line[0] == '|' && len(trimmed) == 1) {
				inQuote = false
			}
		}
		if len(line) == 0 || len(line) == 1 && line[0] == '\n' && !inQuote {
			break
		}
	}
	return ta, nil
}

func dumpIndent(w io.Writer, level int) {
	io.WriteString(w, "| ")
	for i := 0; i < level; i++ {
		io.WriteString(w, "  ")
	}
}

type sortedAttributes []html.Attribute

func (a sortedAttributes) Len() int           { return len(a) }
func (a sortedAttributes) Less(i, j int) bool { return a[i].Key < a[j].Key }
func (a sortedAttributes) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func dumpLevel(w io.Writer, n *html.Node, level int) error {
	dumpIndent(w, level)
	level++
	switch n.Type {
	case html.ElementNode:
		if n.Namespace != "" {
			fmt.Fprintf(w, "<%s %s>", n.Namespace, n.Data)
		} else {
			fmt.Fprintf(w, "<%s>", n.Data)
		}
		attr := sortedAttributes(append([]html.Attribute(nil), n.Attr...))
		sort.Stable(attr)
		for _, a := range attr {
			io.WriteString(w, "\n")
			dumpIndent(w, level)
			if a.Namespace != "" {
				fmt.Fprintf(w, `%s %s="%s"`, a.Namespace, a.Key, a.Val)
			} else {
				fmt.Fprintf(w, `%s="%s"`, a.Key, a.Val)
			}
		}
		if n.Namespace == "" && n.DataAtom == atom.Template {
			io.WriteString(w, "\n")
			dumpIndent(w, level)
			level++
			io.WriteString(w, "content")
		}
	case html.TextNode:
		fmt.Fprintf(w, `"%s"`, n.Data)
	case html.CommentNode:
		fmt.Fprintf(w, "<!-- %s -->", n.Data)
	case html.DoctypeNode:
		fmt.Fprintf(w, "<!DOCTYPE %s", n.Data)
		if n.Attr != nil {
			var p, s string
			for _, a := range n.Attr {
				switch a.Key {
				case "public":
					p = a.Val
				case "system":
					s = a.Val
				}
			}
			if p != "" || s != "" {
				fmt.Fprintf(w, ` "%s"`, p)
				fmt.Fprintf(w, ` "%s"`, s)
			}
		}
		io.WriteString(w, ">")
	default:
		return errors.New("unexpected node type")
	}
	io.WriteString(w, "\n")
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := dumpLevel(w, c, level); err != nil {
			return err
		}
	}
	return nil
}

func dump(n *html.Node) (string, error) {
	var b bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := dumpLevel(&b, c, 0); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// parse runs one case and returns the dump, or "ERR " and the error text.
func parse(tc *testCase) (out string) {
	defer func() {
		if x := recover(); x != nil {
			out = fmt.Sprintf("ERR %v", x)
		}
	}()
	opt := html.ParseOptionEnableScripting(tc.scripting)
	var doc *html.Node
	if tc.tag == "" {
		d, err := html.ParseWithOptions(strings.NewReader(tc.text), opt)
		if err != nil {
			return "ERR " + err.Error()
		}
		doc = d
	} else {
		ctx := &html.Node{Data: tc.tag, DataAtom: atom.Lookup([]byte(tc.tag)), Namespace: tc.ns, Type: html.ElementNode}
		nodes, err := html.ParseFragmentWithOptions(strings.NewReader(tc.text), ctx, opt)
		if err != nil {
			return "ERR " + err.Error()
		}
		doc = &html.Node{Type: html.DocumentNode}
		for _, n := range nodes {
			doc.AppendChild(n)
		}
	}
	s, err := dump(doc)
	if err != nil {
		return "ERR " + err.Error()
	}
	return s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// sourceCases reads the html5lib and Go tree-construction files of x/net.
func sourceCases() []*testCase {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "golang.org/x/net").Output()
	if err != nil {
		panic(err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "html", "testdata")
	var all []*testCase
	for _, sub := range []string{"html5lib-tests/tree-construction", "go"} {
		files, _ := filepath.Glob(filepath.Join(dir, sub, "*.dat"))
		sort.Strings(files)
		for _, f := range files {
			fh, err := os.Open(f)
			if err != nil {
				panic(err)
			}
			r := bufio.NewReader(fh)
			for i := 0; ; i++ {
				ta, err := readParseTest(r)
				if err == io.EOF {
					break
				}
				if err != nil {
					panic(f + ": " + err.Error())
				}
				ta.name = fmt.Sprintf("%s/%s/%d", sub, filepath.Base(f), i)
				all = append(all, ta)
			}
			fh.Close()
		}
	}
	return all
}

// q renders s with the escapes the Xo side of the corpus writes: backslash, quote, dollar, newline, carriage
// return, tab and the other controls as \xHH.
func q(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '$':
			b.WriteString(`\$`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// renderDoc is Render of the parse of text (scripting on), or "ERR " and the error.
func renderDoc(text string) (out string) {
	defer func() {
		if x := recover(); x != nil {
			out = fmt.Sprintf("ERR %v", x)
		}
	}()
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return "ERR " + err.Error()
	}
	var b bytes.Buffer
	if err := html.Render(&b, doc); err != nil {
		return "ERR " + err.Error()
	}
	return b.String()
}

var curated = []string{
	"", "x", " ", "<!DOCTYPE html>", "<!doctype html><html><head><title>t</title></head><body>b</body></html>",
	"<p>a<p>b", "<p>a<div>b</div>c", "<b>1<p>2</b>3</p>", "<a>1<a>2</a>3</a>", "<b><i>1</b>2</i>3", "<b><b><b><b>x", "<nobr>1<nobr>2",
	"<table><tr><td>1<td>2<tr><td>3</table>", "<table><b>x<tr><td>y</table>", "<table>text<tr>x</table>", "<table><caption>c<td>x</table>",
	"<table><colgroup><col><col></colgroup><tbody><tr><th>h</th></tr></tbody></table>", "<table><form><input type=hidden><input></table>",
	"<ul><li>1<li>2<ul><li>3</ul><li>4</ul>", "<dl><dt>a<dd>b<dt>c<dd>d</dl>", "<select><option>1<option>2<optgroup><option>3</select>",
	"<select><select><option>x", "<form><form><p>x</form>", "<h1>a<h2>b</h2>c</h1>", "<pre>\nx\n</pre><pre>\n\nx", "<textarea>\nx</textarea>", "<listing>\r\nx",
	"<svg><circle r=1 /><foreignObject><p>x</foreignObject></svg>", "<svg viewbox=0><clippath><path d=1></clippath></svg>", "<math><mi>x</mi><mtext><b>y</b></mtext><annotation-xml encoding=text/html><p>z</annotation-xml></math>",
	"<svg><title><b>x</b></title></svg>", "<svg><p>x</p></svg>", "<svg><font color=red>x</font></svg>", "<math><svg><desc><p>x", "<svg xlink:href=a xml:lang=b xmlns:xlink=c>",
	"<frameset><frame><frameset><frame></frameset></frameset>", "<html><frameset></frameset></html>x", "<head><frameset>", "<body><frameset>", "<p>a<frameset>",
	"<template><tr><td>x</template>", "<template><template><p>x</template></template>", "<template></template><template>", "<table><template><td>x</template></table>",
	"<noscript><p>x</noscript>", "<head><noscript><p>x</noscript>", "<iframe><p>x</iframe>", "<xmp><b></xmp>", "<plaintext><b>x", "<noembed><b>", "<noframes><b>",
	"<!--a--><!--b-->x<!--c-->", "<html><!--a--></html><!--b-->", "<body></body><!--c-->", "<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01//EN\" \"http://x\">",
	"<!DOCTYPE html PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\"><table>", "<!DOCTYPE foo><p>", "<!DOCTYPE html SYSTEM 'about:legacy-compat'>",
	"<a b=1 b=2 c>", "<img/>x<br/>", "<hr/><input/><area/>", "<image src=a>", "<p/>x</p/>", "</p>", "</br>", "</html>x", "</body>x", "</div>x</span>",
	"a&amp;b&lt;c&notit;d&#65;e&#x42;f&copy", "<p title=\"&quot;&amp;\">x", "\x00a<p>\x00b", "<svg>\x00x</svg>", "<table>\x00<tr>",
	"<button><button>x", "<applet><p>x</applet>y", "<marquee><b>x</marquee>y</b>", "<object><param><b>x</object>y</b>", "<ruby><rb>a<rt>b<rp>c</ruby>",
	"<font><font><font><font>x", "<font color=r><p>x", "<em><tr><td>x", "<big><center>x</big>y</center>",
	"<p><b>x</p>y", "<div><b>x</div>y", "<b><p><i>x</b>y</p>z</i>", "<a><table><a>x</table>",
	"<body onload=1><body id=2>x", "<html a=1><html b=2><html a=3>", "<head><head><title>a</title>", "<body><head>", "<html><head></head><body></body></html><p>x",
	"<main>x</main><search>y</search><dialog>z</dialog>", "<menuitem>x", "<isindex>x", "<keygen>x", "<command>x", "<embed>x</embed>", "<wbr>x",
	"<option>x", "<optgroup>x", "<col>", "<tr>", "<td>", "<caption>", "<tbody>", "<thead><tr>", "<th>x</th>",
	"é<b>ü</b>日本", "<日本 x=é>", "<a\x00b>", "<x-y z-w=1>q</x-y>", "<A HREF=x>B</A>", "<P CLASS='a'>x",
	"<math><mi><svg><p>x", "<svg><foreignobject><p>x", "<select><svg>x", "<table><svg><circle>x",
	"<math><template><mo><template>", "<template><svg><foo><template><foreignObject><div></template><div>",
	"<a><b><i><u><s><em><strong><small><big><font><tt><code>x</a>y",
	strings.Repeat("<div>", 600), strings.Repeat("<b>", 600), strings.Repeat("<table><tr><td>", 200),
	strings.Repeat("<div>", 511) + "x", strings.Repeat("<div>", 510) + "<p>x",
}

var pieces = []string{
	"<html>", "</html>", "<head>", "</head>", "<body>", "</body>", "<p>", "</p>", "<div>", "</div>", "<span>", "</span>", "<a href=x>", "</a>", "<b>", "</b>", "<i>", "</i>", "<em>", "</em>", "<font color=r>", "</font>",
	"<nobr>", "</nobr>", "<u>", "</u>", "<table>", "</table>", "<tbody>", "</tbody>", "<tr>", "</tr>", "<td>", "</td>", "<th>", "</th>", "<caption>", "</caption>", "<colgroup>", "<col>", "</colgroup>",
	"<select>", "</select>", "<option>", "<optgroup>", "<form>", "</form>", "<input>", "<input type=hidden>", "<button>", "</button>", "<ul>", "</ul>", "<ol>", "<li>", "</li>", "<dl>", "<dt>", "<dd>",
	"<h1>", "</h1>", "<h2>", "<pre>", "</pre>", "<textarea>", "</textarea>", "<script>", "</script>", "<style>", "</style>", "<title>", "</title>", "<template>", "</template>",
	"<svg>", "</svg>", "<math>", "</math>", "<foreignObject>", "</foreignObject>", "<desc>", "<mi>", "<mtext>", "<annotation-xml encoding=text/html>", "<circle r=1/>", "<path d=x>",
	"<frameset>", "</frameset>", "<frame>", "<noscript>", "</noscript>", "<iframe>", "</iframe>", "<plaintext>", "<xmp>", "<br>", "<hr>", "<img src=a>", "<object>", "</object>", "<marquee>", "<applet>", "<ruby>", "<rt>",
	"text", " ", "\n", "&amp;", "&lt;", "&", "\x00", "é", "<!-- c -->", "<!--", "-->", "<!DOCTYPE html>", "<!DOCTYPE>", "<![CDATA[x]]>", "<?pi?>", "<", ">", "/>", "<x-y>", "</x-y>", "<main>", "<center>", "<big>", "<table><tr><td>",
}

// randomCases builds seeded documents from the pieces, each parsed as a document, and some as a fragment.
func randomCases(n int) []*testCase {
	r := rand.New(rand.NewSource(570))
	ctxs := [][2]string{{"", "div"}, {"", "table"}, {"", "tr"}, {"", "select"}, {"", "template"}, {"", "body"}, {"", "html"}, {"", "colgroup"}, {"svg", "svg"}, {"math", "mi"}, {"", "title"}, {"", "textarea"}, {"", "script"}, {"", "form"}}
	var out []*testCase
	for i := 0; i < n; i++ {
		var b strings.Builder
		for j, k := 0, 1+r.Intn(14); j < k; j++ {
			b.WriteString(pieces[r.Intn(len(pieces))])
		}
		tc := &testCase{name: fmt.Sprintf("random/%d", i), text: b.String(), scripting: r.Intn(5) != 0}
		if r.Intn(5) == 0 {
			c := ctxs[r.Intn(len(ctxs))]
			tc.ns, tc.tag = c[0], c[1]
		}
		out = append(out, tc)
	}
	return out
}

func xoCorpus(goVersion string) string {
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from golang.org/x/net v0.62.0's html.Parse, ParseFragment and Render (" + goVersion + ") on its html5lib and Go tree-construction test files, curated documents and seeded random documents.\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString("use std/htmlparse\n\n")
	o.WriteString("/// (input, context namespace, context tag, scripting, Go's tree dump or \"ERR \" and its error)\nconst CASES: List[(Str, Str, Str, Bool, Str)] = [\n")
	n := 0
	for _, tc := range sourceCases() {
		fmt.Fprintf(&o, "  (\"%s\", \"%s\", \"%s\", %v, \"%s\"),\n", q(tc.text), q(tc.ns), q(tc.tag), tc.scripting, q(parse(tc)))
		n++
	}
	var docs []*testCase
	for _, in := range curated {
		docs = append(docs, &testCase{text: in, scripting: true})
	}
	docs = append(docs, randomCases(1200)...)
	for _, tc := range docs {
		fmt.Fprintf(&o, "  (\"%s\", \"%s\", \"%s\", %v, \"%s\"),\n", q(tc.text), q(tc.ns), q(tc.tag), tc.scripting, q(parse(tc)))
		n++
	}
	o.WriteString("]\n\n")
	fmt.Fprintf(&o, "const COUNT: Int = %d\n\n", n)
	o.WriteString("/// (input, Go's Render of html.Parse(input), or \"ERR \" and its error)\nconst RENDERS: List[(Str, Str)] = [\n")
	m := 0
	for _, tc := range docs {
		if tc.tag != "" || !tc.scripting {
			continue
		}
		fmt.Fprintf(&o, "  (\"%s\", \"%s\"),\n", q(tc.text), q(renderDoc(tc.text)))
		m++
	}
	o.WriteString("]\n\n")
	fmt.Fprintf(&o, "const RENDER_COUNT: Int = %d\n", m)
	o.WriteString(xoTests)
	return o.String()
}

const xoTests = `
/// The escapes of the oracle's q: backslash, quote, dollar, newline, carriage return, tab, controls as \\xHH.
fn q(s: Str) -> Str {
  var o = BytesBuilder.new()
  let hex = "0123456789abcdef".bytes()
  for c in s.bytes() {
    if c == 92 {
      o.write_str("\\\\")
    } else if c == 10 {
      o.write_str("\\n")
    } else if c == 13 {
      o.write_str("\\r")
    } else if c == 9 {
      o.write_str("\\t")
    } else if c == 34 {
      o.write_str("\\\"")
    } else if c == 36 {
      o.write_str("\\$")
    } else if c < 32 || c == 127 {
      o.write_str("\\x")
      o.write_byte(hex[Int(c) / 16])
      o.write_byte(hex[Int(c) % 16])
    } else {
      o.write_byte(c)
    }
  }
  o.freeze().to_str_lossy()
}

fn dump_case(input: Str, ns: Str, tag: Str, scripting: Bool) -> Str {
  let r = if tag == "" {
    htmlparse.parse_with(input, scripting)
  } else if scripting {
    htmlparse.parse_fragment(input, ns, tag)
  } else {
    htmlparse.parse_fragment_with(input, ns, tag, scripting)
  }
  match r {
    Ok(t) => t.dump()
    Err(e) => "ERR ${e.display()}"
  }
}

test "tree dumps equal Go's x/net/html" {
  for (input, ns, tag, scripting, want) in CASES {
    let got = dump_case(input, ns, tag, scripting)
    if got != want {
      // the first difference shows in the failure
      expect("input ${q(input)} context ${ns} ${tag} scripting ${scripting}\\n${got}") == "input ${q(input)} context ${ns} ${tag} scripting ${scripting}\\n${want}"
    }
  }
}

test "render equals Go's x/net/html Render" {
  for (input, want) in RENDERS {
    var got = ""
    match htmlparse.parse(input) {
      Ok(t) => {
        match t.render_node(t.root) {
          Ok(s) => got = s
          Err(e) => got = "ERR ${e.display()}"
        }
      }
      Err(e) => got = "ERR ${e.display()}"
    }
    if got != want {
      expect("input ${q(input)}\\n${got}") == "input ${q(input)}\\n${want}"
    }
  }
}

test "the corpus has its cases" {
  expect(CASES.len()) == COUNT
  expect(RENDERS.len()) == RENDER_COUNT
}
`

func main() {
	if len(os.Args) >= 2 && os.Args[1] != "files" {
		os.Stdout.WriteString(xoCorpus(os.Args[1]))
		return
	}
	cases := sourceCases()
	if len(os.Args) >= 4 && os.Args[1] == "files" {
		cf, _ := os.Create(os.Args[2])
		ef, _ := os.Create(os.Args[3])
		for _, tc := range cases {
			sc := "0"
			if tc.scripting {
				sc = "1"
			}
			fmt.Fprintf(cf, "%s %s %s %d\n%s\n", sc, dash(tc.ns), dash(tc.tag), len(tc.text), tc.text)
			s := parse(tc)
			fmt.Fprintf(ef, "%d\n%s\n", len(s), s)
		}
		cf.Close()
		ef.Close()
		fmt.Fprintln(os.Stderr, len(cases), "cases")
		return
	}
	fmt.Fprintln(os.Stderr, "usage: htmlparseoracle files CASES EXPECT")
	os.Exit(2)
}
