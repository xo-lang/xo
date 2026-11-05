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
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

type Rt struct {
	D time.Duration  `yaml:"d"`
	B []byte         `yaml:"b"`
	P *time.Duration `yaml:"p"`
	E time.Duration  `yaml:"e,omitempty"`
	F []byte         `yaml:"f,omitempty"`
}

func rtFmt(r Rt) string {
	p := "nil"
	if r.P != nil {
		p = strconv.FormatInt(int64(*r.P), 10)
	}
	var bs, fs []string
	for _, x := range r.B {
		bs = append(bs, strconv.Itoa(int(x)))
	}
	for _, x := range r.F {
		fs = append(fs, strconv.Itoa(int(x)))
	}
	return fmt.Sprintf("d=%d b=[%s] p=%s e=%d f=[%s]", int64(r.D), strings.Join(bs, " "), p, int64(r.E), strings.Join(fs, " "))
}

// rtShow decodes into Rt and prints it, or the error.
func rtShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var r Rt
	if err := yaml.Unmarshal([]byte(in), &r); err != nil {
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	return rtFmt(r)
}

var rtCases = []string{
	"d: 1h30m", "d: 5", "d: 1.5", "d: abc", "d: \"10\"", "d: ~", "d: 1s\nb: [1, 2]", "d: [1]", "d: {a: 1}", "d: 1h\nd: 2h",
	"d: 100ms", "d: 1.5s", "d: -3m", "d: 0", "d: 2h45m30.5s", "d: 1us", "d: 7ns", "d: 36000h", "d: 0.1s", "d: 1d", "d: \" 1s\"", "d: 1s ", "d: !!str 5s",
	"b: hello", "b: !!binary aGVsbG8=", "b: [1, 2, 255]", "b: [256]", "b: [-1]", "b: [1.5, 2]", "b: [a, 2]", "b: [~, 3]", "b: [[1]]", "b: 5", "b: ~", "b: {a: 1}", "b: []", "b: [0x10, 0b11, 1_0]",
	"p: 5s", "p: ~", "p: 5", "e: 5s\nf: [1]", "e: 0s",
}

// dumpNode writes n and its content, one line a node.
func dumpNode(b *strings.Builder, n *yaml.Node, depth int) {
	a := "-"
	if n.Alias != nil {
		a = n.Alias.Anchor
	}
	fmt.Fprintf(b, "%s%d %s %d %s %s %d:%d ->%s %s %s %s\n", strings.Repeat(" ", depth), int(n.Kind), n.Tag, int(n.Style), strconv.Quote(n.Value), strconv.Quote(n.Anchor), n.Line, n.Column, a, strconv.Quote(n.HeadComment), strconv.Quote(n.LineComment), strconv.Quote(n.FootComment))
	for _, c := range n.Content {
		dumpNode(b, c, depth+1)
	}
}

// nodesShow parses every document of in into Nodes (Decoder.Decode into a yaml.Node)
// and dumps them, or the error.
func nodesShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	dec := yaml.NewDecoder(strings.NewReader(in))
	var b strings.Builder
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "ERR " + strconv.Quote(norm(err.Error()))
		}
		b.WriteString("doc\n")
		dumpNode(&b, &n, 0)
	}
	return b.String()
}

// jsonSafe reports whether v (decoded into an any) has only what to_json writes as Go
// does: string keys, no time.Time, no integer above the int64 range.
func jsonSafe(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for _, e := range x {
			if !jsonSafe(e) {
				return false
			}
		}
	case []any:
		for _, e := range x {
			if !jsonSafe(e) {
				return false
			}
		}
	case string:
		return utf8.ValidString(x)
	case map[any]any, time.Time, uint64:
		return false
	}
	return true
}

// jsonShow is yaml.v3 decoding into an any then json.Marshal: OK text, ERR, or SKIP.
func jsonShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "ERR"
		}
	}()
	var v any
	if err := yaml.Unmarshal([]byte(in), &v); err != nil {
		return "ERR"
	}
	if !jsonSafe(v) {
		return "SKIP"
	}
	var jb bytes.Buffer
	enc := json.NewEncoder(&jb)
	enc.SetEscapeHTML(false) // to_json writes < > & as they are, which is valid JSON
	if err := enc.Encode(v); err != nil {
		return "ERR"
	}
	return "OK " + strings.TrimSuffix(jb.String(), "\n")
}

// emitShow parses in into a Node and marshals it back, or the error; SKIP for a
// text with no document (a zero Node, which Marshal writes as null).
func emitShow(in string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = "ERR " + strconv.Quote(norm(fmt.Sprint(r)))
		}
	}()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(in), &n); err != nil {
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	if n.Kind == 0 {
		return "SKIP"
	}
	b, err := yaml.Marshal(&n)
	if err != nil {
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	return "OK " + string(b)
}

type Nd struct {
	A int       `yaml:"a"`
	N yaml.Node `yaml:"n"`
	Z string    `yaml:"z"`
}

// ndShow decodes into Nd and prints a, z and the Node tree of n, or the error.
func ndShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var t Nd
	if err := yaml.Unmarshal([]byte(in), &t); err != nil {
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "a=%d z=%s\n", t.A, t.Z)
	if t.N.Kind == 0 {
		b.WriteString("zero\n")
	} else {
		dumpNode(&b, &t.N, 0)
	}
	return b.String()
}

var ndCases = []string{
	"a: 1\nn: hello\nz: x\n",
	"n: [1, 2, {a: b}]\n",
	"n: &x {a: 1}\nz: after\n",
	"a: 3\n",
	"n: ~\n",
	"n: !!str 5\nz: y\n",
	"n: |\n  text\n  more\n",
	"x: &a 1\nn: *a\n",
	"# head\nn: 1 # line\n# foot\nz: q\n",
	"n:\n  - a\n  - b: c\n",
	"a: bad\nn: 1\n",
	"n: {a: 1, a: 2}\n",
}

type Tm struct {
	T time.Time  `yaml:"t"`
	P *time.Time `yaml:"p"`
	N int        `yaml:"n"`
}

func tmNanos(t time.Time) string {
	if t.IsZero() {
		return "0" // Xo's zero Time is the epoch
	}
	sec, ns := t.Unix(), int64(t.Nanosecond())
	if !((sec > -9223372037 || (sec == -9223372037 && ns >= 145224192)) && (sec < 9223372036 || (sec == 9223372036 && ns <= 854775807))) {
		return "RANGE"
	}
	return strconv.FormatInt(t.UnixNano(), 10)
}

// tmShow decodes into Tm and prints the instants, or the error (a Time.UnmarshalText
// error is ERRPARSE: its text is Go's layout error).
func tmShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var t Tm
	if err := yaml.Unmarshal([]byte(in), &t); err != nil {
		if strings.HasPrefix(err.Error(), "parsing time") {
			return "ERRPARSE"
		}
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	p := "nil"
	if t.P != nil {
		p = tmNanos(*t.P)
	}
	return fmt.Sprintf("t=%s p=%s n=%d", tmNanos(t.T), p, t.N)
}

// cvShow decodes into Tm and marshals it back (the offsets of the timestamps are kept), or SKIP when t
// is the zero time (Xo's zero Civil is the epoch) or ERRPARSE / ERR.
func cvShow(in string) (out string) {
	defer func() {
		if recover() != nil {
			out = "PANIC"
		}
	}()
	var t Tm
	if err := yaml.Unmarshal([]byte(in), &t); err != nil {
		if strings.HasPrefix(err.Error(), "parsing time") {
			return "ERRPARSE"
		}
		return "ERR " + strconv.Quote(norm(err.Error()))
	}
	if t.T.IsZero() {
		return "SKIP"
	}
	b, err := yaml.Marshal(t)
	if err != nil {
		return "SKIP"
	}
	return "OK " + string(b)
}

var tmP = time.Date(2002, 2, 3, 4, 5, 6, 7, time.UTC)

var tmCases = []string{
	"t: 2001-12-14", "t: 2001-12-14t21:59:43.10-05:00", "t: 2001-12-14T21:59:43.10-05:00", "t: 2001-12-14 21:59:43.10", "t: 2001-12-14 21:59:43",
	"t: 2001-1-2", "t: 2001-12-14T21:59:43Z", "t: 2001-12-14T1:2:3Z", "t: 2001-12-14 21:59:43 -5", "t: nope", "t: \"2001-12-14\"", "t: \"2001-12-14T21:59:43Z\"",
	"t: 5", "t: 2001-12-32", "t: !!timestamp 2001-12-14", "t: ~", "t: [1]", "t: {a: 1}", "p: 2002-02-03", "p: ~", "t: 1970-01-01", "t: 1969-12-31T23:59:59.999999999Z",
	"t: 2262-04-11T23:47:16.854775807Z", "t: 1677-09-21T00:12:43.145224192Z", "t: 3000-01-01", "t: 1000-01-01", "t: 2001-12-14T21:59:43.123456789+05:30", "t: 2001-12-14T21:59:43.1234567891Z",
	"n: 1\nt: 2020-02-29\np: 2021-02-29", "t: 2000-02-29T24:00:00Z", "t: 2001-12-14T21:59:43.Z", "t: 2001-12-14T21:59:43+0530",
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
	s = strings.ReplaceAll(s, "map[string]string", "Map[Str, Str]")
	// yaml.v3 cuts a long value at 7 bytes, maybe inside a rune; an Xo Str is valid text
	return strings.ToValidUTF8(s, "\ufffd")
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
use std/time

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

struct Rt {
  d: Duration
  b: Bytes
  p: Duration?
  e yaml ",omitempty": Duration
  f yaml ",omitempty": Bytes
} derive Yaml

fn show_rt(text: Str) -> Str {
  match yaml.decode[Rt](text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(r) => {
      var bs: List[Str] = []
      for i in 0..r.b.len() {
        bs.push("${r.b[i]}")
      }
      var fs: List[Str] = []
      for i in 0..r.f.len() {
        fs.push("${r.f[i]}")
      }
      let p = match r.p {
        Some(v) => "${v.nanos()}"
        None => "nil"
      }
      "d=${r.d.nanos()} b=[${bs.join(" ")}] p=${p} e=${r.e.nanos()} f=[${fs.join(" ")}]"
    }
  }
}

struct Nd {
  a: Int
  n: yaml.Node
  z: Str
} derive Yaml

fn show_nd(text: Str) -> Str {
  match yaml.decode[Nd](text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(t) => {
      var out = StrBuilder.new()
      out.write("a=${t.a} z=${t.z}\n")
      if t.n.line == 0 && t.n.value == "null" {
        out.write("zero\n")
      } else {
        dump_node(out, t.n, 0)
      }
      out.freeze()
    }
  }
}

struct Tm {
  t: Time
  p: Time?
  n: Int
} derive Yaml

fn show_tm(text: Str) -> Str {
  match yaml.decode[Tm](text) {
    Err(e) => if e.display().starts_with("yaml: parsing time") { "ERRPARSE" } else { "ERR ${e.display().quote()}" }
    Ok(t) => {
      let p = match t.p {
        Some(v) => "${(v - Time.UNIX_EPOCH).nanos()}"
        None => "nil"
      }
      "t=${(t.t - Time.UNIX_EPOCH).nanos()} p=${p} n=${t.n}"
    }
  }
}

struct Cv {
  t: time.Civil
  p: time.Civil?
  n: Int
} derive Yaml

fn show_cv(text: Str) -> Str {
  match yaml.decode[Cv](text) {
    Err(e) => if e.display().starts_with("yaml: parsing time") { "ERRPARSE" } else { "ERR ${e.display().quote()}" }
    Ok(c) => match yaml.encode_value[Cv](c) {
      Ok(t) => "OK ${t}"
      Err(e) => "ERR ${e.display().quote()}"
    }
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

fn node_kind_n(k: yaml.NodeKind) -> Int {
  match k {
    Document => 1
    Sequence => 2
    Mapping => 4
    Scalar => 8
    Alias => 16
  }
}

fn dump_node(var out: StrBuilder, n: yaml.Node, depth: Int) {
  let a = match n.alias {
    Some(t) => t.anchor
    None => "-"
  }
  out.write("${" ".repeat(depth)}${node_kind_n(n.kind)} ${n.tag} ${n.style} ${n.value.quote()} ${n.anchor.quote()} ${n.line}:${n.column} ->${a} ${n.head_comment.quote()} ${n.line_comment.quote()} ${n.foot_comment.quote()}\n")
  for c in n.content {
    dump_node(out, c, depth + 1)
  }
}

fn show_nodes(text: Str) -> Str {
  var out = StrBuilder.new()
  match yaml.parse_all(text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(ns) => {
      for n in ns {
        out.write("doc\n")
        dump_node(out, n, 0)
      }
      out.freeze()
    }
  }
}

fn show_emit(text: Str) -> Str {
  match yaml.parse(text) {
    Err(e) => "ERR ${e.display().quote()}"
    Ok(n) => {
      if n.content.is_empty() {
        return "SKIP"
      }
      match yaml.encode_node(n) {
        Ok(t) => "OK ${t}"
        Err(e) => "ERR ${e.display().quote()}"
      }
    }
  }
}

fn show_json(text: Str) -> Str {
  match yaml.to_json(text) {
    Ok(t) => "OK ${t}"
    Err(_) => "ERR"
  }
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
	o.WriteString("/// (the YAML text, the Node trees yaml.v3 parses it into: kind tag style value anchor line:column ->alias, indented by depth)\nconst NODES: List[(Str, Str)] = [\n")
	for _, e := range ents {
		in, _ := cases.ReadFile("cases/" + e.Name())
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(string(in)), xq(nodesShow(string(in))))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (the YAML text, what yaml.v3 writes for the Node it parses it into)\nconst EMITS: List[(Str, Str)] = [\n")
	for _, e := range ents {
		in, _ := cases.ReadFile("cases/" + e.Name())
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(string(in)), xq(emitShow(string(in))))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (the YAML text, what yaml.v3 decoding into an any and json.Marshal write: OK text, ERR, or SKIP)\nconst JSONS: List[(Str, Str)] = [\n")
	for _, e := range ents {
		in, _ := cases.ReadFile("cases/" + e.Name())
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(string(in)), xq(jsonShow(string(in))))
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
	o.WriteString("/// (a document, what yaml.v3 decodes it into Rt as: a Duration, Bytes, an optional Duration)\nconst RT: List[(Str, Str)] = [\n")
	for _, d := range rtCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(rtShow(d)))
	}
	o.WriteString("]\n\n")
	p5 := 5 * time.Second
	rts := []Rt{
		{D: 90 * time.Minute, B: []byte("hello"), P: &p5, E: 1500 * time.Millisecond, F: []byte{0, 255}},
		{},
		{D: -time.Hour - 30*time.Second - 500*time.Millisecond, B: []byte{}, E: 7, F: nil},
		{D: 100 * time.Microsecond, E: 1234567 * time.Nanosecond},
		{D: 25*time.Hour + 1, E: -1},
	}
	for i, r := range rts {
		rb, err := yaml.Marshal(r)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&o, "const MARSHALED_RT%d: Str = %s\n", i, xq(string(rb)))
	}
	o.WriteString("\n")
	o.WriteString("/// (a document, what yaml.v3 decodes it into Nd as: a, z and the Node tree of n)\nconst ND: List[(Str, Str)] = [\n")
	for _, d := range ndCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(ndShow(d)))
	}
	o.WriteString("]\n\n")
	var ndOut strings.Builder
	for i, d := range ndCases[:10] {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(d), &doc); err != nil || len(doc.Content) == 0 {
			continue
		}
		// the value of the document's first key, as a Node field of Nd
		root := doc.Content[0]
		if root.Kind != yaml.MappingNode || len(root.Content) < 2 {
			continue
		}
		mb, err := yaml.Marshal(Nd{A: i, N: *root.Content[1], Z: "z"})
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&ndOut, "  (%s, %d, %s),\n", xq(d), i, xq(string(mb)))
	}
	fmt.Fprintf(&o, "/// (a document, what yaml.v3 writes for an Nd whose n is the value of its first key; a is the index)\nconst ND_MARSHALED: List[(Str, Int, Str)] = [\n%s]\n\n", ndOut.String())
	o.WriteString("/// (a document, what yaml.v3 decodes it into Tm as: the UnixNano of t and p; RANGE outside int64 nanoseconds, ERRPARSE for Time.UnmarshalText's error)\nconst TM: List[(Str, Str)] = [\n")
	for _, d := range tmCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(tmShow(d)))
	}
	o.WriteString("]\n\n")
	o.WriteString("/// (a document, what yaml.v3 decodes into Tm and marshals back: the offsets are kept; SKIP, ERRPARSE, ERR)\nconst CV: List[(Str, Str)] = [\n")
	for _, d := range tmCases {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(d), xq(cvShow(d)))
	}
	o.WriteString("]\n\n")
	tms := []Tm{
		{T: time.Date(2001, 12, 15, 2, 59, 43, 100000000, time.UTC), N: 1},
		{T: time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC)},
		{T: time.Date(1969, 12, 31, 23, 59, 59, 999999999, time.UTC), P: &tmP},
		{T: time.Date(2262, 4, 11, 23, 47, 16, 854775807, time.UTC)},
	}
	for i, r := range tms {
		rb, err := yaml.Marshal(r)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&o, "const MARSHALED_TM%d: Str = %s\n", i, xq(string(rb)))
	}
	o.WriteString("\n")
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

test "Duration and Bytes fields are yaml.v3's" {
  for (text, want) in RT {
    expect(show_rt(text)) == want
  }
  let rs = [
    Rt{d: Duration.from_nanos(5400000000000), b: b"hello", p: Some(Duration.from_nanos(5000000000)), e: Duration.from_nanos(1500000000), f: b"\x00\xff"},
    Rt{d: Duration.from_nanos(0), b: b"", p: None, e: Duration.from_nanos(0), f: b""},
    Rt{d: Duration.from_nanos(-3630500000000), b: b"", p: None, e: Duration.from_nanos(7), f: b""},
    Rt{d: Duration.from_nanos(100000), b: b"", p: None, e: Duration.from_nanos(1234567), f: b""},
    Rt{d: Duration.from_nanos(90000000000001), b: b"", p: None, e: Duration.from_nanos(-1), f: b""},
  ]
  let want = [MARSHALED_RT0, MARSHALED_RT1, MARSHALED_RT2, MARSHALED_RT3, MARSHALED_RT4]
  for (i, r) in rs.indexed() {
    expect(yaml.encode_value[Rt](r).ok()) == Some(want[i])
  }
}

test "parse_all builds yaml.v3's Node trees" {
  for (text, want) in NODES {
    expect(show_nodes(text)) == want
  }
}

test "encode_node writes the Node a document parses into as yaml.v3's Marshal" {
  for (text, want) in EMITS {
    if want == "SKIP" {
      continue
    }
    expect(show_emit(text)) == want
  }
}

test "to_json is yaml.v3 decoded into an any and marshaled by encoding/json" {
  for (text, want) in JSONS {
    if want == "SKIP" {
      continue
    }
    expect(show_json(text)) == want
  }
}

test "a yaml.Node field is the node, as yaml.v3's" {
  for (text, want) in ND {
    expect(show_nd(text)) == want
  }
  for (text, idx, want) in ND_MARSHALED {
    let doc = yaml.parse(text).ok() ?? fault("parse")
    let root = doc.content[0]
    expect(yaml.encode_value[Nd](Nd{a: idx, n: root.content[1], z: "z"}).ok()) == Some(want)
  }
}

test "Time fields are the instants yaml.v3's time.Time holds" {
  for (text, want) in TM {
    let got = show_tm(text)
    if want.contains("RANGE") {
      expect(got.contains("ERR") || got.contains("RANGE")) == true
      continue
    }
    if want == "ERRPARSE" || got == "ERRPARSE" {
      expect(got) == want
      continue
    }
    expect(got) == want
  }
  let ts = [
    Tm{t: Time.UNIX_EPOCH + Duration.from_nanos(1008385183100000000), p: None, n: 1},
    Tm{t: Time.UNIX_EPOCH + Duration.from_nanos(1582934400000000000), p: None, n: 0},
    Tm{t: Time.UNIX_EPOCH + Duration.from_nanos(-1), p: Some(Time.UNIX_EPOCH + Duration.from_nanos(1012709106000000007)), n: 0},
    Tm{t: Time.UNIX_EPOCH + Duration.from_nanos(9223372036854775807), p: None, n: 0},
  ]
  let want = [MARSHALED_TM0, MARSHALED_TM1, MARSHALED_TM2, MARSHALED_TM3]
  for (i, t) in ts.indexed() {
    expect(yaml.encode_value[Tm](t).ok()) == Some(want[i])
  }
}

test "time.Civil fields keep the offset of the timestamp, as yaml.v3's time.Time" {
  for (text, want) in CV {
    if want == "SKIP" {
      continue
    }
    let got = show_cv(text)
    if want.contains("ERR") && !want.contains("ERRPARSE") {
      expect(got.starts_with("ERR")) == true
      continue
    }
    expect(got) == want
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
