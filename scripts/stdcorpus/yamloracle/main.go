// Command yamloracle prints the std/yaml typed decode and encode corpus:
// gopkg.in/yaml.v3 v3.0.1 (std/yaml's reference) decoding the inputs of cases/
// (hand written typed cases and the documents of the yaml-test-suite) into one
// struct, and encoding a struct with every kind of field. scripts/stdcorpus
// runs it; it is its own module so the repository's go.mod has no third-party
// requirement.
package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed cases/*.yaml
var cases embed.FS

type Inner struct {
	X int `yaml:"x"`
}

type T struct {
	N  int               `yaml:"n"`
	F  float64           `yaml:"f"`
	S  string            `yaml:"s"`
	B  bool              `yaml:"b"`
	O  *string           `yaml:"o"`
	L  []int             `yaml:"l"`
	M  map[string]string `yaml:"m"`
	In Inner             `yaml:"in"`
}

type Item struct {
	N    int      `yaml:"n"`
	Tags []string `yaml:"tags"`
}

type Cfg struct {
	Name       string         `yaml:"name"`
	APIVersion string         `yaml:"apiVersion"`
	Port       int            `yaml:"port"`
	Ratio      float64        `yaml:"ratio"`
	On         bool           `yaml:"on"`
	O          *string        `yaml:"o"`
	None       *string        `yaml:"none"`
	Items      []Item         `yaml:"items"`
	M          map[string]int `yaml:"m"`
	Empty      []int          `yaml:"empty"`
	Text       string         `yaml:"text"`
	Yes        string         `yaml:"yes"`
	Num        string         `yaml:"num"`
	Inner      Item           `yaml:"inner"`
}

type P struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
}

type Opts struct {
	A string         `yaml:"a,omitempty"`
	B int            `yaml:"b,omitempty"`
	C []int          `yaml:"c,omitempty"`
	D []int          `yaml:"d,flow"`
	E map[string]int `yaml:"e,flow"`
	F P              `yaml:"f,flow"`
	G []string       `yaml:"g,omitempty,flow"`
	H *string        `yaml:"h,omitempty"`
	I []P            `yaml:"i,flow"`
	J string         `yaml:"j,flow"`
	K bool           `yaml:"k,omitempty"`
	L float64        `yaml:"l,omitempty"`
	M []int          `yaml:"m"`
}

type Deep struct {
	D [][][][]string `yaml:"d"`
}

// deepShow decodes into Deep and prints the count of leaves, or the error.
func deepShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var d Deep
	if err := yaml.Unmarshal([]byte(in), &d); err != nil {
		return "ERR " + strconv.Quote(err.Error())
	}
	n := 0
	for _, a := range d.D {
		for _, b := range a {
			for _, c := range b {
				n += len(c)
			}
		}
	}
	return fmt.Sprintf("leaves=%d", n)
}

// deepCases are the alias expansion documents: yaml.v3 refuses a document whose
// aliases expand into too much (the "billion laughs" shape) and an anchor that
// contains itself.
func deepCases() []string {
	row := func(item string, n int) string {
		xs := make([]string, n)
		for i := range xs {
			xs[i] = item
		}
		return strings.Join(xs, ", ")
	}
	var out []string
	for _, k := range []int{2, 3, 5, 9, 12} {
		out = append(out, "d: [[[ &a ["+row("x", k)+"] ]]]\n")
		out = append(out, "d: &d [ &c [ &b [ &a ["+row("x", k)+"], "+row("*a", k)+" ], "+row("*b", k)+" ], "+row("*c", k)+" ]\n")
		out = append(out, "d: [ [ &b [ &a ["+row("x", k)+"], "+row("*a", k)+" ], "+row("*b", k)+" ] ]\n")
	}
	out = append(out, "d: &a [*a]\n", "d: [&a [[*a]]]\n", "d: [[[ [x] ]]]\nm: &a {z: *a}\n", "d: *a\n", "x: &a [y]\nd: [[[*a, *a]], [[*a]]]\n")
	return out
}

type Meta struct {
	Name string   `yaml:"name"`
	Tags []string `yaml:"tags"`
}

type Base struct {
	ID   int  `yaml:"id"`
	Meta Meta `yaml:"meta"`
}

type Top struct {
	Base   `yaml:",inline"`
	Extra  string `yaml:"extra"`
	Hidden int    `yaml:"-"`
	N      int    `yaml:"n"`
}

// inlineShow decodes into Top and prints it, or the error.
func inlineShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var t Top
	if err := yaml.Unmarshal([]byte(in), &t); err != nil {
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	return fmt.Sprintf("id=%d meta=%s/%s extra=%s hidden=%d n=%d", t.ID, t.Meta.Name, strings.Join(t.Meta.Tags, ","), t.Extra, t.Hidden, t.N)
}

// strictShow is inlineShow with KnownFields(true).
func strictShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var t Top
	dec := yaml.NewDecoder(strings.NewReader(in))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		if err == io.EOF {
			return "id=0 meta=/ extra= hidden=0 n=0"
		}
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	return fmt.Sprintf("id=%d meta=%s/%s extra=%s hidden=%d n=%d", t.ID, t.Meta.Name, strings.Join(t.Meta.Tags, ","), t.Extra, t.Hidden, t.N)
}

var strictCases = []string{
	"id: 5\nextra: q\nn: 8\n",
	"id: 5\nunknown: 1\nn: 8\nother: [1]\n",
	"hidden: 4\nid: 1\n",
	"meta: {name: z, nope: 1}\n",
	"x: 1\nid: bad\ny: 2\n",
	"{id: 1, zz: 2}",
	"",
}

var inlineCases = []string{
	"id: 5\nextra: q\nhidden: 4\nn: 8\nmeta: {name: z, tags: [p, q]}\nunknown: 1\n",
	"id: x\nn: y\nmeta: {name: 1}\n",
	"n: 1\nid: 2\nid: 3\n",
	"meta: [1]\nextra: [2]\n",
	"{id: 1, meta: {tags: [a, b, c]}, extra: e}",
	"",
	"id: ~\nmeta: ~\nn: ~\n",
}

// xq is s as an Xo string literal.
func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func fl(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// norm names the Go types in an error as Xo does.
func norm(s string) string {
	s = strings.ReplaceAll(s, "main.", "")
	s = strings.ReplaceAll(s, "[]int", "List[Int]")
	return strings.ReplaceAll(s, "map[string]string", "Map[Str, Str]")
}

func show(in []byte) string {
	var t T
	if err := yaml.Unmarshal(in, &t); err != nil {
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	return fmtT(t)
}

func fmtT(t T) string {
	o := "nil"
	if t.O != nil {
		o = strconv.Quote(*t.O)
	}
	var ls []string
	for _, x := range t.L {
		ls = append(ls, strconv.Itoa(x))
	}
	var ks []string
	for k := range t.M {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var ms []string
	for _, k := range ks {
		ms = append(ms, strconv.Quote(k)+"="+strconv.Quote(t.M[k]))
	}
	return fmt.Sprintf("n=%d f=%s s=%s b=%v o=%s l=[%s] m={%s} in=%d", t.N, fl(t.F), strconv.Quote(t.S), t.B, o, strings.Join(ls, ","), strings.Join(ms, ","), t.In.X)
}

// allShow decodes every document (Decoder.Decode until io.EOF), the values joined by " | ".
func allShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	dec := yaml.NewDecoder(strings.NewReader(in))
	var parts []string
	for {
		var t T
		err := dec.Decode(&t)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "ERR " + strconv.Quote(norm(err.Error()))
		}
		parts = append(parts, fmtT(t))
	}
	return fmt.Sprintf("%d docs: %s", len(parts), strings.Join(parts, " | "))
}

var allCases = []string{
	"n: 1\n---\nn: 2\n...\n---\ns: three\n",
	"",
	"---\n",
	"n: 1\n---\n---\nn: 3\n",
	"n: 1\n---\nn: bad\n---\nn: 3\n",
	"n: 1\n---\n: : :\n",
	"%YAML 1.1\n---\nn: 4\n...\n%YAML 1.1\n---\nn: 5\n",
	"- 1\n",
}

func marshalAll(docs []Cfg) string {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			panic(err)
		}
	}
	enc.Close()
	return b.String()
}

const xoPrelude = `use std/yaml

struct Inner {
  x: Int
} derive Yaml

struct T {
  n: Int
  f: Float
  s: Str
  b: Bool
  o: Str?
  l: List[Int]
  m: Map[Str, Str]
  in_ as "in": Inner
} derive Yaml

struct P {
  x: Int
  y: Int
} derive Yaml

struct Opts {
  a as "a,omitempty": Str
  b as "b,omitempty": Int
  c as "c,omitempty": List[Int]
  d yaml "d,flow": List[Int]
  e yaml "e,flow": Map[Str, Int]
  f yaml "f,flow": P
  g yaml "g,omitempty,flow": List[Str]
  h as "h,omitempty": Str?
  i yaml "i,flow": List[P]
  j yaml "j,flow": Str
  k as "k,omitempty": Bool
  l as "l,omitempty": Float
  m: List[Int]
} derive Yaml

struct Item {
  n: Int
  tags: List[Str]
} derive Yaml

struct Cfg {
  name: Str
  api_version as "apiVersion": Str
  port: Int
  ratio: Float
  on: Bool
  o: Str?
  none: Str?
  items: List[Item]
  m: Map[Str, Int]
  empty: List[Int]
  text: Str
  yes: Str
  num: Str
  inner: Item
} derive Yaml

struct Meta {
  name: Str
  tags: List[Str]
} derive Yaml

struct Base {
  id: Int
  meta: Meta
} derive Yaml

struct Top {
  base yaml ",inline": Base
  extra: Str
  hidden as "-": Int
  n: Int
} derive Yaml

fn show_top(text: Str) -> Str {
  show_top_with(text, false)
}

fn show_top_with(text: Str, strict: Bool) -> Str {
  match yaml.decode[Top](text, strict) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(t) => "id=${t.base.id} meta=${t.base.meta.name}/${t.base.meta.tags.join(",")} extra=${t.extra} hidden=${t.hidden} n=${t.n}"
  }
}

struct Deep {
  d: List[List[List[List[Str]]]]
} derive Yaml

fn show_deep(text: Str) -> Str {
  match yaml.decode[Deep](text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(v) => {
      var n = 0
      for a in v.d {
        for b in a {
          for c in b {
            n += c.len()
          }
        }
      }
      "leaves=${n}"
    }
  }
}

fn fl(f: Float) -> Str {
  if f.is_nan() {
    return "nan"
  }
  if f.is_inf() {
    return if f > 0.0 { "inf" } else { "-inf" }
  }
  f.to_general(-1)
}

fn show(text: Str) -> Str {
  match yaml.decode[T](text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(t) => show_t(t)
  }
}

fn show_t(t: T) -> Str {
  var ls: List[Str] = []
  for x in t.l {
    ls.push("${x}")
  }
  var ms: List[Str] = []
  for k in t.m.keys().sort() {
    ms.push("${k.quote()}=${(t.m.get(k) ?? "").quote()}")
  }
  let o = match t.o {
    Some(v) => v.quote()
    None => "nil"
  }
  "n=${t.n} f=${fl(t.f)} s=${t.s.quote()} b=${t.b} o=${o} l=[${ls.join(",")}] m={${ms.join(",")}} in=${t.in_.x}"
}

fn show_all(text: Str) -> Str {
  match yaml.decode_all[T](text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(ts) => {
      var parts: List[Str] = []
      for t in ts {
        parts.push(show_t(t))
      }
      "${ts.len()} docs: ${parts.join(" | ")}"
    }
  }
}

`

func main() {
	if len(os.Args) > 3 && os.Args[1] == "fuzz" {
		fuzzMain(os.Args[2], os.Args[3])
		return
	}
	ents, err := cases.ReadDir("cases")
	if err != nil {
		panic(err)
	}
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from gopkg.in/yaml.v3 v3.0.1's Unmarshal and Marshal on hand written typed cases and the yaml-test-suite documents.\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString(xoPrelude)
	o.WriteString(`/// (the YAML text, what yaml.v3 decodes it into T as)
const CASES: List[(Str, Str)] = [
`)
	for _, e := range ents {
		in, _ := cases.ReadFile("cases/" + e.Name())
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(string(in)), xq(show(in)))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (an alias expansion document, what yaml.v3 decodes it into Deep as)\nconst DEEP: List[(Str, Str)] = [\n")
	for _, d := range deepCases() {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(deepShow(d)))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (a document, what yaml.v3 decodes it into Top as: an inline struct and a skipped field)\nconst INLINE: List[(Str, Str)] = [\n")
	for _, d := range inlineCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(inlineShow(d)))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (a document, what yaml.v3 decodes it into Top as with KnownFields(true))\nconst STRICT: List[(Str, Str)] = [\n")
	for _, d := range strictCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(strictShow(d)))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (a stream of documents, what yaml.v3's Decoder reads from it as T values)\nconst ALL: List[(Str, Str)] = [\n")
	for _, d := range allCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(allShow(d)))
	}
	o.WriteString("]\n\n")
	hh := "x"
	docs := []Cfg{
		{"a", "v1", 1, 1.5, true, nil, nil, []Item{{1, []string{"t"}}}, map[string]int{"k": 1}, []int{}, "s", "yes", "1", Item{2, nil}},
		{"b", "v2", 2, 0, false, &hh, nil, nil, nil, nil, "l1\nl2\n", "no", "2", Item{}},
		{"c", "v3", 3, 2.5, true, nil, nil, []Item{}, map[string]int{}, []int{1}, "", "", "", Item{3, []string{"z"}}},
	}
	fmt.Fprintf(&o, "const MARSHALED_ALL: Str = %s\n\n", xq(marshalAll(docs)))
	topv := Top{Base{7, Meta{"m", []string{"a"}}}, "e", 99, 3}
	tb, err := yaml.Marshal(topv)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(&o, "const MARSHALED_TOP: Str = %s\n\n", xq(string(tb)))
	h := "hi"
	c := Cfg{"orders", "v1", 8080, 2.5, true, &h, nil, []Item{{1, []string{"a", "b c"}}, {2, []string{}}}, map[string]int{"b10": 1, "b9": 2, "a": 3}, []int{}, "l1\nl2\n", "yes", "123", Item{7, []string{"x: y", "- z", ""}}}
	b, err := yaml.Marshal(c)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(&o, "const MARSHALED: Str = %s\n\n", xq(string(b)))
	var b2 bytes.Buffer
	enc := yaml.NewEncoder(&b2)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		panic(err)
	}
	enc.Close()
	fmt.Fprintf(&o, "const MARSHALED2: Str = %s\n\n", xq(b2.String()))
	e := ""
	empty := ""
	for _, v := range []Opts{
		{"", 0, nil, []int{1, 2}, map[string]int{"z": 1, "a": 2}, P{1, 2}, []string{"p", "q r"}, nil, []P{{3, 4}}, "l1\nl2", false, 0, []int{5}},
		{"s", 2, []int{1}, []int{}, map[string]int{}, P{}, []string{}, &empty, []P{}, "x", true, 1.5, []int{}},
	} {
		ob, err := yaml.Marshal(v)
		if err != nil {
			panic(err)
		}
		e += string(ob)
	}
	fmt.Fprintf(&o, "const MARSHALED_OPTS: Str = %s\n\n", xq(e))
	o.WriteString(`test "decode into a struct that derives Yaml is yaml.v3's Unmarshal, errors included" {
  for (text, want) in CASES {
    expect(show(text)) == want
  }
}

test "alias expansion is limited as yaml.v3 limits it" {
  for (text, want) in DEEP {
    expect(show_deep(text)) == want
  }
}

test "a strict decode refuses unknown keys as yaml.v3's KnownFields" {
  for (text, want) in STRICT {
    expect(show_top_with(text, true)) == want
  }
}

test "decode_all reads every document as yaml.v3's Decoder does" {
  for (text, want) in ALL {
    expect(show_all(text)) == want
  }
}

test "encode_all writes the documents as yaml.v3's Encoder does" {
  let docs = [
    Cfg{name: "a", api_version: "v1", port: 1, ratio: 1.5, on: true, o: None, none: None, items: [Item{n: 1, tags: ["t"]}], m: {"k": 1}, empty: [], text: "s", yes: "yes", num: "1", inner: Item{n: 2, tags: []}},
    Cfg{name: "b", api_version: "v2", port: 2, ratio: 0.0, on: false, o: Some("x"), none: None, items: [], m: {}, empty: [], text: "l1\nl2\n", yes: "no", num: "2", inner: Item{n: 0, tags: []}},
    Cfg{name: "c", api_version: "v3", port: 3, ratio: 2.5, on: true, o: None, none: None, items: [], m: {}, empty: [1], text: "", yes: "", num: "", inner: Item{n: 3, tags: ["z"]}},
  ]
  expect(yaml.encode_all[Cfg](docs).ok()) == Some(MARSHALED_ALL)
}

test "inline and skipped fields are yaml.v3's" {
  for (text, want) in INLINE {
    expect(show_top(text)) == want
  }
  let t = Top{base: Base{id: 7, meta: Meta{name: "m", tags: ["a"]}}, extra: "e", hidden: 99, n: 3}
  expect(yaml.encode_value[Top](t).ok()) == Some(MARSHALED_TOP)
}

test "encode_value is yaml.v3's Marshal" {
  let c = Cfg{name: "orders", api_version: "v1", port: 8080, ratio: 2.5, on: true, o: Some("hi"), none: None, items: [Item{n: 1, tags: ["a", "b c"]}, Item{n: 2, tags: []}], m: {"b10": 1, "b9": 2, "a": 3}, empty: [], text: "l1\nl2\n", yes: "yes", num: "123", inner: Item{n: 7, tags: ["x: y", "- z", ""]}}
  expect(yaml.encode_value[Cfg](c).ok()) == Some(MARSHALED)
  expect(yaml.encode_value[Cfg](c, 2).ok()) == Some(MARSHALED2)
}

test "encode_value honors omitempty and flow as yaml.v3 does" {
  let a = Opts{a: "", b: 0, c: [], d: [1, 2], e: {"z": 1, "a": 2}, f: P{x: 1, y: 2}, g: ["p", "q r"], h: None, i: [P{x: 3, y: 4}], j: "l1\nl2", k: false, l: 0.0, m: [5]}
  let b = Opts{a: "s", b: 2, c: [1], d: [], e: {}, f: P{x: 0, y: 0}, g: [], h: Some(""), i: [], j: "x", k: true, l: 1.5, m: []}
  expect((yaml.encode_value[Opts](a).ok() ?? "") + (yaml.encode_value[Opts](b).ok() ?? "")) == MARSHALED_OPTS
}
`)
	fmt.Print(o.String())
}
