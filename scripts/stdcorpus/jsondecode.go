package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// The API this corpus covers: the decode side of std/json — json.decode and
// decode_str into the types std.md 5's rule table lists, json.parse_value and
// json.from_value, and Patch's absent/null/value. The expectations are Go's:
// a Go twin type is filled with encoding/json and the row's want is that
// twin re-encoded (which std/rt/json.go makes byte for byte Go's with
// SetEscapeHTML(false), so a decode that differs shows as a text that
// differs). Error rows expect only that the text is refused, because Xo's
// messages carry a path and are worded for Xo; the ones where Xo documents
// Go's own text ("invalid JSON: <Go's text>") assert the text as well.
//
// Where Xo's reading is documented to differ from Go's, the row says so:
// parse_value's Int rule and object key order (decision 0299), duplicate
// keys, and a Time outside the range of Time (decision 0364). Go's
// encoding/json tests are Copyright 2009 The Go Authors, BSD-style license.

// xoCanonical is the text std/json documents for a Value built from text
// (decision 0299): an object keeps each key at its first position with its
// last value, a number without a fraction or exponent that fits an Int is
// Int, and every other number is a Float written as encode writes one.
func xoCanonical(text string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	for i := int(dec.InputOffset()); i < len(text); i++ {
		if c := text[i]; c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return "", fmt.Errorf("invalid character %q after top-level value", c)
		}
	}
	return xoCanonicalValue(v)
}

func xoCanonicalValue(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "null", nil
	case bool:
		return strconv.FormatBool(x), nil
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return strconv.FormatInt(n, 10), nil
			}
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return "", err
		}
		return jsonFloatText(math.Float64bits(f)), nil
	case string:
		return jsonGo(x), nil
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			s, err := xoCanonicalValue(e)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	case map[string]any:
		// Go's map has no order; the corpus passes objects through a
		// decoder that keeps the text's order, so the ordered case is built
		// by xoCanonicalOrdered instead
		return "", fmt.Errorf("unordered object")
	}
	return "", fmt.Errorf("unexpected %T", v)
}

// xoCanonicalOrdered is xoCanonical for an object whose keys keep the order of
// the text, each at its first position with its last value (decision 0299).
// It walks the tokens, so duplicate keys and order are what the text says.
func xoCanonicalOrdered(text string) (string, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	s, err := xoCanonicalTokens(dec)
	if err != nil {
		return "", err
	}
	for i := int(dec.InputOffset()); i < len(text); i++ {
		if c := text[i]; c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return "", fmt.Errorf("invalid character %q after top-level value", c)
		}
	}
	return s, nil
}

func xoCanonicalTokens(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '[':
			var parts []string
			for dec.More() {
				s, err := xoCanonicalTokens(dec)
				if err != nil {
					return "", err
				}
				parts = append(parts, s)
			}
			if _, err := dec.Token(); err != nil {
				return "", err
			}
			return "[" + strings.Join(parts, ",") + "]", nil
		case '{':
			var keys []string
			vals := map[string]string{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return "", err
				}
				k, _ := kt.(string)
				s, err := xoCanonicalTokens(dec)
				if err != nil {
					return "", err
				}
				if _, seen := vals[k]; !seen {
					keys = append(keys, k)
				}
				vals[k] = s
			}
			if _, err := dec.Token(); err != nil {
				return "", err
			}
			var parts []string
			for _, k := range keys {
				parts = append(parts, jsonGo(k)+":"+vals[k])
			}
			return "{" + strings.Join(parts, ",") + "}", nil
		}
	case json.Number:
		s := t.String()
		if !strings.ContainsAny(s, ".eE") {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return strconv.FormatInt(n, 10), nil
			}
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return "", err
		}
		return jsonFloatText(math.Float64bits(f)), nil
	case string:
		return jsonGo(t), nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		return "null", nil
	}
	return "", fmt.Errorf("unexpected %T", tok)
}

// ptr helpers build the Option twins of the corpus's Option fields.
func ptrInt(n int64) *int64      { return &n }
func ptrStr(s string) *string    { return &s }
func ptrBool(b bool) *bool       { return &b }
func ptrInts(n []int64) *[]int64 { return &n }

// decodeRow is one typed decode case: the text, whether Go's decoder takes it,
// and the Go twin of what it decodes to (nil for an error row).
type decodeRow struct {
	text  string
	ok    bool
	value any
	why   string // a labelled difference, or ""
}

func jsonDecodeCorpus() string {
	var o strings.Builder
	o.WriteString(header("encoding/json decoding into Go twins of the corpus's types — the text std/rt/json.go encodes, so a decode that differs shows — for std/json's decode and decode_str, then parse_value and from_value against the Value rules of decision 0299 (Int or Float, object key order, duplicate keys) and the malformed texts Go's scanner refuses; Time and Duration fields as std.md 5 and decision 0364 read them (Go's encoding/json tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/json\n")

	o.WriteString(`
// The types the decode cases use, the same shapes as the encode corpus.
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

struct Defaulted {
  n: Int = 7
  s: Str = "d"
} derive Json

struct Nested {
  inner: Inner
  list: List[Inner]
} derive Json

struct Patched {
  p: json.Patch[Int]
  q: Int
} derive Json
`)

	// Go twins of those types.
	type goScalars struct {
		I int64   `json:"i"`
		F float64 `json:"f"`
		B bool    `json:"b"`
		S string  `json:"s"`
	}
	type goOpts struct {
		A *int64   `json:"a"`
		B *string  `json:"b"`
		C *bool    `json:"c"`
		D *[]int64 `json:"d"`
	}
	type goInner struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	type goCircle struct {
		R float64 `json:"r"`
	}
	type goRect struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	type goEnums struct {
		Lvl   string         `json:"lvl"`
		Shape map[string]any `json:"shape"`
	}
	type goCollections struct {
		L    []string         `json:"l"`
		St   []string         `json:"st"`
		M    map[string]int64 `json:"m"`
		Pair []any            `json:"pair"`
	}
	type goNested struct {
		Inner goInner   `json:"inner"`
		List  []goInner `json:"list"`
	}

	scalars := []decodeRow{
		{text: `{"i":1,"f":1.5,"b":true,"s":"x"}`, ok: true, value: goScalars{1, 1.5, true, "x"}},
		{text: `{"i":-1,"f":-0.0,"b":false,"s":""}`, ok: true, value: goScalars{-1, math.Copysign(0, -1), false, ""}},
		{text: `{"i":0,"f":1e21,"b":false,"s":"tab\there"}`, ok: true, value: goScalars{0, 1e21, false, "tab\there"}},
		{text: ` {"i": 2 , "f": 0.5 , "b": true , "s": "y" } `, ok: true, value: goScalars{2, 0.5, true, "y"}},
		{text: `{"i":9223372036854775807,"f":5e-324,"b":true,"s":"z"}`, ok: true, value: goScalars{math.MaxInt64, 5e-324, true, "z"}},
		{text: `{"i":-9223372036854775808,"f":1.7976931348623157e+308,"b":true,"s":""}`, ok: true, value: goScalars{math.MinInt64, math.MaxFloat64, true, ""}},
		// unknown keys are ignored (std.md 5)
		{text: `{"i":1,"f":1.5,"b":true,"s":"x","extra":123,"more":[1]}`, ok: true, value: goScalars{1, 1.5, true, "x"}},
		// the last of a repeated key wins, as in Go
		{text: `{"i":1,"i":2,"f":1.5,"b":true,"s":"x"}`, ok: true, value: goScalars{2, 1.5, true, "x"}},
		{text: `{"i":1,"f":1.5,"b":true}`, ok: false, why: "a missing field with no default is an error (std.md 5)"},
		{text: `{"i":null,"f":1.5,"b":true,"s":"x"}`, ok: false, why: "null into a non-Option field is an error"},
		{text: `{"i":1.0,"f":1.5,"b":true,"s":"x"}`, ok: false, why: "Go refuses 1.0 for an int64; Xo is checked against it"},
		{text: `{"i":"1","f":1.5,"b":true,"s":"x"}`, ok: false},
		{text: `{"i":true,"f":1.5,"b":true,"s":"x"}`, ok: false},
		{text: `{"i":1,"f":"1.5","b":true,"s":"x"}`, ok: false},
		{text: `{"i":1,"f":1.5,"b":1,"s":"x"}`, ok: false},
		{text: `{"i":9223372036854775808,"f":1.5,"b":true,"s":"x"}`, ok: false, why: "out of range (decision 0287's None rule for parse)"},
		{text: `{"i":1e3,"f":1.5,"b":true,"s":"x"}`, ok: false},
		{text: `[]`, ok: false},
		{text: `{"i":1,"f":1.5,"b":true,"s":"x"`, ok: false},
	}
	opts := []decodeRow{
		{text: `{"a":1,"b":"x","c":true,"d":[1,2]}`, ok: true, value: goOpts{ptrInt(1), ptrStr("x"), ptrBool(true), ptrInts([]int64{1, 2})}},
		{text: `{"a":null,"b":null,"c":null,"d":null}`, ok: true, value: goOpts{}},
		{text: `{"a":0,"b":"","c":false,"d":[]}`, ok: true, value: goOpts{ptrInt(0), ptrStr(""), ptrBool(false), ptrInts([]int64{})}},
		{text: `{}`, ok: true, value: goOpts{}},
		{text: `{"a":1}`, ok: true, value: goOpts{A: ptrInt(1)}},
		{text: `{"a":"nope"}`, ok: false},
	}
	enums := []decodeRow{
		{text: `{"lvl":"Low","shape":{"Circle":{"r":1.5}}}`, ok: true, value: goEnums{"Low", map[string]any{"Circle": goCircle{1.5}}}},
		{text: `{"lvl":"High","shape":{"Rect":{"w":1,"h":2}}}`, ok: true, value: goEnums{"High", map[string]any{"Rect": goRect{1, 2}}}},
		{text: `{"lvl":"Nope","shape":{"Circle":{"r":1}}}`, ok: false, why: "an unknown variant is an error"},
		{text: `{"lvl":"Low","shape":{"Circle":{"r":"x"}}}`, ok: false},
		{text: `{"lvl":"Low","shape":{"Circle":{}}}`, ok: false},
		{text: `{"lvl":"Low"}`, ok: false},
	}
	colls := []decodeRow{
		{text: `{"l":["a","b"],"st":["x","y"],"m":{"a":1,"b":2},"pair":["k",-1]}`, ok: true,
			value: goCollections{L: []string{"a", "b"}, St: []string{"x", "y"}, M: map[string]int64{"a": 1, "b": 2}, Pair: []any{"k", int64(-1)}}},
		{text: `{"l":[],"st":[],"m":{},"pair":["",0]}`, ok: true,
			value: goCollections{L: []string{}, St: []string{}, M: map[string]int64{}, Pair: []any{"", int64(0)}}},
		// a repeated key in a map: the last value, at the first position
		{text: `{"l":[],"st":[],"m":{"k":1,"k":2},"pair":["",0]}`, ok: true, why: "an object in a Map keeps a repeated key once, with its last value (std.md 5)",
			value: goCollections{L: []string{}, St: []string{}, M: map[string]int64{"k": 2}, Pair: []any{"", int64(0)}}},
		{text: `{"l":[],"st":[],"m":{},"pair":["a"]}`, ok: false, why: "a tuple needs exactly its length (decision 0088)"},
		{text: `{"l":[],"st":[],"m":{},"pair":["a",1,2]}`, ok: false, why: "a tuple needs exactly its length (decision 0088)"},
		{text: `{"l":[],"st":[],"m":{"k":"v"},"pair":["",0]}`, ok: false},
		{text: `{"l":[],"st":[],"m":{},"pair":[1,0]}`, ok: false},
	}
	nested := []decodeRow{
		{text: `{"inner":{"id":1,"name":"n"},"list":[]}`, ok: true, value: goNested{goInner{1, "n"}, []goInner{}}},
		{text: `{"inner":{"id":0,"name":""},"list":[{"id":2,"name":"m"}]}`, ok: true, value: goNested{goInner{0, ""}, []goInner{{2, "m"}}}},
		{text: `{"inner":{"id":1},"list":[]}`, ok: false},
		{text: `{"inner":{"id":1,"name":"n"},"list":[{"id":2}]}`, ok: false},
	}
	_ = nested

	tables := []struct {
		name  string
		typ   string
		rows  []decodeRow
		kinds string
	}{
		{"SCALARS_CASES", "Scalars", scalars, "both"},
		{"OPTS_CASES", "Opts", opts, "both"},
		{"ENUMS_CASES", "Enums", enums, "both"},
		{"COLLECTIONS_CASES", "Collections", colls, "both"},
		{"NESTED_CASES", "Nested", nested, "both"},
	}
	for _, t := range tables {
		o.WriteString("\n/// (JSON text, whether Go's decoder takes it, the Go twin of the value re-encoded, and why a row reads differently from Go where it does)\n")
		fmt.Fprintf(&o, "const %s: List[(Str, Bool, Str, Str)] = [\n", t.name)
		for _, r := range t.rows {
			want := ""
			if r.ok {
				want = jsonGo(r.value)
			}
			fmt.Fprintf(&o, "  (%s, %v, %s, %s),\n", xq(r.text), r.ok, xq(want), xq(r.why))
		}
		o.WriteString("]\n")
	}

	// Special: Bytes, Duration, Time and a newtype. Go has no Duration that
	// decodes from a string, so its twin holds the parsed Duration's own
	// text, which is what Xo's encode writes back.
	type goSpecial struct {
		Bytes []byte `json:"bytes"`
		Dur   string `json:"dur"`
		When  string `json:"when"`
		Mail  string `json:"mail"`
	}
	special := []struct {
		text string
		ok   bool
		b    []byte
		dur  string
		when string
		mail string
		why  string
	}{
		{text: `{"bytes":"AAH/","dur":"1.5s","when":"2026-10-24T12:34:56Z","mail":"a@b"}`,
			ok: true, b: []byte{0, 1, 0xff}, dur: "1.5s", when: "2026-10-24T12:34:56Z", mail: "a@b"},
		{text: `{"bytes":"","dur":"0s","when":"1970-01-01T00:00:00Z","mail":""}`,
			ok: true, b: []byte{}, dur: "0s", when: "1970-01-01T00:00:00Z", mail: ""},
		{text: `{"bytes":"aGVsbG8=","dur":"1500ms","when":"1999-12-31T23:59:59Z","mail":"x@y.z"}`,
			ok: true, b: []byte("hello"), dur: "1.5s", when: "1999-12-31T23:59:59Z", mail: "x@y.z",
			why: "a Duration decodes from any Duration.parse form and encodes as its own text (std.md 5)"},
		{text: `{"bytes":"","dur":"1h0m0s","when":"2026-10-24T12:34:56.123456789Z","mail":""}`,
			ok: true, b: []byte{}, dur: "1h0m0s", when: "2026-10-24T12:34:56.123456789Z", mail: "",
			why: "a fractional second keeps its nanoseconds"},
		{text: `{"bytes":"","dur":"1.5s","when":"2026-10-24T12:34:56+02:00","mail":""}`,
			ok: true, b: []byte{}, dur: "1.5s", when: "2026-10-24T10:34:56Z", mail: "",
			why: "a Time decodes from an offset and reads in UTC"},
		{text: `{"bytes":"!!","dur":"1.5s","when":"2026-10-24T12:34:56Z","mail":""}`,
			ok: false, why: "Bytes is base64 (std.md 5)"},
		{text: `{"bytes":"","dur":"bogus","when":"2026-10-24T12:34:56Z","mail":""}`, ok: false},
		{text: `{"bytes":"","dur":"1.5s","when":"2026-10-24 12:34:56","mail":""}`, ok: false},
		{text: `{"bytes":"","dur":"1.5s","when":"1677-09-21T00:12:43.145224191Z","mail":""}`,
			ok: false, why: "a Time outside the range of Time is refused (decision 0364)"},
		{text: `{"bytes":"","dur":"1.5s","when":"2262-04-11T23:47:16.854775808Z","mail":""}`,
			ok: false, why: "a Time outside the range of Time is refused (decision 0364)"},
		{text: `{"bytes":"","dur":"1.5s","when":"2026-10-24T12:34:56Z","mail":"a@b","x":1}`,
			ok: true, b: []byte{}, dur: "1.5s", when: "2026-10-24T12:34:56Z", mail: "a@b"},
	}
	o.WriteString("\n/// (JSON text, whether Go's decoder takes it, the Go twin re-encoded, and why a row reads differently from Go where it does; a Duration's twin holds the text it writes back, and a Time's the RFC 3339 text in UTC)\nconst SPECIAL_CASES: List[(Str, Bool, Str, Str)] = [\n")
	for _, r := range special {
		want := ""
		if r.ok {
			want = jsonGo(goSpecial{Bytes: r.b, Dur: r.dur, When: r.when, Mail: r.mail})
		}
		fmt.Fprintf(&o, "  (%s, %v, %s, %s),\n", xq(r.text), r.ok, xq(want), xq(r.why))
	}
	o.WriteString("]\n")

	// Defaults: a missing field takes its default (std.md 5).
	type goDefaulted struct {
		N int64  `json:"n"`
		S string `json:"s"`
	}
	defs := []struct {
		text string
		ok   bool
		val  goDefaulted
	}{
		{text: `{"n":1,"s":"x"}`, ok: true, val: goDefaulted{1, "x"}},
		{text: `{}`, ok: true, val: goDefaulted{7, "d"}},
		{text: `{"n":0}`, ok: true, val: goDefaulted{0, "d"}},
		{text: `{"s":""}`, ok: true, val: goDefaulted{7, ""}},
		{text: `{"n":1,"s":"x","n":2}`, ok: true, val: goDefaulted{2, "x"}},
	}
	o.WriteString("\n/// (JSON text, whether Go's decoder takes it, the Go twin re-encoded)\nconst DEFAULT_CASES: List[(Str, Bool, Str)] = [\n")
	for _, r := range defs {
		want := ""
		if r.ok {
			want = jsonGo(r.val)
		}
		fmt.Fprintf(&o, "  (%s, %v, %s),\n", xq(r.text), r.ok, xq(want))
	}
	o.WriteString("]\n")

	// Patch: a missing key is Absent, null is Null, a value is Value(v).
	o.WriteString("\n/// (JSON text, what the Patch field reads as, and what q reads as)\nconst PATCH_CASES: List[(Str, Str, Int)] = [\n")
	for _, r := range []struct {
		text string
		p    string
		q    int64
	}{
		{`{"q":1}`, "absent", 1},
		{`{"p":null,"q":2}`, "null", 2},
		{`{"p":5,"q":3}`, "value 5", 3},
		{`{"p":0,"q":4}`, "value 0", 4},
	} {
		fmt.Fprintf(&o, "  (%s, %s, %d),\n", xq(r.text), xq(r.p), r.q)
	}
	o.WriteString("]\n")

	// Value: the texts std/json reads into a Value, and the ones it refuses.
	// A canonical text is decision 0299's reading, not Go's, which is why
	// duplicates, -0 and an exponent all have their own rows.
	type valueRow struct {
		text string
		ok   bool
		why  string
	}
	valueRows := []valueRow{
		{text: `null`, ok: true},
		{text: `true`, ok: true},
		{text: `false`, ok: true},
		{text: `0`, ok: true},
		{text: `-1`, ok: true},
		{text: `9223372036854775807`, ok: true},
		{text: `1.5`, ok: true},
		{text: `-0.0`, ok: true},
		{text: `"a"`, ok: true},
		{text: `""`, ok: true},
		{text: `[]`, ok: true},
		{text: `[1,2,3]`, ok: true},
		{text: `[[[]]]`, ok: true},
		{text: `{}`, ok: true},
		{text: `{"a":1}`, ok: true},
		{text: `{"a":1,"b":2}`, ok: true},
		{text: `{"a":{"b":[true,null,{"c":"d"}]}}`, ok: true},
		{text: ` {"a" : 1 } `, ok: true},
		// decision 0299's Int rule and key order
		{text: `-0`, ok: true, why: "Value's Int rule: -0 is Int 0 and encodes 0, where Go reads float64 and keeps the sign (decision 0299)"},
		{text: `1e3`, ok: true, why: "an exponent is a Num: 1e3 is 1000.0 and encodes 1000, where Go with UseNumber keeps the text"},
		{text: `9223372036854775808`, ok: true, why: "one past Int: a Num, so it encodes as the Float it reads (Go with UseNumber keeps the digits)"},
		{text: `0.1`, ok: true},
		{text: `1e21`, ok: true},
		{text: `1e-7`, ok: true},
		{text: `{"b":1,"a":2}`, ok: true, why: "an object keeps the text's key order, where Go's map has none (decision 0299)"},
		{text: `{"k":1,"j":2,"k":3}`, ok: true, why: "a repeated key keeps its first position and its last value (decision 0299)"},
		{text: `"A"`, ok: true, why: "an escape is read to the rune and written back as itself"},
		{text: `"a b"`, ok: true, why: "a line separator is escaped again by encode"},
		{text: `"tab\there\n"`, ok: true},
		// malformed
		{text: ``, ok: false, why: "no value"},
		{text: `   `, ok: false},
		{text: `{`, ok: false},
		{text: `[`, ok: false},
		{text: `{"a":}`, ok: false},
		{text: `{"a" 1}`, ok: false},
		{text: `{a:1}`, ok: false},
		{text: `{"a":1,}`, ok: false},
		{text: `[1,]`, ok: false},
		{text: `nul`, ok: false},
		{text: `tru`, ok: false},
		{text: `True`, ok: false},
		{text: `01`, ok: false},
		{text: `+1`, ok: false},
		{text: `.5`, ok: false},
		{text: `1.`, ok: false},
		{text: `1e`, ok: false},
		{text: `1e+`, ok: false},
		{text: `0x10`, ok: false},
		{text: `1 2`, ok: false, why: "one value and white space (RFC 8259)"},
		{text: `{"a":1} {"b":2}`, ok: false},
		{text: `"unterminated`, ok: false},
		{text: "\"bad \xff utf8\"", ok: true, why: "a raw invalid byte in a string reads as U+FFFD, as in Go (std.md 5.2)"},
	}
	o.WriteString("\n/// (JSON text, whether parse_value takes it, its canonical text where it does — decision 0299's reading — and why a row reads differently from Go where it does)\nconst VALUE_CASES: List[(Str, Bool, Str, Str)] = [\n")
	for _, r := range valueRows {
		want := ""
		if r.ok {
			s, err := xoCanonicalOrdered(r.text)
			if err != nil {
				fmt.Fprintf(os.Stderr, "stdcorpus: the generator cannot canonically read the json text %q: %v\n", r.text, err)
				os.Exit(1)
			}
			want = s
		}
		fmt.Fprintf(&o, "  (%s, %v, %s, %s),\n", xq(r.text), r.ok, xq(want), xq(r.why))
	}
	o.WriteString("]\n")

	// Deep nesting: one array per level, and an object per level, at the
	// depths Go's scanner takes. The expectation is Go's Unmarshal, whose
	// scanner stops past 10000 (encoding/json's jsMaxNestingDepth); the
	// Go backend's own scanner ports that limit. (At 10001 and deeper the
	// native backends take the text where these two refuse it: a finding
	// reported with this corpus, not a row, since no single expectation
	// holds on all three backends.)
	o.WriteString("\n/// (JSON text, whether Go's encoding/json takes it)\nconst DEPTH_CASES: List[(Str, Bool)] = [\n")
	for _, n := range []int{1, 2, 10, 100, 1000, 9999, 10000} {
		text := strings.Repeat("[", n) + strings.Repeat("]", n)
		var v any
		fmt.Fprintf(&o, "  (%s, %v),\n", xq(text), json.Unmarshal([]byte(text), &v) == nil)
	}
	text := strings.Repeat(`{"a":`, 500) + "1" + strings.Repeat("}", 500)
	var v any
	fmt.Fprintf(&o, "  (%s, %v),\n", xq(text), json.Unmarshal([]byte(text), &v) == nil)
	o.WriteString("]\n")

	// The tests.
	// from_value reads a Value back into a type as decode reads its text, so
	// the Scalars rows that decode are the ones it runs over.
	o.WriteString("\n/// (JSON text, the Go twin re-encoded)\nconst SCALARS_VALUES: List[(Str, Str)] = [\n")
	for _, r := range scalars {
		if r.ok {
			fmt.Fprintf(&o, "  (%s, %s),\n", xq(r.text), xq(jsonGo(r.value)))
		}
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

/// One typed decode: the value re-encoded, or !error.
fn typed_got[T: Json](text: Str) -> Str {
  match json.decode_str[T](text) {
    Ok(v) => json.encode_str(v)
    Err(e) => "!error ${e.path}: ${e.msg}"
  }
}
`)
	for _, t := range []struct{ name, typ string }{
		{"SCALARS_CASES", "Scalars"},
		{"OPTS_CASES", "Opts"},
		{"ENUMS_CASES", "Enums"},
		{"COLLECTIONS_CASES", "Collections"},
		{"NESTED_CASES", "Nested"},
		{"SPECIAL_CASES", "Special"},
	} {
		label := "a struct's fields"
		switch t.name {
		case "OPTS_CASES":
			label = "Option fields"
		case "ENUMS_CASES":
			label = "enums"
		case "COLLECTIONS_CASES":
			label = "lists, sets, maps and tuples"
		case "NESTED_CASES":
			label = "nesting"
		case "SPECIAL_CASES":
			label = "Bytes, Duration, Time and a newtype"
		}
		fmt.Fprintf(&o, `
test "decode of %s is Go's, then re-encodes as Go does" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want, why) in %s {
    let got = match json.decode_str[%s](text) {
      Ok(v) => json.encode_str(v)
      Err(_) => "!error"
    }
    let want_text = if ok { want } else { "!error" }
    if got != want_text {
      bad.push(("${text} (${why})", got, want_text))
    }
  }
  expect(show3(bad)) == []
}
`, label, t.name, t.typ)
	}

	o.WriteString(`
test "a missing field takes its default" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want) in DEFAULT_CASES {
    let got = match json.decode_str[Defaulted](text) {
      Ok(v) => json.encode_str(v)
      Err(_) => "!error"
    }
    let want_text = if ok { want } else { "!error" }
    if got != want_text {
      bad.push((text, got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "a Patch field is Absent, Null or Value" {
  var bad: List[(Str, Str, Str)] = []
  for (text, want_p, want_q) in PATCH_CASES {
    let got = match json.decode_str[Patched](text) {
      Ok(v) => match v.p {
        json.Patch.Absent => "absent ${v.q}"
        json.Patch.Null => "null ${v.q}"
        json.Patch.Value(x) => "value ${x} ${v.q}"
      }
      Err(_) => "!error"
    }
    let want = "${want_p} ${want_q}"
    if got != want {
      bad.push((text, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "parse_value then encode is decision 0299's reading" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want, why) in VALUE_CASES {
    let got = match json.parse_value(text) {
      Ok(v) => v.encode()
      Err(_) => "!error"
    }
    let want_text = if ok { want } else { "!error" }
    if got != want_text {
      bad.push(("${text} (${why})", got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "parse_value takes the depths Go's scanner takes" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok) in DEPTH_CASES {
    let got = match json.parse_value(text) {
      Ok(_) => "ok"
      Err(_) => "!error"
    }
    let want_text = if ok { "ok" } else { "!error" }
    if got != want_text {
      bad.push(("depth ${text.len()}", got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "from_value of parse_value gives the same encoding" {
  var bad: List[(Str, Str, Str)] = []
  for (text, want) in SCALARS_VALUES {
    let got = match json.parse_value(text) {
      Ok(v) => match json.from_value[Scalars](v) {
        Ok(s) => json.encode_str(s)
        Err(_) => "!value"
      }
      Err(_) => "!error"
    }
    if got != want {
      bad.push((text, got, want))
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
