package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
)

// The API this corpus covers: the encode side of std/json, which Lang 3's
// decoder change does not touch. json.encode and encode_str over every type
// the Json table in std.md 5 lists and a derived struct's rules: field order,
// wire names, `derive Json(case: ...)`, `,omitempty`, Option as null, enums
// with and without fields, tuples, List, Set, Map, Bytes as base64, Duration
// and Time as strings, newtypes, nesting; then json.to_value, Value.encode,
// json.parse_value and json.from_value.
//
// The expectations are Go's encoding/json with SetEscapeHTML(false), which is
// exactly what std/rt/json.go appends and its json_test.go checks. Go's
// Marshal is not usable directly for three reasons the corpus works around:
// it escapes <, > and & (so every value goes through an Encoder with
// SetEscapeHTML(false), whose trailing newline is trimmed), it sorts map
// keys (so the field order cases use structs, not maps), and it refuses NaN
// and +-Inf where Xo writes null (those rows say so).

// jsonGo is Go's encoding of v, as Xo writes it: SetEscapeHTML(false), no
// indentation, no trailing newline.
func jsonGo(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: Go cannot encode a json corpus value: %v\n", err)
		os.Exit(1)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// jsonFloatText is Go's JSON text of the Float that bits hold, or "null" for
// the values Go refuses and Xo writes as null.
func jsonFloatText(bits uint64) string {
	f := math.Float64frombits(bits)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "null"
	}
	return jsonGo(f)
}

func jsonEncodeCorpus() string {
	var o strings.Builder
	o.WriteString(header("encoding/json with SetEscapeHTML(false) — the text std/rt/json.go appends — for the encode side of std/json: json.encode and encode_str over scalars, strings, floats, lists, sets, maps, tuples, Options, enums with and without fields, newtypes, derived structs with wire names and case rules and omitempty, nesting, Bytes, Duration and Time, then to_value, Value.encode, parse_value and from_value, on seeded values and on Go's own escaping, number and float cases (Go's encoding/json tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/json\n")

	o.WriteString(`
// The types the encode cases use. Declared here, not in std: the corpus is
// the program that uses them, as every corpus is.
enum Level {
  Low
  High
} derive Json

enum Shape {
  Circle(r: Float)
  Rect(w: Float, h: Float)
} derive Json

type Email struct(Str)

struct Point {
  x: Int
  y: Int
} derive Json

struct Inner {
  id: Int
  name: Str
} derive Json

struct Scalars {
  i: Int
  f: Float
  b: Bool
  s: Str
} derive Json

struct Opts {
  a: Int?
  b: Str?
  c: Bool?
  d: List[Int]?
} derive Json

struct Enums {
  lvl: Level
  shape: Shape
} derive Json

struct Collections {
  l: List[Str]
  st: Set[Str]
  m: Map[Str, Int]
  pair: (Str, Int)
} derive Json

struct Special {
  bytes: Bytes
  dur: Duration
  when: Time
  mail: Email
} derive Json

struct Cased {
  a_b: Str
  c: Str
} derive Json(case: camel)

struct Wired {
  kind as "type": Str
  note as "note,omitempty": Str
  always: Str
} derive Json

struct Nested {
  inner: Inner
  list: List[Inner]
} derive Json

struct StrBox {
  s: Str
} derive Json

struct FloatBox {
  v: Float
} derive Json

struct IntBox {
  n: Int
} derive Json
`)

	// Strings: the escapes Go's encoder decides on, plus seeded texts.
	strs := []string{"", "plain", `quote " and backslash \`, "tab\there",
		"newline\nhere", "carriage\rreturn", "slice\bbell\f", "\x00\x01\x1f",
		"<html> & </html>", "line\u2028sep\u2029para",
		"� replacement", "long " + strings.Repeat("x", 200),
		"nul\u0000end", "a/b", "~/path", "\u007f del"}
	rs := rand.New(rand.NewSource(109))
	runes := []rune("héllo wörld ✓ 🚀 \u2028 \ufffd")
	for i := 0; i < 20; i++ {
		var b strings.Builder
		for j := 0; j < 1+rs.Intn(30); j++ {
			switch rs.Intn(5) {
			case 0:
				b.WriteByte(byte(rs.Intn(0x20)))
			case 1:
				b.WriteByte(byte(0x20 + rs.Intn(0x5f)))
			case 2:
				b.WriteRune(rune(0x80 + rs.Intn(0x7f)))
			case 3:
				b.WriteRune(rune(0x4e00 + rs.Intn(0x100)))
			default:
				b.WriteRune(runes[rs.Intn(len(runes))])
			}
		}
		strs = append(strs, b.String())
	}
	o.WriteString("\n/// (value, Go's encoding/json text with SetEscapeHTML(false))\nconst STR_CASES: List[(StrBox, Str)] = [\n")
	for _, s := range strs {
		fmt.Fprintf(&o, "  (StrBox{s: %s}, %s),\n", xq(s), xq(jsonGo(struct {
			S string `json:"s"`
		}{s})))
	}
	o.WriteString("]\n")

	// Floats: Go's number layout (the 'e' switch at 1e21 and 1e-7, the
	// two-digit exponent), the zeros, and the values Go refuses and Xo
	// writes as null.
	bits := []uint64{0, 1 << 63, math.Float64bits(1), math.Float64bits(-1),
		math.Float64bits(0.5), math.Float64bits(1.5), math.Float64bits(0.1),
		math.Float64bits(1e20), math.Float64bits(1e21), math.Float64bits(1e-6),
		math.Float64bits(1e-7), math.Float64bits(123456789),
		math.Float64bits(1e300), math.Float64bits(5e-324),
		math.Float64bits(math.MaxFloat64), math.Float64bits(2.2250738585072014e-308),
		math.Float64bits(3.141592653589793), math.Float64bits(1e6),
		math.Float64bits(1e-5), math.Float64bits(9007199254740993),
		math.Float64bits(math.Inf(1)), math.Float64bits(math.Inf(-1)),
		math.Float64bits(math.NaN())}
	rf := rand.New(rand.NewSource(113))
	for i := 0; i < 40; i++ {
		bits = append(bits, rf.Uint64())
	}
	o.WriteString("\n/// (bits, Go's encoding/json text of that Float field; both infinities and NaN are null in Xo, which Go refuses to encode)\nconst FLOAT_CASES: List[(FloatBox, Str)] = [\n")
	for _, b := range bits {
		f := math.Float64frombits(b)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			// Go refuses to encode these; Xo writes null (std/rt/json.go
			// jsonFloat, and the native append_json_float).
			fmt.Fprintf(&o, "  (FloatBox{v: Float.from_bits(%d)}, %s),\n", int64(b), xq(`{"v":null}`))
			continue
		}
		fmt.Fprintf(&o, "  (FloatBox{v: Float.from_bits(%d)}, %s),\n", int64(b),
			xq(jsonGo(struct {
				V float64 `json:"v"`
			}{f})))
	}
	o.WriteString("]\n")

	// Ints.
	ints := []int64{0, 1, -1, 42, -42, math.MaxInt64, math.MinInt64,
		9007199254740993, 1000000000000000000}
	rin := rand.New(rand.NewSource(127))
	for i := 0; i < 15; i++ {
		ints = append(ints, rin.Int63()-rin.Int63())
	}
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst INT_CASES: List[(IntBox, Str)] = [\n")
	for _, n := range ints {
		fmt.Fprintf(&o, "  (IntBox{n: %d}, %s),\n", n, xq(jsonGo(struct {
			N int64 `json:"n"`
		}{n})))
	}
	o.WriteString("]\n")

	// A struct of scalars, in declaration order.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst SCALARS_CASES: List[(Scalars, Str)] = [\n")
	for _, v := range []struct {
		i int64
		f float64
		b bool
		s string
	}{
		{0, 0, false, ""},
		{1, 1.5, true, "x"},
		{-1, -0.0, false, `a"b`},
		{math.MaxInt64, 1e21, true, "tab\there"},
		{math.MinInt64, 5e-324, false, "<&>"},
	} {
		text := fmt.Sprintf(`{"i":%d,"f":%s,"b":%t,"s":%s}`, v.i, jsonFloatText(math.Float64bits(v.f)), v.b, jsonGo(v.s))
		fmt.Fprintf(&o, "  (Scalars{i: %d, f: Float.from_bits(%d), b: %v, s: %s}, %s),\n",
			v.i, int64(math.Float64bits(v.f)), v.b, xq(v.s), xq(text))
	}
	o.WriteString("]\n")

	// row writes one table entry: the Xo literal, then the JSON Go writes.
	// Xo has no backtick strings, so the text goes through xq like any other.
	row := func(xo, want string) {
		fmt.Fprintf(&o, "  (%s, %s),\n", xo, xq(want))
	}

	// Options: None is null, Some is the value.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst OPTS_CASES: List[(Opts, Str)] = [\n")
	row(`Opts{a: None, b: None, c: None, d: None}`, `{"a":null,"b":null,"c":null,"d":null}`)
	row(`Opts{a: Some(0), b: Some(""), c: Some(false), d: Some([])}`, `{"a":0,"b":"","c":false,"d":[]}`)
	row(`Opts{a: Some(-1), b: Some("x"), c: Some(true), d: Some([1, 2])}`, `{"a":-1,"b":"x","c":true,"d":[1,2]}`)
	o.WriteString("]\n")

	// Enums: a unit variant is a string, a variant with fields an object
	// keyed by the variant.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst ENUMS_CASES: List[(Enums, Str)] = [\n")
	row(`Enums{lvl: Level.Low, shape: Shape.Circle(r: 0.0)}`, `{"lvl":"Low","shape":{"Circle":{"r":0}}}`)
	row(`Enums{lvl: Level.High, shape: Shape.Circle(r: 1.5)}`, `{"lvl":"High","shape":{"Circle":{"r":1.5}}}`)
	row(`Enums{lvl: Level.Low, shape: Shape.Rect(w: 1.0, h: 2.0)}`, `{"lvl":"Low","shape":{"Rect":{"w":1,"h":2}}}`)
	o.WriteString("]\n")

	// Collections: a Set in insertion order, a Map in insertion order (Go
	// sorts map keys, so these two are written in sorted order), a tuple as
	// an array.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst COLLECTIONS_CASES: List[(Collections, Str)] = [\n")
	row(`Collections{l: [], st: #{}, m: {}, pair: ("", 0)}`, `{"l":[],"st":[],"m":{},"pair":["",0]}`)
	row(`Collections{l: ["a", "b"], st: #{"x", "y"}, m: {"a": 1, "b": 2}, pair: ("k", -1)}`, `{"l":["a","b"],"st":["x","y"],"m":{"a":1,"b":2},"pair":["k",-1]}`)
	row(`Collections{l: ["one"], st: #{"only"}, m: {"k": 0}, pair: ("t", 9223372036854775807)}`, `{"l":["one"],"st":["only"],"m":{"k":0},"pair":["t",9223372036854775807]}`)
	o.WriteString("]\n")

	// Bytes as base64, Duration and Time as strings, a newtype as its
	// underlying value.
	o.WriteString("\n/// (value, Go's encoding/json text; a Duration and a Time are strings and a newtype is its underlying value, as std.md 5 says)\nconst SPECIAL_CASES: List[(Special, Str)] = [\n")
	row(`Special{bytes: b"", dur: Duration.from_secs_int(0), when: Time.parse_rfc3339("1970-01-01T00:00:00Z") ?? Time.UNIX_EPOCH, mail: Email("a@b")}`, `{"bytes":"","dur":"0s","when":"1970-01-01T00:00:00Z","mail":"a@b"}`)
	row(`Special{bytes: b"\x00\x01\xff", dur: Duration.from_secs(1.5), when: Time.parse_rfc3339("2026-10-24T12:34:56Z") ?? Time.UNIX_EPOCH, mail: Email("x@y.z")}`, `{"bytes":"AAH/","dur":"1.5s","when":"2026-10-24T12:34:56Z","mail":"x@y.z"}`)
	row(`Special{bytes: b"hello, world", dur: Duration.from_secs_int(3600), when: Time.parse_rfc3339("1999-12-31T23:59:59Z") ?? Time.UNIX_EPOCH, mail: Email("")}`, `{"bytes":"aGVsbG8sIHdvcmxk","dur":"1h0m0s","when":"1999-12-31T23:59:59Z","mail":""}`)
	o.WriteString("]\n")

	// A struct with a case rule: camelCase keys.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst CASED_CASES: List[(Cased, Str)] = [\n")
	row(`Cased{a_b: "v", c: "w"}`, `{"aB":"v","c":"w"}`)
	row(`Cased{a_b: "", c: ""}`, `{"aB":"","c":""}`)
	o.WriteString("]\n")

	// Wire names and omitempty.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst WIRED_CASES: List[(Wired, Str)] = [\n")
	row(`Wired{kind: "k", note: "", always: "a"}`, `{"type":"k","always":"a"}`)
	row(`Wired{kind: "k", note: "n", always: "a"}`, `{"type":"k","note":"n","always":"a"}`)
	o.WriteString("]\n")

	// Nesting.
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst NESTED_CASES: List[(Nested, Str)] = [\n")
	row(`Nested{inner: Inner{id: 1, name: "n"}, list: []}`, `{"inner":{"id":1,"name":"n"},"list":[]}`)
	row(`Nested{inner: Inner{id: 0, name: ""}, list: [Inner{id: 2, name: "m"}]}`, `{"inner":{"id":0,"name":""},"list":[{"id":2,"name":"m"}]}`)
	o.WriteString("]\n")

	// Value: a canonical text stays itself through parse_value and encode
	// (keys in sorted order, so Go's compact agrees), and a built Value is
	// the same text as the value it came from.
	o.WriteString("\n/// (JSON text, its canonical text: parse_value then encode)\nconst VALUE_CASES: List[(Str, Str)] = [\n")
	texts := []string{`null`, `true`, `false`, `0`, `-0`, `1`, `-1`,
		`1.5`, `-1.5`, `0.1`, `1e21`, `1e-7`, `"a"`, `"a\"b"`,
		`"tab\there"`, `""`, `[]`, `[1,2,3]`, `[[1],[2]]`, `{}`,
		`{"a":1}`, `{"a":1,"b":2}`, `{"a":{"b":[true,null]}}`,
		`{"a":1,"b":[{"c":"d"}]}`, `[{"a":1},{"a":2}]`}
	for _, t := range texts {
		if t == "-0" {
			// Value's Int rule (decision 0299): a number without a fraction
			// or exponent that fits an Int is Int, and Int 0 encodes as 0.
			// Go decodes into float64 instead and keeps the sign, so this is
			// the one row the two do not share.
			fmt.Fprintf(&o, "  (%s, %s),\n", xq(t), xq("0"))
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(t), &v); err != nil {
			fmt.Fprintf(os.Stderr, "stdcorpus: Go cannot read a json corpus text %q: %v\n", t, err)
			os.Exit(1)
		}
		want := jsonGo(v)
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(t), xq(want))
	}
	o.WriteString("]\n")

	// The tests. Each table is one encode over its values; the runner names
	// the row by index, and on a failure the two texts show.
	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}
`)
	type box struct {
		name string // table name
		typ  string // the Xo type, for the test's name
	}
	for _, b := range []box{
		{"STR_CASES", "a Str"},
		{"FLOAT_CASES", "a Float"},
		{"INT_CASES", "an Int"},
		{"SCALARS_CASES", "a struct of scalars"},
		{"OPTS_CASES", "an Option field"},
		{"ENUMS_CASES", "an enum"},
		{"COLLECTIONS_CASES", "lists, sets, maps and tuples"},
		{"SPECIAL_CASES", "Bytes, Duration, Time and a newtype"},
		{"CASED_CASES", "a derive Json(case: camel) struct"},
		{"WIRED_CASES", "wire names and omitempty"},
		{"NESTED_CASES", "a nested struct"},
	} {
		fmt.Fprintf(&o, `
test "encode of %s is Go's" {
  var bad: List[(Str, Str, Str)] = []
  var i = 0
  for (v, want) in %s {
    let got = json.encode_str(v)
    if got != want {
      bad.push(("row ${i}", got, want))
    }
    let bytes_text = json.encode(v)
    if bytes_text.len() != got.len() || bytes_text.to_hex() != got.bytes().to_hex() {
      bad.push(("row ${i} as Bytes", bytes_text.to_hex(), got.bytes().to_hex()))
    }
    i += 1
  }
  expect(show3(bad)) == []
}
`, b.typ, b.name)
	}

	o.WriteString(`
test "parse_value then encode gives the canonical text" {
  var bad: List[(Str, Str, Str)] = []
  var i = 0
  for (text, want) in VALUE_CASES {
    let got = match json.parse_value(text) {
      Ok(v) => v.encode()
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("row ${i} ${text}", got, want))
    }
    i += 1
  }
  expect(show3(bad)) == []
}

test "to_value of a struct encodes as the struct does" {
  var bad: List[(Str, Str, Str)] = []
  for (v, want) in NESTED_CASES {
    let got = json.to_value(v).encode()
    if got != want {
      bad.push(("nested", got, want))
    }
  }
  for (v, want) in SCALARS_CASES {
    let got = json.to_value(v).encode()
    if got != want {
      bad.push(("scalars", got, want))
    }
  }
  for (v, want) in ENUMS_CASES {
    let got = json.to_value(v).encode()
    if got != want {
      bad.push(("enums", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "from_value of to_value gives the same encoding back" {
  var bad: List[(Str, Str, Str)] = []
  for (v, want) in NESTED_CASES {
    let got = match json.from_value[Nested](json.to_value(v)) {
      Ok(back) => json.encode_str(back)
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("nested", got, want))
    }
  }
  for (v, want) in COLLECTIONS_CASES {
    let got = match json.from_value[Collections](json.to_value(v)) {
      Ok(back) => json.encode_str(back)
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("collections", got, want))
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
