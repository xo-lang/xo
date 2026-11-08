package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// The `string` wire option (decision 0555) against Go's encoding/json: the
// struct below carries `json:"x,string"` tags, and every expectation is
// what Go's Marshal and Unmarshal do with it. Encode rows are (value, Go's
// text); decode rows are (input text, Go's re-encoding of what it decoded,
// or "!" when Go refuses the input), the Xo side being json.decode_str then
// json.encode_str. A plain field given null is left out (Go keeps the
// field, Xo's decoder refuses a null Int), and Go's error words are not
// compared, only whether it refuses.

type jsStrGo struct {
	I   int64    `json:"i,string"`
	I8  int8     `json:"i8,string"`
	U   uint64   `json:"u,string"`
	F   float64  `json:"f,string"`
	F32 float32  `json:"f32,string"`
	B   bool     `json:"b,string"`
	S   string   `json:"s,string"`
	OI  *int64   `json:"oi,string"`
	OS  *string  `json:"os,string"`
	OB  *bool    `json:"ob,string"`
	OF  *float64 `json:"of,string"`
}

// jsStrText is Go's encoding of v, as Xo writes it (no HTML escaping).
func jsStrText(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "!"
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// jsStrDecode is Go's re-encoding of what text decodes to, or "!".
func jsStrDecode(text string) string {
	var v jsStrGo
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return "!"
	}
	return jsStrText(v)
}

func jsonStringCorpus() string {
	var o strings.Builder
	o.WriteString(header("encoding/json's `,string` tag option against the `string` wire option of derive Json (decision 0555): scalar fields written as quoted literals, a Str quoted twice, Option fields, omitempty, and decoding of quoted, unquoted, malformed and mutated texts (Go's encoding/json tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString(`
use std/json

struct Q {
  i as "i,string": Int
  i8 as "i8,string": Int8
  u as "u,string": UInt64
  f as "f,string": Float
  f32 as "f32,string": Float32
  b as "b,string": Bool
  s as "s,string": Str
  oi as "oi,string": Int?
  os as "os,string": Str?
  ob as "ob,string": Bool?
  of as "of,string": Float?
} derive Json

struct Omit {
  i as "i,omitempty,string": Int
  s as "s,omitempty,string": Str
  b as "b,omitempty,string": Bool
  f as "f,omitempty,string": Float
  oi as "oi,omitempty,string": Int?
} derive Json

fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}
`)

	// Encode rows.
	ints := []int64{0, 1, -1, 42, math.MaxInt64, math.MinInt64, 9007199254740993}
	strs := []string{"", "x", `a"b`, "tab\t", "<&>", "é✓", " ", "\x00"}
	floats := []float64{0, 1.5, -2.5, 1e21, 1e-7, 123456789, 5e-324, math.MaxFloat64}
	bools := []bool{false, true}
	rs := rand.New(rand.NewSource(555))
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst ENC_CASES: List[(Q, Str)] = [\n")
	n := 0
	for _, i := range ints {
		for _, s := range strs {
			f := floats[n%len(floats)]
			b := bools[n%2]
			var oi *int64
			var os *string
			var ob *bool
			var of *float64
			optX := func(i int64) string { return fmt.Sprintf("Some(%d)", i) }
			oiX, osX, obX, ofX := "None", "None", "None", "None"
			switch n % 3 {
			case 1:
				oi, os, ob, of = &i, &s, &b, &f
				oiX, osX, obX, ofX = optX(i), "Some("+xq(s)+")", fmt.Sprintf("Some(%v)", b), fmt.Sprintf("Some(Float.from_bits(%d))", int64(math.Float64bits(f)))
			case 2:
				z, zs, zb, zf := int64(0), "", false, 0.0
				oi, os, ob, of = &z, &zs, &zb, &zf
				oiX, osX, obX, ofX = "Some(0)", `Some("")`, "Some(false)", "Some(0.0)"
			}
			i8 := int8(rs.Intn(256) - 128)
			u := rs.Uint64()
			f32 := float32(rs.NormFloat64())
			g := jsStrGo{I: i, I8: i8, U: u, F: f, F32: f32, B: b, S: s, OI: oi, OS: os, OB: ob, OF: of}
			fmt.Fprintf(&o, "  (Q{i: %d, i8: %d, u: UInt64.from_bits(%d), f: Float.from_bits(%d), f32: Float32.from_bits(%d), b: %v, s: %s, oi: %s, os: %s, ob: %s, of: %s}, %s),\n",
				i, i8, int64(u), int64(math.Float64bits(f)), math.Float32bits(f32), b, xq(s), oiX, osX, obX, ofX, xq(jsStrText(g)))
			n++
		}
	}
	o.WriteString("]\n")

	// omitempty with string: emptiness is decided before quoting.
	type omitGo struct {
		I  int64   `json:"i,omitempty,string"`
		S  string  `json:"s,omitempty,string"`
		B  bool    `json:"b,omitempty,string"`
		F  float64 `json:"f,omitempty,string"`
		OI *int64  `json:"oi,omitempty,string"`
	}
	o.WriteString("\n/// (value, Go's encoding/json text)\nconst OMIT_CASES: List[(Omit, Str)] = [\n")
	one, zero := int64(1), int64(0)
	for _, c := range []struct {
		g omitGo
		x string
	}{
		{omitGo{}, `Omit{i: 0, s: "", b: false, f: 0.0, oi: None}`},
		{omitGo{I: 5, S: "a", B: true, F: 1.5, OI: &one}, `Omit{i: 5, s: "a", b: true, f: 1.5, oi: Some(1)}`},
		{omitGo{OI: &zero}, `Omit{i: 0, s: "", b: false, f: 0.0, oi: Some(0)}`},
		{omitGo{I: -1}, `Omit{i: -1, s: "", b: false, f: 0.0, oi: None}`},
	} {
		fmt.Fprintf(&o, "  (%s, %s),\n", c.x, xq(jsStrText(c.g)))
	}
	o.WriteString("]\n")

	// Decode rows: single member objects over every field, then mutations.
	type member struct {
		key   string
		texts []string // the JSON text of the member's value
	}
	q := func(s string) string { return jsStrText(s) } // a JSON string literal
	numTexts := []string{"0", "1", "-1", "12", "007", "-0", "1.5", "1e2", "0x10", "+1", " 1", "1 ", "", "-", "9223372036854775807", "9223372036854775808", "-9223372036854775808", "-9223372036854775809", "18446744073709551615", "18446744073709551616", "300", "-129", "127", "128", "255", "null", "true", "\"1\"", "[1]", "{}", "NaN", "Infinity", "1_0"}
	floatTexts := []string{"0", "1.5", "-2.5", "1e21", "1e-7", "2.5e3", "1e400", "-1e400", "1E5", ".5", "5.", "1e", "0x1p3", "+1.5", " 1.5", "1.5 ", "", "null", "true", "NaN", "Inf", "340282366920938463463374607431768211456", "3.4e38", "3.5e38"}
	boolTexts := []string{"true", "false", "True", "TRUE", "1", "0", "yes", "", " true", "true ", "null", "\"true\""}
	strTexts := []string{`"a"`, `""`, `"a\"b"`, `"A"`, `"\n"`, `a`, ``, `"`, `"a`, `null`, `"null"`, `1`, `true`, `[]`, `"\ud800"`, ` "a"`, `"a" `}
	var members []member
	wrap := func(ts []string) []string {
		var out []string
		for _, t := range ts {
			out = append(out, q(t)) // the text as the quoted value
			out = append(out, t)    // and the unquoted value itself
		}
		return out
	}
	members = append(members,
		member{"i", wrap(numTexts)}, member{"i8", wrap(numTexts)}, member{"u", wrap(numTexts)},
		member{"f", wrap(floatTexts)}, member{"f32", wrap(floatTexts)}, member{"b", wrap(boolTexts)},
		member{"s", wrap(strTexts)}, member{"oi", wrap(numTexts)}, member{"os", wrap(strTexts)},
		member{"ob", wrap(boolTexts)}, member{"of", wrap(floatTexts)})
	o.WriteString("\n/// (input text, Go's re-encoding of the decoded value, or \"!\" when Go refuses it)\nconst DEC_CASES: List[(Str, Str)] = [\n")
	plain := map[string]bool{"i": true, "i8": true, "u": true, "f": true, "f32": true, "b": true, "s": true}
	var inputs []string
	for _, m := range members {
		for _, t := range m.texts {
			if plain[m.key] && (t == "null" || t == `"null"`) {
				// a plain field and null: Go keeps the field, Xo refuses it
				continue
			}
			text := fmt.Sprintf(`{"%s":%s}`, m.key, t)
			inputs = append(inputs, text)
		}
	}
	for _, text := range inputs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(text), xq(jsStrDecode(text)))
	}
	o.WriteString("]\n")

	// Mutations of a valid document: one byte replaced, inserted or removed.
	base := jsStrText(jsStrGo{I: 7, I8: -3, U: 9, F: 2.5, F32: 0.5, B: true, S: "hi", OI: &one, OS: new(string), OB: new(bool), OF: new(float64)})
	rm := rand.New(rand.NewSource(559))
	o.WriteString("\n/// (mutated document, Go's re-encoding or \"!\")\nconst MUT_CASES: List[(Str, Str)] = [\n")
	alphabet := []byte(`"\ 0123456789-+.eEtrufalsn{}[],:`)
	for k := 0; k < 400; k++ {
		b := []byte(base)
		for j := 0; j < 1+rm.Intn(2); j++ {
			p := rm.Intn(len(b))
			switch rm.Intn(3) {
			case 0:
				b[p] = alphabet[rm.Intn(len(alphabet))]
			case 1:
				b = append(b[:p], append([]byte{alphabet[rm.Intn(len(alphabet))]}, b[p:]...)...)
			default:
				b = append(b[:p], b[p+1:]...)
			}
		}
		text := string(b)
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(text), xq(jsStrDecode(text)))
	}
	o.WriteString("]\n")

	o.WriteString(`
test "encode of a string option struct is Go's" {
  var bad: List[(Str, Str, Str)] = []
  var i = 0
  for (v, want) in ENC_CASES {
    let got = json.encode_str(v)
    if got != want {
      bad.push(("row ${i}", got, want))
    }
    i += 1
  }
  expect(show3(bad)) == []
}

test "omitempty with the string option is decided before quoting" {
  var bad: List[(Str, Str, Str)] = []
  var i = 0
  for (v, want) in OMIT_CASES {
    let got = json.encode_str(v)
    if got != want {
      bad.push(("row ${i}", got, want))
    }
    i += 1
  }
  expect(show3(bad)) == []
}

fn redecode(text: Str) -> Str {
  match json.decode_str[Q](text) {
    Ok(v) => json.encode_str(v)
    Err(_) => "!"
  }
}

test "decode of quoted, unquoted and malformed members is Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (text, want) in DEC_CASES {
    let got = redecode(text)
    if got != want {
      bad.push((text, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "decode of mutated documents is Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (text, want) in MUT_CASES {
    let got = redecode(text)
    if got != want {
      bad.push((text, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "from_value of to_value reads the quoted form back" {
  var bad: List[(Str, Str, Str)] = []
  for (v, want) in ENC_CASES {
    let got = match json.from_value[Q](json.to_value(v)) {
      Ok(back) => json.encode_str(back)
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("to_value", got, want))
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
