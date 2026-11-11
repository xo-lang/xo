package main

import (
	"bytes"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

// asn1Tree is the std/asn1 tree Go's encoding/asn1 reads from der, in the
// corpus's text form (asn1_test.xo's tree_text): `seq(A,B)`, `int:N`, ...,
// or `err TEXT` with Go's error (`err time` for a time Go refuses: its
// time.Parse texts are not ported).
func asn1Tree(der []byte) (string, []byte) {
	var rv asn1.RawValue
	rest, err := asn1.Unmarshal(der, &rv)
	if err != nil {
		return "err " + asn1ErrText(err), nil
	}
	s, enc, err := asn1Node(rv)
	if err != nil {
		return "err " + asn1ErrText(err), nil
	}
	if len(rest) > 0 {
		return "err asn1: syntax error: trailing data", nil
	}
	return s, enc
}

func asn1ErrText(err error) string {
	t := err.Error()
	if strings.HasPrefix(t, "parsing time") || strings.HasPrefix(t, "asn1: time did not") {
		return "time"
	}
	return t
}

// asn1Node is rv's tree text and its DER as std/asn1's der writes it (nil
// with an error text in the string when Marshal refuses).
func asn1Node(rv asn1.RawValue) (string, []byte, error) {
	raw := func() (string, []byte, error) {
		b, _ := asn1.Marshal(asn1.RawValue{Class: rv.Class, Tag: rv.Tag, IsCompound: rv.IsCompound, Bytes: rv.Bytes})
		return fmt.Sprintf("raw:%d:%d:%v:%x", rv.Class, rv.Tag, rv.IsCompound, rv.Bytes), b, nil
	}
	if rv.Class != 0 {
		return raw()
	}
	if rv.IsCompound {
		if rv.Tag != 16 && rv.Tag != 17 {
			return raw()
		}
		var parts []string
		var encs [][]byte
		rest := rv.Bytes
		for len(rest) > 0 {
			var c asn1.RawValue
			var err error
			rest, err = asn1.Unmarshal(rest, &c)
			if err != nil {
				return "", nil, err
			}
			s, e, err := asn1Node(c)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, s)
			encs = append(encs, e)
		}
		name := "seq"
		if rv.Tag == 17 {
			name = "set"
			sort.Slice(encs, func(i, j int) bool { return bytes.Compare(encs[i], encs[j]) < 0 })
		}
		var enc []byte
		for _, e := range encs {
			if e == nil {
				enc = nil
				break
			}
			enc = append(enc, e...)
		}
		if enc == nil && len(encs) > 0 {
			return name + "(" + strings.Join(parts, ",") + ")", nil, nil
		}
		b, _ := asn1.Marshal(asn1.RawValue{Tag: rv.Tag, IsCompound: true, Bytes: enc})
		return name + "(" + strings.Join(parts, ",") + ")", b, nil
	}
	full := rv.FullBytes
	m := func(v any, params string) []byte {
		b, err := asn1.MarshalWithParams(v, params)
		if err != nil {
			return nil
		}
		return b
	}
	switch rv.Tag {
	case 1:
		var v bool
		if _, err := asn1.Unmarshal(full, &v); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("bool:%v", v), m(v, ""), nil
	case 2:
		var v *big.Int
		if _, err := asn1.Unmarshal(full, &v); err != nil {
			return "", nil, err
		}
		return "int:" + v.String(), m(v, ""), nil
	case 3:
		var v asn1.BitString
		if _, err := asn1.Unmarshal(full, &v); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("bits:%x:%d", v.Bytes, v.BitLength), m(v, ""), nil
	case 4:
		return fmt.Sprintf("octets:%x", rv.Bytes), m(rv.Bytes, ""), nil
	case 5:
		if len(rv.Bytes) == 0 {
			return "null", asn1.NullBytes, nil
		}
		return raw()
	case 6:
		var v asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(full, &v); err != nil {
			return "", nil, err
		}
		return "oid:" + v.String(), m(v, ""), nil
	case 10:
		// read as an INTEGER of up to 64 bits (std/asn1's Enumerated is an Int)
		re := append([]byte{2}, full[1:]...)
		var v int64
		if _, err := asn1.Unmarshal(re, &v); err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("enum:%d", v), m(asn1.Enumerated(v), ""), nil
	case 12, 18, 19, 20, 22, 30, 23, 24:
		var v any
		if _, err := asn1.Unmarshal(full, &v); err != nil {
			return "", nil, err
		}
		names := map[int]string{12: "utf8", 18: "numeric", 19: "printable", 20: "t61", 22: "ia5", 30: "bmp"}
		params := map[int]string{12: "utf8", 18: "numeric", 19: "printable", 22: "ia5"}
		switch x := v.(type) {
		case string:
			var enc []byte
			switch rv.Tag {
			case 20:
				enc, _ = asn1.Marshal(asn1.RawValue{Tag: 20, Bytes: []byte(x)})
			case 30:
				var b []byte
				for _, u := range utf16.Encode([]rune(x)) {
					b = append(b, byte(u>>8), byte(u))
				}
				enc, _ = asn1.Marshal(asn1.RawValue{Tag: 30, Bytes: b})
			default:
				enc = m(x, params[rv.Tag])
			}
			return fmt.Sprintf("%s:%x", names[rv.Tag], x), enc, nil
		case time.Time:
			// std/asn1 holds a Time (1677 to 2262): it refuses a written
			// year outside 1678 to 2261
			if x.Year() < 1678 || x.Year() > 2261 {
				return "", nil, fmt.Errorf("asn1: time out of range")
			}
			u := x.UTC()
			if rv.Tag == 23 {
				return fmt.Sprintf("utc:%d.%09d", u.Unix(), u.Nanosecond()), m(u, "utc"), nil
			}
			return fmt.Sprintf("gen:%d.%09d", u.Unix(), u.Nanosecond()), m(u, "generalized"), nil
		}
		return raw()
	}
	return raw()
}

// asn1Gen is a random tree's DER, Go's Marshal of it (so valid).
func asn1Gen(r *rand.Rand, depth int) []byte {
	mk := func(v any, params string) []byte {
		b, err := asn1.MarshalWithParams(v, params)
		if err != nil {
			panic(err)
		}
		return b
	}
	k := r.Intn(18)
	if depth > 3 && k >= 15 {
		k = r.Intn(15)
	}
	switch k {
	case 0:
		return mk(r.Intn(2) == 0, "")
	case 1:
		n := new(big.Int).Rand(r, new(big.Int).Lsh(big.NewInt(1), uint(r.Intn(140))))
		if r.Intn(2) == 0 {
			n.Neg(n)
		}
		return mk(n, "")
	case 2:
		n := r.Intn(40)
		b := make([]byte, (n+7)/8)
		r.Read(b)
		if n%8 != 0 {
			b[len(b)-1] &= ^byte(0xff >> uint(n%8))
		}
		return mk(asn1.BitString{Bytes: b, BitLength: n}, "")
	case 3:
		b := make([]byte, r.Intn(20))
		r.Read(b)
		return mk(b, "")
	case 4:
		return asn1.NullBytes
	case 5:
		o := asn1.ObjectIdentifier{r.Intn(3), r.Intn(40)}
		for i, n := 0, r.Intn(6); i < n; i++ {
			o = append(o, []int{r.Intn(128), r.Intn(20000), r.Intn(1 << 31)}[r.Intn(3)])
		}
		return mk(o, "")
	case 6:
		return mk(asn1.Enumerated(r.Intn(1000)-500), "")
	case 7:
		return mk(asn1Pick(r, []string{"", "hello", "héllo", "日本", "a*b", "x&y", "ok 123"}), "utf8")
	case 8:
		return mk(asn1Pick(r, []string{"", "Hello World", "a-b.c", "Q?(x)=1"}), "printable")
	case 9:
		return mk(asn1Pick(r, []string{"", "user@example.com", "a\tb"}), "ia5")
	case 10:
		return mk(asn1Pick(r, []string{"", "0123", "1 2 3"}), "numeric")
	case 11:
		t := time.Unix(int64(r.Intn(2400000000))-int64(600000000), 0).UTC()
		return mk(t, "utc")
	case 12:
		t := time.Unix(int64(r.Intn(4000000000))-int64(2000000000), int64(r.Intn(3))*int64(r.Intn(1000000000))).UTC()
		b, err := asn1.MarshalWithParams(t, "generalized")
		if err != nil {
			panic(err)
		}
		return b
	case 13:
		b, _ := asn1.Marshal(asn1.RawValue{Class: 2, Tag: r.Intn(40), IsCompound: r.Intn(2) == 0, Bytes: asn1Gen(r, depth+1)})
		return b
	case 14:
		var b []byte
		for _, u := range utf16.Encode([]rune(asn1Pick(r, []string{"", "AB", "é€", "𝄞x"}))) {
			b = append(b, byte(u>>8), byte(u))
		}
		out, _ := asn1.Marshal(asn1.RawValue{Tag: 30, Bytes: b})
		return out
	default:
		var c []byte
		for i, n := 0, r.Intn(4); i < n; i++ {
			c = append(c, asn1Gen(r, depth+1)...)
		}
		tag := 16
		if k == 17 {
			tag = 17
		}
		b, _ := asn1.Marshal(asn1.RawValue{Tag: tag, IsCompound: true, Bytes: c})
		return b
	}
}

func asn1Pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// asn1Fixed are hand-made inputs for each rule: lengths, tags, integers,
// padding bits, strings, times.
func asn1Fixed() [][]byte {
	hx := func(s string) []byte {
		b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
		if err != nil {
			panic(s)
		}
		return b
	}
	var out [][]byte
	for _, s := range []string{
		"", "30", "3080", "3081", "308100", "30817f", "3082007f", "30820080" + strings.Repeat("00", 128),
		"0400", "048180" + strings.Repeat("aa", 128), "0484ffffffff", "1f1e00", "1f1f00", "1f8000", "1f810000", "1f7f00", "1f8f8f8f8f0f00",
		"0200", "020100", "02020000", "0202007f", "02020080", "0202ff80", "0202ff7f", "0209000000000000000001", "02088000000000000000",
		"010100", "0101ff", "010101", "0100", "01020000",
		"0300", "030100", "030101", "030700", "03020780", "03020781", "030208ff",
		"0600", "0601", "06012a", "06022a80", "060280", "0603813403", "06058fffffff7f", "06058080808001",
		"0a0105", "0a0900ffffffffffffffff", "0a00",
		"0c03e697a5", "0c02c328", "0c00",
		"13024142", "13022a26", "13017e", "1600", "160180", "12023132", "12017a", "14017f", "1401e9",
		"1e00", "1e0100", "1e020041", "1e04d8340041", "1e020000", "1e04004100 00", "1e02fffe",
		"0500", "050100",
		"170d3439313233313233353935395a", "170d3530303130313030303030305a", "170b343931323331323335395a",
		"1711343931323331323335392b30313030", "1711343931323331323335392d30303030", "170d3439313332313233353935395a",
		"170d3439303233303030303030305a", "170d3439313233313234303030305a", "170c34393132333132333539305a",
		"180f32303230313233313233353935395a", "18123230323031323331323335393539 2e32355a",
		"18133230323031323331323335393539 2e3235305a", "18113230323031323331323335393539 2e5a",
		"18133230323031323331323335393539 2c32355a", "180f32303230313233313233353936305a",
		"181332303230313233313233353935392b30313330", "181a3230323031323331323335393539 2e3132333435363738395a",
		"181b3230323031323331323335393539 2e313233343536373839315a",
		"3000", "3100", "3103020101", "310602010502 0101", "30030201", "3005020101", "3003020101 00",
		"a000", "a003020101", "8001ff", "4001aa", "c001aa", "bf1f00", "2400", "2c00", "0d00",
		"30060101ff0500",
	} {
		out = append(out, hx(s))
	}
	return out
}

func asn1Corpus() string {
	r := rand.New(rand.NewSource(601))
	var o strings.Builder
	o.WriteString(header("encoding/asn1's Unmarshal and Marshal on seeded random DER trees, their mutations, and hand-made edge cases"))
	o.WriteString("use std/asn1\n\n")
	o.WriteString("/// (DER, Go's tree as tree_text gives it or `err TEXT`, the DER der() writes as hex or `-` when Go's Marshal refuses)\nconst CASES: List[(Bytes, Str, Str)] = [\n")
	add := func(der []byte) {
		t, enc := asn1Tree(der)
		e := "-"
		if enc != nil {
			e = hex.EncodeToString(enc)
		}
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(der), xq(t), xq(e))
	}
	for _, b := range asn1Fixed() {
		add(b)
	}
	for i := 0; i < 1200; i++ {
		b := asn1Gen(r, 0)
		switch r.Intn(5) {
		case 0, 1:
		case 2:
			if len(b) > 0 {
				c := append([]byte(nil), b...)
				c[r.Intn(len(c))] ^= byte(1 << uint(r.Intn(8)))
				b = c
			}
		case 3:
			if len(b) > 1 {
				b = b[:r.Intn(len(b))]
			}
		case 4:
			if len(b) > 0 {
				c := append([]byte(nil), b...)
				c[r.Intn(len(c))] = byte(r.Intn(256))
				b = c
			}
		}
		add(b)
	}
	o.WriteString("]\n\n/// (bytes, bit length, Go's BitString.At(i) for i in -1 to n as digits, Go's RightAlign as hex)\nconst BITS: List[(Bytes, Int, Str, Str)] = [\n")
	for i := 0; i < 120; i++ {
		n := r.Intn(30)
		b := make([]byte, (n+7)/8)
		r.Read(b)
		if r.Intn(3) == 0 && n > 0 {
			// right_align takes any bytes; At reads them as they are
			n -= r.Intn(n)
		}
		bs := asn1.BitString{Bytes: b, BitLength: n}
		var at strings.Builder
		for k := -1; k <= n; k++ {
			if k/8 < len(b) || k < 0 || k >= n {
				fmt.Fprintf(&at, "%d", bs.At(k))
			}
		}
		fmt.Fprintf(&o, "  (%s, %d, %s, %s),\n", xbytes(b), n, xq(at.String()), xq(hex.EncodeToString(bs.RightAlign())))
	}
	o.WriteString("]\n\n/// (dotted OID, Go's Marshal of its ObjectIdentifier as hex or `-`)\nconst OIDS: List[(Str, Str)] = [\n")
	for i := 0; i < 150; i++ {
		var arcs []int
		for k, n := 0, r.Intn(7); k < n; k++ {
			arcs = append(arcs, []int{r.Intn(4), r.Intn(45), r.Intn(200000), r.Intn(1 << 31)}[r.Intn(4)])
		}
		parts := make([]string, len(arcs))
		for k, a := range arcs {
			parts[k] = fmt.Sprint(a)
		}
		e := "-"
		if b, err := asn1.Marshal(asn1.ObjectIdentifier(arcs)); err == nil {
			e = hex.EncodeToString(b)
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(strings.Join(parts, ".")), xq(e))
	}
	o.WriteString("]\n\n/// (a context specific constructed value, Go's tree of its one inner value, as explicit reads it)\nconst EXPLICIT: List[(Bytes, Str)] = [\n")
	for i := 0; i < 120; i++ {
		inner := asn1Gen(r, 1)
		if r.Intn(4) == 0 && len(inner) > 1 {
			inner = inner[:r.Intn(len(inner))]
		}
		b, _ := asn1.Marshal(asn1.RawValue{Class: 2, Tag: r.Intn(5), IsCompound: true, Bytes: inner})
		t, _ := asn1Tree(inner)
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(b), xq(t))
	}
	o.WriteString("]\n")
	o.WriteString(asn1Tests)
	return o.String()
}

const asn1Tests = `
fn class_num(c: asn1.Class) -> Int {
  match c {
    asn1.Class.Universal => 0
    asn1.Class.Application => 1
    asn1.Class.ContextSpecific => 2
    asn1.Class.Private => 3
  }
}

fn when(t: Time) -> Str {
  let s = t.unix()
  let frac = (t - Time.from_unix(s)) / 1ns
  let f = "${frac}"
  "${s}.${"0".repeat(9 - f.len())}${f}"
}

/// The corpus's text of a tree (the generator's asn1Node).
fn tree_text(v: asn1.Value) -> Str {
  match v {
    asn1.Value.Bool(b) => "bool:${b}"
    asn1.Value.Int(n) => "int:${n}"
    asn1.Value.BitString(b, n) => "bits:${b.to_hex()}:${n}"
    asn1.Value.OctetString(b) => "octets:${b.to_hex()}"
    asn1.Value.Null => "null"
    asn1.Value.Oid(o) => "oid:${o}"
    asn1.Value.Enumerated(n) => "enum:${n}"
    asn1.Value.Utf8(s) => "utf8:${s.bytes().to_hex()}"
    asn1.Value.Printable(s) => "printable:${s.bytes().to_hex()}"
    asn1.Value.Ia5(s) => "ia5:${s.bytes().to_hex()}"
    asn1.Value.Numeric(s) => "numeric:${s.bytes().to_hex()}"
    asn1.Value.T61(s) => "t61:${s.bytes().to_hex()}"
    asn1.Value.Bmp(s) => "bmp:${s.bytes().to_hex()}"
    asn1.Value.UtcTime(t) => "utc:${when(t)}"
    asn1.Value.GeneralizedTime(t) => "gen:${when(t)}"
    asn1.Value.Sequence(items) => "seq(${items.map(tree_text).join(",")})"
    asn1.Value.Set(items) => "set(${items.map(tree_text).join(",")})"
    asn1.Value.Tagged(c, t, e, x) => "tagged:${class_num(c)}:${t}:${e}:${tree_text(x)}"
    asn1.Value.Raw(c, t, k, b) => "raw:${class_num(c)}:${t}:${k}:${b.to_hex()}"
  }
}

fn err_text(e: asn1.Asn1Err) -> Str {
  let t = "${e}"
  if t.starts_with("asn1: invalid UTCTime") || t.starts_with("asn1: invalid GeneralizedTime") {
    return "time"
  }
  t
}

fn first_bad(bad: List[(Bytes, Str, Str)]) -> List[(Bytes, Str, Str)] { bad[..bad.len().min(5)] }

test "parse and der are Go's Unmarshal and Marshal" {
  var bad: List[(Bytes, Str, Str)] = []
  for (der, want, wder) in CASES {
    match asn1.parse(der) {
      Ok(v) => {
        let got = tree_text(v)
        if got != want {
          bad.push((der, got, want))
        }
        let gder = match v.der() {
          Ok(b) => b.to_hex()
          Err(_) => "-"
        }
        if gder != wder {
          bad.push((der, "der ${gder}", wder))
        }
      }
      Err(e) => {
        let got = "err ${err_text(e)}"
        if got != want {
          bad.push((der, got, want))
        }
      }
    }
  }
  expect(first_bad(bad)) == []
}

test "bit_at and right_align are Go's BitString.At and RightAlign" {
  var bad: List[(Bytes, Str, Str)] = []
  for (b, n, at, ra) in BITS {
    let v = asn1.Value.BitString(bytes: b, bit_len: n)
    var got = ""
    for k in -1..n + 1 {
      if k / 8 < b.len() || k < 0 || k >= n {
        got = got + "${v.bit_at(k)}"
      }
    }
    if got != at {
      bad.push((b, got, at))
    }
    let gra = asn1.right_align(b, n).to_hex()
    if gra != ra {
      bad.push((b, gra, ra))
    }
  }
  expect(first_bad(bad)) == []
}

test "oid parses what Go's ObjectIdentifier marshals" {
  var bad: List[(Bytes, Str, Str)] = []
  for (dotted, want) in OIDS {
    let got = match asn1.oid(dotted) {
      Ok(o) => match asn1.Value.Oid(v: o).der() {
        Ok(b) => b.to_hex()
        Err(_) => "-"
      }
      Err(_) => "-"
    }
    if got != want {
      bad.push((dotted.bytes(), got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "tagged_as explicit reads the inner value as Go does" {
  var bad: List[(Bytes, Str, Str)] = []
  for (der, want) in EXPLICIT {
    let raw = match asn1.parse(der) {
      Ok(v) => v
      Err(e) => fault("${e}")
    }
    let got = match raw.tagged_as(true) {
      Ok(asn1.Value.Tagged(_, _, _, x)) => tree_text(x)
      Ok(_) => "not tagged"
      Err(e) => "err ${err_text(e)}"
    }
    if got != want {
      bad.push((der, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
