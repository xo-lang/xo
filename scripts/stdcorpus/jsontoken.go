package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

// jsonTokenResult is what Go's Decoder.Token (UseNumber) gives on s: the
// tokens rendered ({ } [ ] K:key S:str N:number B:bool null), the input
// offset after each, whether More() was true before each, how it ended
// ("" at a clean end, else "ERR invalid JSON: " and Go's text; an end
// inside an array or object, where Go returns io.EOF, is unexpected EOF),
// and, for a clean end, the tokens written back with encoding/json's string
// forms (HTML escaping on), the values separated by a newline.
func jsonTokenResult(s []byte) (toks []string, offs []int, mores, status, rewritten string) {
	dec := json.NewDecoder(bytes.NewReader(s))
	dec.UseNumber()
	var stack []byte
	var key []bool // per open level: the next string is a key (objects only)
	var w strings.Builder
	var count []int // per open level: members written
	top := 0
	esc := func(s string) string {
		var b bytes.Buffer
		e := json.NewEncoder(&b)
		e.Encode(s)
		return strings.TrimSuffix(b.String(), "\n")
	}
	// the comma, or the top level newline, before a value
	begin := func() {
		if len(stack) == 0 {
			if top > 0 {
				w.WriteByte('\n')
			}
			top++
			return
		}
		if stack[len(stack)-1] == '[' {
			if count[len(count)-1] > 0 {
				w.WriteByte(',')
			}
			count[len(count)-1]++
		}
	}
	done := func() {
		if len(stack) > 0 && stack[len(stack)-1] == '{' {
			key[len(key)-1] = true
		}
	}
	for {
		var m byte = '0'
		if dec.More() {
			m = '1'
		}
		tok, err := dec.Token()
		if err == io.EOF {
			if len(stack) > 0 {
				return toks, offs, mores, "ERR invalid JSON: unexpected EOF", ""
			}
			return toks, offs, mores, "", w.String()
		}
		if err != nil {
			return toks, offs, mores, "ERR invalid JSON: " + err.Error(), ""
		}
		mores += string(m)
		offs = append(offs, int(dec.InputOffset()))
		switch t := tok.(type) {
		case json.Delim:
			toks = append(toks, string(t))
			switch t {
			case '{', '[':
				begin()
				w.WriteByte(byte(t))
				stack = append(stack, byte(t))
				key = append(key, t == '{')
				count = append(count, 0)
			default:
				w.WriteByte(byte(t))
				stack = stack[:len(stack)-1]
				key = key[:len(key)-1]
				count = count[:len(count)-1]
				done()
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1] == '{' && key[len(key)-1] {
				toks = append(toks, "K:"+t)
				if count[len(count)-1] > 0 {
					w.WriteByte(',')
				}
				count[len(count)-1]++
				w.WriteString(esc(t) + ":")
				key[len(key)-1] = false
			} else {
				toks = append(toks, "S:"+t)
				begin()
				w.WriteString(esc(t))
				done()
			}
		case json.Number:
			toks = append(toks, "N:"+string(t))
			begin()
			w.WriteString(string(t))
			done()
		case bool:
			toks = append(toks, "B:"+strconv.FormatBool(t))
			begin()
			w.WriteString(strconv.FormatBool(t))
			done()
		case nil:
			toks = append(toks, "null")
			begin()
			w.WriteString("null")
			done()
		}
	}
}

var jsonTokenBad = []string{
	``, ` `, `{"a" 1}`, `[1 2]`, `{,}`, `["a",]`, `{"a":1,}`, `:`, `,`, `[1,,2]`, `"\x"`, `"abc`, `tru`, `-`, `1.`, `1e`, `[`, `{"a"`, `{"a":`,
	`nul`, `"\ud800"`, `"\u00zz"`, "\"a\x01b\"", `[1]]`, `{}}`, `{"a":1 "b":2}`, `{1:2}`, `{"a"::1}`, `[:]`, `["a":1]`, `}`, `]`, `{"a":}`,
	"\"\xff\xfe\"", "[\"a\xc3\"]", `"😀 é \/"`, `{"<>&":"<>& "}`, `[1,2,3]x`, `01`, `-01`, `1.5.5`, `[1e5,1E-5,-0,0.0]`, `{"a":{"b":[{"c":null}]}}`,
	`  [ ]  { }  "x"  1  true  `, `{"a":1}{"b":2}`, `truefalse`, `[true,false,null]`, `"` + strings.Repeat("a", 300) + `"`,
}

func jsonTokenCorpus() string {
	r := rand.New(rand.NewSource(5))
	var o strings.Builder
	o.WriteString(header("encoding/json's Decoder.Token (UseNumber), More and InputOffset on seeded random streams, some cut short or with a byte changed, and encoding/json's string and float forms"))
	o.WriteString("use std/json\n\n/// (stream, chunk seed, Go's tokens, the input offset after each, More before each, how Go stopped, the tokens written back with Go's string forms)\nconst STREAMS: List[(Bytes, Int, List[Str], List[Int], Str, Str, Str)] = [\n")
	var inputs [][]byte
	for _, s := range jsonTokenBad {
		inputs = append(inputs, []byte(s))
	}
	for i := 0; i < 1200; i++ {
		var b strings.Builder
		for j, n := 0, r.Intn(6); j < n; j++ {
			if j > 0 || r.Intn(3) == 0 {
				b.WriteString([]string{" ", "\n", "\r\n", "\t", "  \n "}[r.Intn(5)])
			}
			jsonGen(r, &b, 3)
		}
		if r.Intn(3) == 0 {
			b.WriteString([]string{"\n", " ", ""}[r.Intn(3)])
		}
		s := []byte(b.String())
		switch {
		case i%5 == 3 && len(s) > 0:
			s = s[:r.Intn(len(s))]
		case i%5 == 4 && len(s) > 0:
			s = append([]byte{}, s...)
			s[r.Intn(len(s))] = "x}],:\"0 {["[r.Intn(10)]
		case i%11 == 5 && len(s) > 2:
			s = append([]byte{}, s...)
			s[r.Intn(len(s))] = "\xff\xc3<>&\x01"[r.Intn(6)]
		}
		inputs = append(inputs, s)
	}
	for _, s := range inputs {
		toks, offs, mores, status, rw := jsonTokenResult(s)
		qs := make([]string, len(toks))
		for k, v := range toks {
			qs[k] = xq(v)
		}
		os := make([]string, len(offs))
		for k, v := range offs {
			os[k] = strconv.Itoa(v)
		}
		fmt.Fprintf(&o, "  (%s, %d, [%s], [%s], %s, %s, %s),\n", xbytes(s), r.Intn(1<<30), strings.Join(qs, ", "), strings.Join(os, ", "), xq(mores), xq(status), xq(rw))
	}
	o.WriteString("]\n\n")
	// encoding/json's string forms: Marshal with and without HTML escaping
	o.WriteString("/// (string, Go's encoding with HTML escaping, without)\nconst STRINGS: List[(Str, Str, Str)] = [\n")
	alpha := []string{"a", "Z", " ", "\"", "\\", "\n", "\r", "\t", "\x01", "\x1f", "\x7f", "<", ">", "&", "é", "世", " ", " ", "�", "😀", "/", "'", "\u0080", " "}
	for i := 0; i < 400; i++ {
		var b strings.Builder
		for j, n := 0, r.Intn(10); j < n; j++ {
			b.WriteString(alpha[r.Intn(len(alpha))])
		}
		s := b.String()
		enc := func(html bool) string {
			var out bytes.Buffer
			e := json.NewEncoder(&out)
			e.SetEscapeHTML(html)
			e.Encode(s)
			return strings.TrimSuffix(out.String(), "\n")
		}
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(s), xq(enc(true)), xq(enc(false)))
	}
	o.WriteString("]\n\n")
	// floatEncoder: the number forms
	o.WriteString("/// (the float as Float.parse reads it, Go's encoding)\nconst FLOATS: List[(Str, Str)] = [\n")
	fl := []float64{0, 1, -1, 0.1, 0.5, 100, 1e20, 1e21, 1e22, 1e-6, 1e-7, 1.5e-7, 123456789, 123456789.125, 5e-324, math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64, 1.0 / 3, 2.5e-5, 99999999999999999999.0, 1e300, 12345678901234567890, 0.000001, 0.0000001, 3.14159, -2.5e10, 1e6, 1e-9, 4.9e-324, float64(1 << 53), float64(1<<53 + 2)}
	for i := 0; i < 200; i++ {
		fl = append(fl, math.Float64frombits(r.Uint64()&0x7fffffffffffffff)*float64(1-2*r.Intn(2)))
		fl = append(fl, (r.Float64()-0.5)*math.Pow(10, float64(r.Intn(40)-20)))
	}
	for _, f := range fl {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		b, _ := json.Marshal(f)
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(strconv.FormatFloat(f, 'g', -1, 64)), xq(string(b)))
	}
	o.WriteString("]\n")
	o.WriteString(jsonTokenTests)
	return o.String()
}

const jsonTokenTests = `
fn chunks(seed: Int, n: Int) -> List[Int] {
  var x = seed & 0x7fffffff
  var out: List[Int] = []
  var left = n
  while left > 0 {
    x = (x * 1103515245 + 12345) & 0x7fffffff
    var c = 1 + (x >> 8) % 7
    if c > left {
      c = left
    }
    out.push(c)
    left -= c
  }
  out
}

fn show(t: json.Token) -> Str {
  match t {
    ObjectStart => "{"
    ObjectEnd => "}"
    ArrayStart => "["
    ArrayEnd => "]"
    Key(s) => "K:${s}"
    Str(s) => "S:${s}"
    Number(s) => "N:${s}"
    Bool(b) => "B:${b}"
    Null => "null"
  }
}

/// A token written to w, as the oracle's rewrite does: strings and keys by
/// Go's forms, numbers as they were.
fn put(var w: json.JsonWriter, t: json.Token) {
  match t {
    ObjectStart => w.object_start()
    ObjectEnd => w.object_end()
    ArrayStart => w.array_start()
    ArrayEnd => w.array_end()
    Key(s) => w.key(s)
    Str(s) => w.str(s)
    Number(s) => w.number(s).ok() ?? ()
    Bool(b) => w.bool(b)
    Null => w.null()
  }
}

test "json.tokens reads Go's Decoder.Token tokens, offsets and More, and stops where Go stops" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (s, _, want, want_offs, want_more, want_status, _)) in STREAMS.indexed() {
    var r = json.tokens(s)
    var got: List[Str] = []
    var offs: List[Int] = []
    var more = ""
    var status = ""
    while true {
      let m = r.more()
      match r.next() {
        Ok(t) => match t {
          Some(x) => {
            more = more + (if m { "1" } else { "0" })
            got.push(show(x))
            offs.push(r.offset())
          }
          None => break
        }
        Err(e) => {
          status = "ERR ${e.msg}"
          break
        }
      }
    }
    if got != want || offs != want_offs || more != want_more || status != want_status {
      bad.push((i, "${got.len()} ${offs.map(fn(o) { "${o}" }).join(",")} ${more} ${status}: ${got.join(" ")}", "${want.len()} ${want_offs.map(fn(o) { "${o}" }).join(",")} ${want_more} ${want_status}: ${want.join(" ")}"))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "json.TokenReader.write gives the same tokens from a stream written in pieces" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (s, cs, want, _, _, want_status, _)) in STREAMS.indexed() {
    var r = json.token_reader()
    var got: List[Str] = []
    var status = ""
    var at = 0
    for n in chunks(cs, s.len()) {
      match r.write(s[at..at + n]) {
        Ok(ts) => got = got + ts.map(show)
        Err(e) => {
          status = "ERR ${e.msg}"
          break
        }
      }
      at += n
    }
    if status == "" {
      match r.finish() {
        Ok(ts) => got = got + ts.map(show)
        Err(e) => status = "ERR ${e.msg}"
      }
    }
    if status == "" {
      // an error after the tokens finish returned is the next call's
      match r.finish() {
        Ok(_) => ()
        Err(e) => status = "ERR ${e.msg}"
      }
    }
    if got != want || status != want_status {
      bad.push((i, "${got.len()} ${status}: ${got.join(" ")}", "${want.len()} ${want_status}: ${want.join(" ")}"))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "json.JsonWriter writes the tokens back in Go's string forms" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (s, _, _, _, _, status, want)) in STREAMS.indexed() {
    if status != "" {
      continue
    }
    var r = json.tokens(s)
    var w = json.json_writer()
    while true {
      match r.next() {
        Ok(t) => match t {
          Some(x) => put(w, x)
          None => break
        }
        Err(_) => break
      }
    }
    let got = w.finish().to_str() ?? ""
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "json.JsonWriter.str follows encoding/json's strings, with and without HTML escaping" {
  var bad: List[(Str, Str, Str)] = []
  for (s, html, plain) in STRINGS {
    var a = json.json_writer()
    a.str(s)
    var b = json.json_writer(escape_html: false)
    b.str(s)
    let ga = a.finish().to_str() ?? ""
    let gb = b.finish().to_str() ?? ""
    if ga != html || gb != plain {
      bad.push((s, ga, html))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "json.JsonWriter.float follows Go's floatEncoder" {
  var bad: List[(Str, Str, Str)] = []
  for (t, want) in FLOATS {
    let f = Float.parse(t) ?? 0.0
    var w = json.json_writer()
    w.float(f).ok() ?? ()
    let got = w.finish().to_str() ?? ""
    if got != want {
      bad.push((t, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "json.TokenReader.value and skip read the values json.Decoder reads, and JsonWriter.value writes them back" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (s, _, _, _, _, status, _)) in STREAMS.indexed() {
    if status != "" {
      continue
    }
    var d = json.decoder()
    let vs = (d.write(s).ok() ?? []) + (d.finish().ok() ?? [])
    var r = json.tokens(s)
    var k = json.tokens(s)
    var w = json.json_writer(escape_html: false)
    var got: List[Str] = []
    while r.more() {
      match r.value() {
        Ok(v) => {
          got.push(v.encode())
          w.value(v).ok() ?? ()
        }
        Err(e) => {
          got.push("ERR ${e.msg}")
          break
        }
      }
      k.skip().ok() ?? ()
    }
    let want = vs.map(fn(v) { v.encode() })
    let wrote = want.join("\n")
    let out = w.finish().to_str() ?? ""
    let rest = k.next().ok() ?? None
    if got != want || out != wrote || rest != None || k.offset() != r.offset() {
      bad.push((i, "${got.join(" ")} | ${out} | ${k.offset()} ${r.offset()}", want.join(" ")))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

/// The next token of r as show writes it, "none" at the end, "ERR ..." for an error.
fn step(var r: json.TokenReader) -> Str {
  match r.next() {
    Ok(t) => match t {
      Some(x) => show(x)
      None => "none"
    }
    Err(e) => "ERR ${e.msg}"
  }
}

test "json.TokenReader.skip steps over a member's value, and more ends an array" {
  var r = json.tokens(b"{\"a\":[1,{\"b\":2}],\"c\":\"x\",\"d\":null}")
  expect(step(r)) == "{"
  var seen: List[Str] = []
  while r.more() {
    let k = step(r)
    if k == "K:c" {
      seen.push(step(r))
    } else {
      r.skip().ok() ?? ()
      seen.push("skipped ${k}")
    }
  }
  expect(seen) == ["skipped K:a", "S:x", "skipped K:d"]
  expect(step(r)) == "}"
  expect(step(r)) == "none"
  var bad = json.tokens(b"[1}")
  expect(bad.skip().is_err()) == true
  expect(step(bad)) == "["
  expect(step(bad)) == "N:1"
  expect(step(bad)) == "ERR invalid JSON: invalid character '}' after array element"
  var blank = json.tokens(b"  ")
  expect(step(blank)) == "none"
  var empty = json.tokens(b"")
  expect(step(empty)) == "none"
}

test "json.TokenReader refuses a token over max_token, and JsonWriter checks numbers, floats and raw text" {
  var r = json.token_reader(max_token: 8)
  expect(r.write(b"[\"abcdefghij").is_ok()) == true
  expect(r.write(b"").is_err()) == true
  var w = json.json_writer()
  expect(w.number("1.5e3").is_ok()) == true
  expect(w.number("01").is_err()) == true
  expect(w.number("1 ").is_err()) == true
  expect(w.number("").is_err()) == true
  expect(w.number("\"a\"").is_err()) == true
  expect(w.float(0.0 / 0.0).is_err()) == true
  expect(w.float(1.0 / 0.0).is_err()) == true
  expect(w.raw("[1, {\"a\" : \"<x>\"}]").is_ok()) == true
  expect(w.raw("[1,").is_err()) == true
  expect(w.finish().to_str() ?? "") == "1.5e3\n[1,{\"a\":\"\\u003cx\\u003e\"}]"
  var u = json.json_writer(escape_html: false)
  u.object_start()
  u.key("k")
  expect(u.raw("{ \"<\" : 1 }").is_ok()) == true
  u.object_end()
  expect(u.finish().to_str() ?? "") == "{\"k\":{\"<\":1}}"
}

test "json: a Float -0 is written -0 by encode, to_value and the writer, as encoding/json writes it" {
  let z = -0.0
  expect(json.encode(z)) == b"-0"
  var w = json.json_writer()
  expect(w.value(json.to_value(z)).is_ok()) == true
  expect(w.finish().to_str() ?? "") == "-0"
  var p = json.json_writer()
  expect(p.value(json.to_value(0.0)).is_ok()) == true
  expect(p.finish().to_str() ?? "") == "0"
}
`
