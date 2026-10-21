package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"strings"
)

// jsonGen writes a random JSON value of depth at most d.
func jsonGen(r *rand.Rand, b *strings.Builder, d int) {
	k := r.Intn(9)
	if d <= 0 && k >= 7 {
		k = r.Intn(7)
	}
	switch k {
	case 0:
		b.WriteString([]string{"true", "false", "null"}[r.Intn(3)])
	case 1, 2:
		b.WriteString([]string{"0", "-1", "12", "3.5", "1e3", "-0.25E-2", "9007199254740993", "123456789012"}[r.Intn(8)])
	case 3, 4:
		b.WriteByte('"')
		for i, n := 0, r.Intn(6); i < n; i++ {
			b.WriteString([]string{"a", "Z", " ", `\"`, `\\`, `\n`, `é`, "é", "{", "]", ",", ":"}[r.Intn(12)])
		}
		b.WriteByte('"')
	case 5, 6:
		if d <= 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, n := 0, r.Intn(4); i < n; i++ {
			if i > 0 {
				b.WriteString([]string{",", ", ", " ,\n"}[r.Intn(3)])
			}
			jsonGen(r, b, d-1)
		}
		b.WriteByte(']')
	default:
		b.WriteByte('{')
		for i, n := 0, r.Intn(4); i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, `"k%d"%s:`, r.Intn(5), []string{"", " "}[r.Intn(2)])
			jsonGen(r, b, d-1)
		}
		b.WriteByte('}')
	}
}

// jsonStream is Go's json.Decoder over s: each value's compact text, and
// "" or "err" when it stopped at an error.
func jsonStream(s []byte) ([]string, string) {
	dec := json.NewDecoder(bytes.NewReader(s))
	var vals []string
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if err == io.EOF {
			return vals, ""
		}
		if err != nil {
			return vals, "err"
		}
		var c bytes.Buffer
		json.Compact(&c, raw)
		vals = append(vals, c.String())
	}
}

func jsonCorpus() string {
	r := rand.New(rand.NewSource(3))
	var o strings.Builder
	o.WriteString(header("encoding/json's Decoder reading a stream of values into json.RawMessage until io.EOF, on seeded random streams, some cut short or with a byte changed"))
	o.WriteString("use std/json\n\n/// (stream, chunk seed, Go's values in compact form, Go stopped at an error)\nconst STREAMS: List[(Bytes, Int, List[Str], Bool)] = [\n")
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
		}
		vals, err := jsonStream(s)
		qs := make([]string, len(vals))
		for k, v := range vals {
			qs[k] = xq(v)
		}
		fmt.Fprintf(&o, "  (%s, %d, [%s], %v),\n", xbytes(s), r.Intn(1<<30), strings.Join(qs, ", "), err != "")
	}
	o.WriteString("]\n")
	o.WriteString(jsonTests)
	return o.String()
}

const jsonTests = `
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

test "json.Decoder reads Go's values from a stream written in pieces, and stops where Go stops" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (s, cs, want, want_err)) in STREAMS.indexed() {
    var d = json.decoder()
    var got: List[Str] = []
    var err = false
    var at = 0
    for n in chunks(cs, s.len()) {
      match d.write(s[at..at + n]) {
        Ok(vs) => got = got + vs.map(fn(v) { v.encode() })
        Err(_) => {
          err = true
          break
        }
      }
      at += n
    }
    if !err {
      match d.finish() {
        Ok(vs) => got = got + vs.map(fn(v) { v.encode() })
        Err(_) => err = true
      }
    }
    // Go's values, read as parse_value reads them
    let w = want.map(fn(t) { (json.parse_value(t).ok() ?? json.Value.Null).encode() })
    if got != w || err != want_err {
      bad.push((i, "${got.len()} ${err} ${got.join(" ")}", "${w.len()} ${want_err} ${w.join(" ")}"))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`
