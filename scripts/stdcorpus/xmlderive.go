package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
)

// The Go types of the derive Xml corpus (decision 0315): xmlDeriveXo's
// structs are the same types with the same tags, `xml "..."` strings
// holding Go's tag text.

type XInner struct {
	Name string `xml:"name"`
	N    int    `xml:"n,attr"`
}

type XBasic struct {
	ID    int      `xml:"id,attr"`
	Title string   `xml:"title"`
	Score float64  `xml:"score"`
	Ok    bool     `xml:"ok"`
	Note  *string  `xml:"note,omitempty"`
	Tags  []string `xml:"tags>tag"`
	Count int      `xml:"count,omitempty"`
	Lang  string   `xml:"lang,attr,omitempty"`
	Inner XInner   `xml:"inner"`
	Items []XInner `xml:"item"`
	Opt   *XInner  `xml:"opt"`
	Skip  string   `xml:"-"`
	Text  string   `xml:",chardata"`
}

// xoFloat is Xo's text of a Float: Go's shortest 'g' text, with ".0" when
// it reads as an integer.
func xoFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEIN") {
		s += ".0"
	}
	return s
}

// xmlWellFormed reports whether the Decoder reads every token of doc.
func xmlWellFormed(doc string) bool {
	d := xml.NewDecoder(strings.NewReader(doc))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
	}
}

func showInner(v XInner) string {
	return fmt.Sprintf("I{%s %d}", strconv.Quote(v.Name), v.N)
}

func showBasic(v XBasic) string {
	note := "nil"
	if v.Note != nil {
		note = strconv.Quote(*v.Note)
	}
	var tags []string
	for _, t := range v.Tags {
		tags = append(tags, strconv.Quote(t))
	}
	var items []string
	for _, t := range v.Items {
		items = append(items, showInner(t))
	}
	opt := "nil"
	if v.Opt != nil {
		opt = showInner(*v.Opt)
	}
	return fmt.Sprintf("B{id=%d title=%s score=%s ok=%t note=%s tags=[%s] count=%d lang=%s inner=%s items=[%s] opt=%s skip=%s text=%s}",
		v.ID, strconv.Quote(v.Title), xoFloat(v.Score), v.Ok, note, strings.Join(tags, ","), v.Count,
		strconv.Quote(v.Lang), showInner(v.Inner), strings.Join(items, ","), opt, strconv.Quote(v.Skip), strconv.Quote(v.Text))
}

func pick(r *rand.Rand, xs ...string) string { return xs[r.Intn(len(xs))] }

// xmlDeriveDoc writes one random document for XBasic: fields in a random
// order, repeated, missing, unknown, with comments, CDATA, and bad values.
func xmlDeriveDoc(r *rand.Rand) string {
	var b strings.Builder
	root := pick(r, "XBasic", "XBasic", "XBasic", "Other")
	b.WriteString("<" + root)
	if r.Intn(3) > 0 {
		b.WriteString(` id=` + pick(r, `"7"`, `"-3"`, `' 12 '`, `""`, `"abc"`, `"99999999999999999999"`, `"1.5"`, `"0"`))
	}
	if r.Intn(3) == 0 {
		b.WriteString(` lang=` + pick(r, `"en"`, `""`, `'fr'`, `"a&amp;b"`))
	}
	if r.Intn(6) == 0 {
		b.WriteString(` extra="x"`)
	}
	b.WriteString(">")
	var parts []string
	inner := func() string {
		var s strings.Builder
		s.WriteString("<inner")
		if r.Intn(4) > 0 {
			s.WriteString(` n=` + pick(r, `"1"`, `"22"`, `"x"`, `""`))
		}
		s.WriteString(">")
		if r.Intn(4) > 0 {
			s.WriteString("<name>" + pick(r, "ann", "bo&lt;b", "", "é日", " sp ") + "</name>")
		}
		s.WriteString("</inner>")
		return s.String()
	}
	for i, n := 0, r.Intn(12); i < n; i++ {
		switch r.Intn(14) {
		case 0:
			parts = append(parts, "<title>"+pick(r, "Hello", "a &amp; b", "", "<![CDATA[<raw>]]>", "x<!-- c -->y", "é", "t<b>nested</b>u")+"</title>")
		case 1:
			parts = append(parts, "<score>"+pick(r, "1.5", "2", "-0.25", "1e3", "abc", "", " 3 ", "0.1")+"</score>")
		case 2:
			parts = append(parts, "<ok>"+pick(r, "true", "false", "1", "0", "t", "", "yes", " TRUE ")+"</ok>")
		case 3:
			parts = append(parts, "<note>"+pick(r, "n", "", "a&lt;b")+"</note>")
		case 4:
			parts = append(parts, "<tags>"+strings.Repeat("<tag>"+pick(r, "x", "y", "", "z&amp;")+"</tag>", r.Intn(4))+"</tags>")
		case 5:
			parts = append(parts, "<count>"+pick(r, "5", "0", "-1", "z", "")+"</count>")
		case 6:
			parts = append(parts, inner())
		case 7:
			parts = append(parts, strings.Replace(inner(), "inner", "item", 2))
		case 8:
			parts = append(parts, strings.Replace(inner(), "inner", "opt", 2))
		case 9:
			parts = append(parts, "<skip>ignored</skip>")
		case 10:
			parts = append(parts, pick(r, "text", " ", "\n  ", "a &amp; b", "<![CDATA[cd]]>"))
		case 11:
			parts = append(parts, pick(r, "<!-- c -->", "<?pi x?>", "<unknown a=\"1\"><deep/></unknown>"))
		case 12:
			parts = append(parts, "<tags><tag>"+pick(r, "p", "q")+"</tag></tags>")
		default:
			parts = append(parts, "<title/>")
		}
	}
	for _, p := range parts {
		b.WriteString(p)
	}
	if r.Intn(30) > 0 {
		b.WriteString("</" + root + ">")
	}
	s := b.String()
	if r.Intn(25) == 0 {
		s = s[:r.Intn(len(s))]
	}
	return textOf(s)
}

// xmlDeriveCase is a document's Go results: Unmarshal's value (or error),
// then Marshal and MarshalIndent of that value.
func xmlDeriveCase(doc string) (string, string, string) {
	var v XBasic
	if err := xml.Unmarshal([]byte(doc), &v); err != nil {
		return "err " + textOf(err.Error()), "", ""
	}
	m, err := xml.Marshal(v)
	if err != nil {
		return "ok " + showBasic(v), "err " + textOf(err.Error()), ""
	}
	mi, _ := xml.MarshalIndent(v, "", "  ")
	return "ok " + textOf(showBasic(v)), textOf(string(m)), textOf(string(mi))
}

const xmlDeriveXo = `
struct XInner {
  name: Str
  n xml "n,attr": Int
} derive Xml

struct XBasic {
  id xml "id,attr": Int
  title: Str
  score: Float
  ok: Bool
  note xml "note,omitempty": Str?
  tags xml "tags>tag": List[Str]
  count xml "count,omitempty": Int
  lang xml "lang,attr,omitempty": Str
  inner: XInner
  items xml "item": List[XInner]
  opt: XInner?
  skip xml "-": Str
  text xml ",chardata": Str
} derive Xml

fn show_inner(v: XInner) -> Str { "I{${v.name.quote()} ${v.n}}" }

fn show_basic(v: XBasic) -> Str {
  let note = match v.note {
    Some(s) => s.quote()
    None => "nil"
  }
  let opt = match v.opt {
    Some(i) => show_inner(i)
    None => "nil"
  }
  let tags = v.tags.map(fn(t) { t.quote() }).join(",")
  let items = v.items.map(show_inner).join(",")
  "B{id=${v.id} title=${v.title.quote()} score=${v.score} ok=${v.ok} note=${note} tags=[${tags}] count=${v.count} lang=${v.lang.quote()} inner=${show_inner(v.inner)} items=[${items}] opt=${opt} skip=${v.skip.quote()} text=${v.text.quote()}}"
}

test "decode and encode_value of a derived struct are Go's Unmarshal and Marshal" {
  var bad: List[(Str, Str, Str)] = []
  for (doc, want, want_m, want_i) in DERIVE_DOCS {
    match xml.decode[XBasic](doc) {
      Ok(v) => {
        let got = "ok ${show_basic(v)}"
        if got != want {
          bad.push((doc, got, want))
          continue
        }
        let m = xml.encode_value(v).ok() ?? "err"
        if m != want_m {
          bad.push((doc, m, want_m))
        }
        let mi = xml.encode_value(v, indent: "  ").ok() ?? "err"
        if mi != want_i {
          bad.push((doc, mi, want_i))
        }
      }
      Err(e) => {
        let got = "err ${e}"
        if got != want {
          bad.push((doc, got, want))
        }
      }
    }
  }
  expect(first_bad(bad)) == []
}
`

func xmlDeriveCorpus(o *strings.Builder) {
	r := rand.New(rand.NewSource(15))
	o.WriteString("\n/// (document, Go's Unmarshal into XBasic as `ok VALUE` or `err TEXT`, its Marshal, its MarshalIndent two spaces)\nconst DERIVE_DOCS: List[(Str, Str, Str, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		d := xmlDeriveDoc(r)
		u, m, mi := xmlDeriveCase(d)
		// Go decodes while it reads, so a value error before a later
		// syntax error wins there; std reads the document first, and the
		// syntax error wins (spec std 34): such documents stay out.
		if strings.HasPrefix(u, "err ") && !strings.HasPrefix(u, "err XML syntax error") && !xmlWellFormed(d) {
			i--
			continue
		}
		fmt.Fprintf(o, "  (%s, %s, %s, %s),\n", xq(d), xq(u), xq(m), xq(mi))
	}
	o.WriteString("]\n")
	o.WriteString(xmlDeriveXo)
}
