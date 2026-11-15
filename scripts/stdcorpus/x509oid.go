package main

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	mrand "math/rand"
	"strings"
)

// std/x509's Oid against Go's x509.OID: oid_test.go's tables (oidTests,
// TestInvalidOID, TestOIDMarshal, TestOIDEqualASN1OID, TestOIDFromASN1OID;
// Copyright 2023 The Go Authors, BSD-style license) and random inputs
// near their edges, each with Go's answer.

// oidGoTests is oid_test.go's oidTests: (raw, valid, str, ints).
var oidGoTests = []struct {
	raw   []byte
	valid bool
	str   string
	ints  []uint64
}{
	{[]byte{}, false, "", nil},
	{[]byte{0x80, 0x01}, false, "", nil},
	{[]byte{0x01, 0x80, 0x01}, false, "", nil},
	{[]byte{1, 2, 3}, true, "0.1.2.3", []uint64{0, 1, 2, 3}},
	{[]byte{41, 2, 3}, true, "1.1.2.3", []uint64{1, 1, 2, 3}},
	{[]byte{86, 2, 3}, true, "2.6.2.3", []uint64{2, 6, 2, 3}},
	{[]byte{41, 255, 255, 255, 127}, true, "1.1.268435455", []uint64{1, 1, 268435455}},
	{[]byte{41, 0x87, 255, 255, 255, 127}, true, "1.1.2147483647", []uint64{1, 1, 2147483647}},
	{[]byte{41, 255, 255, 255, 255, 127}, true, "1.1.34359738367", []uint64{1, 1, 34359738367}},
	{[]byte{42, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "1.2.9223372036854775807", []uint64{1, 2, 9223372036854775807}},
	{[]byte{43, 0x81, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "1.3.18446744073709551615", []uint64{1, 3, 18446744073709551615}},
	{[]byte{44, 0x83, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "1.4.36893488147419103231", nil},
	{[]byte{85, 255, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.5.1180591620717411303423", nil},
	{[]byte{85, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.5.19342813113834066795298815", nil},
	{[]byte{255, 255, 255, 127}, true, "2.268435375", []uint64{2, 268435375}},
	{[]byte{0x87, 255, 255, 255, 127}, true, "2.2147483567", []uint64{2, 2147483567}},
	{[]byte{255, 127}, true, "2.16303", []uint64{2, 16303}},
	{[]byte{255, 255, 255, 255, 127}, true, "2.34359738287", []uint64{2, 34359738287}},
	{[]byte{255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.9223372036854775727", []uint64{2, 9223372036854775727}},
	{[]byte{0x81, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.18446744073709551535", []uint64{2, 18446744073709551535}},
	{[]byte{0x83, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.36893488147419103151", nil},
	{[]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.1180591620717411303343", nil},
	{[]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 127}, true, "2.19342813113834066795298735", nil},
	{[]byte{41, 0x80 | 66, 0x80 | 44, 0x80 | 11, 33}, true, "1.1.139134369", []uint64{1, 1, 139134369}},
	{[]byte{0x80 | 66, 0x80 | 44, 0x80 | 11, 33}, true, "2.139134289", []uint64{2, 139134289}},
}

// oidASN1Want is Go's toASN1OID of a valid OID's contents, which is
// unexported: encoding/asn1's parse of the OBJECT IDENTIFIER gives the same
// arcs under the same 31 bit rule.
func oidASN1Want(der []byte) string {
	full := append([]byte{6, byte(len(der))}, der...)
	if len(der) > 127 {
		return "none"
	}
	var a asn1.ObjectIdentifier
	if rest, err := asn1.Unmarshal(full, &a); err != nil || len(rest) > 0 {
		return "none"
	}
	return a.String()
}

// oidWant is "ok HEX TEXT ASN1" for an OID Go made, or "err TEXT".
func oidWant(o x509.OID, err error) string {
	if err != nil {
		return "err " + err.Error()
	}
	b, _ := o.MarshalBinary()
	t, _ := o.MarshalText()
	if string(t) != o.String() {
		panic("MarshalText is not String")
	}
	return fmt.Sprintf("ok %s %s %s", hex.EncodeToString(b), o.String(), oidASN1Want(b))
}

func oidRandArc(r *mrand.Rand) string {
	switch r.Intn(7) {
	case 0:
		return fmt.Sprint(r.Intn(128))
	case 1:
		return fmt.Sprint(r.Intn(1 << 16))
	case 2:
		return fmt.Sprint(math.MaxInt32 - 2 + r.Intn(5))
	case 3:
		return fmt.Sprint(uint64(math.MaxUint64) - uint64(r.Intn(3)))
	case 4:
		n := new(big.Int).Lsh(big.NewInt(1), uint(60+r.Intn(80)))
		return n.Add(n, big.NewInt(int64(r.Intn(1000)-500))).String()
	case 5:
		return "0" + fmt.Sprint(r.Intn(100))
	default:
		return fmt.Sprint(r.Int63())
	}
}

func x509OIDCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/x509 OID against std/x509's Oid (oid_test.go's tables are Copyright 2023 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/x509\n")
	r := mrand.New(mrand.NewSource(6))

	// text: oidTests' strings, TestInvalidOID's, TestOIDMarshal's, and
	// random ones (big arcs, leading zeros, first two at their limits,
	// signs and empty arcs)
	texts := []string{"", "0", "1", "3", "3.100.200", "1.81", "1.81.200", ".1", ".1.", "1.", "1..", "1.2.", "1.2.333.", "1.2.333..", "1.2..", "+1.2", "-1.2", "1.-2", "1.2.+333",
		"0.39", "0.40", "1.39", "1.40", "2.40", "2.0", "2.99999999999999999999999", "02.5", "1.2.3 ", " 1.2.3", "1.2.3\n", "1.2.٣", "1.2.3.4.5.6.7.8.9.10.11.12.13.14.15.16.17.18.19.20",
		"2.47", "2.2147483567", "2.2147483568", "1.39.2147483647", "1.39.2147483648", "0.0", "0.0.0"}
	for _, t := range oidGoTests {
		if t.valid {
			texts = append(texts, t.str)
		}
	}
	for i := 0; i < 150; i++ {
		n := 2 + r.Intn(5)
		parts := []string{fmt.Sprint(r.Intn(4)), fmt.Sprint(r.Intn(45))}
		if parts[0] == "2" && r.Intn(2) == 0 {
			parts[1] = oidRandArc(r)
		}
		for j := 2; j < n; j++ {
			parts = append(parts, oidRandArc(r))
		}
		s := strings.Join(parts, ".")
		switch r.Intn(12) {
		case 0:
			s += "."
		case 1:
			s = strings.Replace(s, ".", "..", 1)
		case 2:
			s = "+" + s
		}
		texts = append(texts, s)
	}
	o.WriteString("\n/// (text, Go's ParseOID: \"ok HEX TEXT ASN1\" or \"err TEXT\")\nconst TEXT_ROWS: List[(Str, Str)] = [\n")
	for _, t := range texts {
		g, err := x509.ParseOID(t)
		var u x509.OID
		uerr := u.UnmarshalText([]byte(t))
		if (err == nil) != (uerr == nil) || (err == nil && !u.Equal(g)) {
			panic("UnmarshalText is not ParseOID: " + t)
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(t), xq(oidWant(g, err)))
	}
	o.WriteString("]\n")

	// binary: oidTests' raw, and mutations of valid contents
	var raws [][]byte
	for _, t := range oidGoTests {
		raws = append(raws, t.raw)
	}
	for i := 0; i < 120; i++ {
		b := append([]byte(nil), oidGoTests[3+r.Intn(len(oidGoTests)-3)].raw...)
		switch r.Intn(4) {
		case 0:
			b[r.Intn(len(b))] ^= byte(1 << r.Intn(8))
		case 1:
			b = b[:r.Intn(len(b))]
		case 2:
			b = append(b, byte(r.Intn(256)))
		default:
			k := r.Intn(len(b))
			b = append(b[:k], append([]byte{0x80}, b[k:]...)...)
		}
		raws = append(raws, b)
	}
	o.WriteString("\n/// (contents, Go's UnmarshalBinary)\nconst BINARY_ROWS: List[(Bytes, Str)] = [\n")
	for _, b := range raws {
		var g x509.OID
		err := g.UnmarshalBinary(b)
		for _, t := range oidGoTests {
			if string(t.raw) == string(b) && (err == nil) != t.valid {
				panic("oidTests disagrees")
			}
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(b), xq(oidWant(g, err)))
	}
	o.WriteString("]\n")

	// ints: oidTests' and TestInvalidOID's, and random ones
	ints := [][]uint64{{}, {1}, {3}, {3, 100, 200}, {1, 81}, {1, 81, 200}, {0, 39}, {0, 40}, {2, math.MaxUint64}, {math.MaxUint64, 1}}
	for _, t := range oidGoTests {
		if t.ints != nil {
			ints = append(ints, t.ints)
		}
	}
	for i := 0; i < 80; i++ {
		a := []uint64{uint64(r.Intn(4)), uint64(r.Intn(45))}
		for j := r.Intn(5); j > 0; j-- {
			switch r.Intn(3) {
			case 0:
				a = append(a, r.Uint64())
			case 1:
				a = append(a, uint64(r.Intn(300)))
			default:
				a = append(a, math.MaxUint64-uint64(r.Intn(3)))
			}
		}
		ints = append(ints, a)
	}
	o.WriteString("\n/// (arcs, Go's OIDFromInts)\nfn int_rows() -> List[(List[UInt64], Str)] {\n  [\n")
	for _, a := range ints {
		var q []string
		for _, x := range a {
			q = append(q, fmt.Sprintf("UInt64.from_bits(%d)", int64(x)))
		}
		fmt.Fprintf(&o, "    ([%s], %s),\n", strings.Join(q, ", "), xq(oidWant(x509.OIDFromInts(a))))
	}
	o.WriteString("  ]\n}\n")

	// asn1: TestOIDFromASN1OID's, and random ones with negative arcs
	asn1s := [][]int{{-1}, {1}, {255, 1}, {1, 255}, {1, 2, -3}, {2, 5, math.MaxInt64}, {}}
	for i := 0; i < 50; i++ {
		a := []int{r.Intn(4), r.Intn(45)}
		for j := r.Intn(4); j > 0; j-- {
			a = append(a, int(r.Int63())-int(r.Int63()/64))
		}
		asn1s = append(asn1s, a)
	}
	o.WriteString("\n/// (arcs, Go's OIDFromASN1OID)\nconst ASN1_ROWS: List[(List[Int], Str)] = [\n")
	for _, a := range asn1s {
		fmt.Fprintf(&o, "  (%s, %s),\n", oidInts(a), xq(oidWant(x509.OIDFromASN1OID(asn1.ObjectIdentifier(a)))))
	}
	o.WriteString("]\n")

	// EqualASN1OID: TestOIDEqualASN1OID's table, and each valid text
	// against its own arcs and a changed copy
	type eqRow struct {
		oid  string // "" is OID{}
		arcs []int
	}
	m := math.MaxInt32
	eqs := []eqRow{
		{"1.2.3", []int{1, 2, 3}}, {"1.2.3", []int{1, 2, 4}}, {"1.2.3", []int{1, 2, 3, 4}}, {"1.33.22", []int{1, 33, 23}}, {"1.33.23", []int{1, 33, 22}},
		{"1.33.127", []int{1, 33, 127}}, {"1.33.128", []int{1, 33, 127}}, {"1.33.128", []int{1, 33, 128}}, {"1.33.129", []int{1, 33, 129}}, {"1.33.128", []int{1, 33, 129}},
		{"1.33.129", []int{1, 33, 128}}, {"1.33.255", []int{1, 33, 255}}, {"1.33.256", []int{1, 33, 256}}, {"2.33.257", []int{2, 33, 256}}, {"2.33.256", []int{2, 33, 257}},
		{"1.33", []int{1, 33, m}}, {fmt.Sprintf("1.33.%d", m), []int{1, 33}}, {fmt.Sprintf("1.33.%d", m), []int{1, 33, m}}, {fmt.Sprintf("1.33.%d", m+1), []int{1, 33, m + 1}},
		{"1.33.256", []int{}}, {"", []int{1, 33, 256}}, {"", []int{}}, {"2.100", []int{2, 100}}, {"2.100", []int{2, 99}}, {"1.2.3", []int{1, 2, -3}},
	}
	for _, t := range texts {
		g, err := x509.ParseOID(t)
		if err != nil {
			continue
		}
		var arcs []int
		ok := true
		for _, p := range strings.Split(g.String(), ".") {
			var n int
			if _, e := fmt.Sscan(p, &n); e != nil {
				ok = false
				break
			}
			arcs = append(arcs, n)
		}
		if ok {
			eqs = append(eqs, eqRow{t, arcs})
			c := append([]int(nil), arcs...)
			c[len(c)-1]++
			eqs = append(eqs, eqRow{t, c})
		}
	}
	o.WriteString("\n/// (Oid's text, \"\" for the empty Oid; arcs; Go's EqualASN1OID)\nconst EQUAL_ROWS: List[(Str, List[Int], Bool)] = [\n")
	for _, e := range eqs {
		var g x509.OID
		if e.oid != "" {
			var err error
			if g, err = x509.ParseOID(e.oid); err != nil {
				panic(err)
			}
		}
		fmt.Fprintf(&o, "  (%s, %s, %v),\n", xq(e.oid), oidInts(e.arcs), g.EqualASN1OID(asn1.ObjectIdentifier(e.arcs)))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn shown(r: Result[x509.Oid, x509.CertErr]) -> Str {
  match r {
    Ok(o) => {
      let a = match o.to_asn1_oid() {
        Some(xs) => xs.map(fn(x) { "${x}" }).join(".")
        None => "none"
      }
      "ok ${o.der.to_hex()} ${o.display()} ${a}"
    }
    Err(e) => "err ${e.display()}"
  }
}

fn diffs(rows: List[(Str, Str, Str)]) -> List[Str] {
  var bad: List[Str] = []
  for (input, got, want) in rows {
    if got != want {
      bad.push("${input}:\n  got  ${got}\n  want ${want}")
    }
  }
  bad
}

test "parse_oid is Go's ParseOID: contents, text, ASN.1 arcs and errors" {
  expect(diffs(TEXT_ROWS.map(fn(r) { (r.0.quote(), shown(x509.parse_oid(r.0)), r.1) }))) == []
}

test "oid_from_binary is Go's UnmarshalBinary" {
  expect(diffs(BINARY_ROWS.map(fn(r) { (r.0.to_hex(), shown(x509.oid_from_binary(r.0)), r.1) }))) == []
}

test "oid_from_ints is Go's OIDFromInts" {
  expect(diffs(int_rows().map(fn(r) { ("${r.0}", shown(x509.oid_from_ints(r.0)), r.1) }))) == []
}

test "oid_from_asn1_oid is Go's OIDFromASN1OID" {
  expect(diffs(ASN1_ROWS.map(fn(r) { ("${r.0}", shown(x509.oid_from_asn1_oid(r.0)), r.1) }))) == []
}

test "equal_asn1_oid is Go's EqualASN1OID" {
  var bad: List[Str] = []
  for (text, arcs, want) in EQUAL_ROWS {
    let o = if text.is_empty() { x509.Oid{} } else { x509.parse_oid(text).ok() ?? fault("parse ${text}") }
    if o.equal_asn1_oid(arcs) != want {
      bad.push("${text} ${arcs}: want ${want}")
    }
  }
  expect(bad) == []
}

test "Go's Equal is ==, and the empty Oid is Go's OID{}" {
  let a = x509.parse_oid("1.2.3").ok() ?? fault("a")
  expect(a == (x509.oid_from_ints([UInt64(1), UInt64(2), UInt64(3)]).ok() ?? fault("b"))) == true
  expect(a == (x509.parse_oid("1.2.4").ok() ?? fault("c"))) == false
  expect(a == (x509.parse_oid("1.2.3.4").ok() ?? fault("d"))) == false
  let zero = x509.Oid{}
  let bad = x509.Oid{der: b"\x80\x01"}
  expect(zero == x509.Oid{}) == true
  expect(zero.display()) == ""
  expect(zero.to_asn1_oid()) == Some([])
  expect(bad.display()) == ""
}
`)
	return o.String()
}

func oidInts(a []int) string {
	var q []string
	for _, x := range a {
		q = append(q, fmt.Sprint(x))
	}
	return "[" + strings.Join(q, ", ") + "]"
}
