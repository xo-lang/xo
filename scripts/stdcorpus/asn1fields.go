package main

import (
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"time"
)

// The struct field corpus of std/asn1 (decision 0661): Go's Unmarshal into
// four struct types that between them use every field tag option, on DER
// from Go's Marshal of seeded random values, its mutations and hand-made
// cases, with Go's Marshal of each decoded value. The Xo side reads and
// writes the same structs with std/asn1's FieldReader and with_field, the
// code `derive Asn1` generates.

type afInner struct {
	X int
	Y string `asn1:"printable"`
}

type afA struct {
	Serial *big.Int
	Alg    asn1.ObjectIdentifier
	Name   string
	Mail   string `asn1:"ia5"`
	When   time.Time
	Until  time.Time `asn1:"generalized"`
	Flags  asn1.BitString
	Key    []byte
	OK     bool
	Count  int
}

type afB struct {
	Version int      `asn1:"optional,explicit,default:1,tag:0"`
	A       int      `asn1:"tag:1"`
	B       string   `asn1:"optional,tag:2,utf8"`
	C       []int    `asn1:"set"`
	D       []string `asn1:"optional,omitempty,explicit,tag:3"`
	F       afInner  `asn1:"optional,explicit,tag:4"`
	G       int      `asn1:"optional,application,tag:5"`
	H       int      `asn1:"optional,private,tag:6"`
	I       bool     `asn1:"optional"`
}

type afC struct {
	Items []afInner
	Raws  []asn1.RawValue
	Any   asn1.RawValue
	Nest  afInner   `asn1:"set"`
	At    time.Time `asn1:"optional,tag:7"`
	Tail  int       `asn1:"optional"`
}

type afD struct {
	Raw asn1.RawContent
	N   int
	S   string `asn1:"tag:0"`
}

// afE is derive-only on the Xo side: ENUMERATED fields and times in
// time.Civil fields, which hold any year GeneralizedTime writes (RFC
// 5280's 99991231235959Z).
type afE struct {
	St    asn1.Enumerated
	Opt   asn1.Enumerated `asn1:"optional,tag:0,default:2"`
	Never time.Time       `asn1:"generalized"`
	At    time.Time
}

func afCivil(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000") }

func afHex(s string) string { return hex.EncodeToString([]byte(s)) }

func afWhen(t time.Time) string {
	u := t.UTC()
	return fmt.Sprintf("%d.%09d", u.Unix(), u.Nanosecond())
}

func afInnerText(x afInner) string { return fmt.Sprintf("{%d,%s}", x.X, afHex(x.Y)) }

func afRawText(v asn1.RawValue) string {
	return fmt.Sprintf("raw:%d:%d:%v:%x", v.Class, v.Tag, v.IsCompound, v.Bytes)
}

func afText(v any) string {
	switch x := v.(type) {
	case *afA:
		return fmt.Sprintf("A %s|%s|%s|%s|%s|%s|%x:%d|%x|%v|%d", x.Serial, x.Alg, afHex(x.Name), afHex(x.Mail), afWhen(x.When), afWhen(x.Until), x.Flags.Bytes, x.Flags.BitLength, x.Key, x.OK, x.Count)
	case *afB:
		var d []string
		for _, s := range x.D {
			d = append(d, afHex(s))
		}
		f := "-"
		if x.F != (afInner{}) {
			f = afInnerText(x.F)
		}
		return fmt.Sprintf("B %d|%d|%s|%v|[%s]|%s|%d|%d|%v", x.Version, x.A, afHex(x.B), x.C, strings.Join(d, ","), f, x.G, x.H, x.I)
	case *afC:
		var it, rs []string
		for _, i := range x.Items {
			it = append(it, afInnerText(i))
		}
		for _, r := range x.Raws {
			rs = append(rs, afRawText(r))
		}
		at := "-"
		if !x.At.IsZero() {
			at = afWhen(x.At)
		}
		return fmt.Sprintf("C [%s]|[%s]|%s|%s|%s|%d", strings.Join(it, ","), strings.Join(rs, ","), afRawText(x.Any), afInnerText(x.Nest), at, x.Tail)
	case *afD:
		return fmt.Sprintf("D %x|%d|%s", []byte(x.Raw), x.N, afHex(x.S))
	case *afE:
		return fmt.Sprintf("E %d|%d|%s|%s", x.St, x.Opt, afCivil(x.Never), afCivil(x.At))
	}
	panic("afText")
}

// afErr is Go's error as the corpus compares it: a time Go refuses is
// `time` (its time.Parse texts are not ported) and `tags don't match`
// stops before Go's %+v of its parameters (pointers in it).
func afErr(err error) string {
	t := err.Error()
	if strings.HasPrefix(t, "parsing time") || strings.HasPrefix(t, "asn1: time did not") {
		return "time"
	}
	if i := strings.Index(t, "tags don't match"); i >= 0 {
		return t[:i+len("tags don't match")]
	}
	return t
}

// afWide reports a decoded time outside what an Xo Time holds (std/asn1
// reads it as WideTime, which a Time field refuses).
func afWide(ts ...time.Time) bool {
	for _, t := range ts {
		if !t.IsZero() && (t.Year() < 1678 || t.Year() > 2261) {
			return true
		}
	}
	return false
}

// afCase is (type letter, DER, Go's result, Go's Marshal of it).
func afCase(kind byte, der []byte) (string, string) {
	var v any
	switch kind {
	case 'A':
		v = new(afA)
	case 'B':
		v = new(afB)
	case 'C':
		v = new(afC)
	case 'E':
		v = new(afE)
	default:
		v = new(afD)
	}
	rest, err := asn1.Unmarshal(der, v)
	if err != nil {
		return "err " + afErr(err), "-"
	}
	if len(rest) > 0 {
		return "err asn1: syntax error: trailing data", "-"
	}
	switch x := v.(type) {
	case *afA:
		if afWide(x.When, x.Until) {
			return "err wide", "-"
		}
	case *afC:
		if afWide(x.At) {
			return "err wide", "-"
		}
	}
	// std/asn1 writes every time in UTC (its Value holds an instant), where
	// Go writes a parsed time's offset back: Go marshals the UTC times
	switch x := v.(type) {
	case *afA:
		x.When, x.Until = x.When.UTC(), x.Until.UTC()
	case *afC:
		x.At = x.At.UTC()
	case *afE:
		x.Never, x.At = x.Never.UTC(), x.At.UTC()
	}
	enc := "-"
	if b, err := asn1.Marshal(reflectElem(v)); err == nil {
		enc = hex.EncodeToString(b)
	} else {
		enc = "err " + afErr(err)
	}
	return afText(v), enc
}

func reflectElem(v any) any {
	switch x := v.(type) {
	case *afA:
		return *x
	case *afB:
		return *x
	case *afC:
		return *x
	case *afD:
		return *x
	case *afE:
		return *x
	}
	panic("reflectElem")
}

func afStr(r *rand.Rand, printable bool) string {
	pick := []string{"", "a", "Hello World", "x-y.z", "Q?(1)=2", "*.example", "a&b", "é", "日本", "@home"}
	if printable {
		pick = []string{"", "a", "Hello World", "x-y.z", "Q?(1)=2", "*.example"}
	}
	return pick[r.Intn(len(pick))]
}

func afTime(r *rand.Rand, lo, hi int) time.Time {
	y := lo + r.Intn(hi-lo)
	return time.Date(y, time.Month(1+r.Intn(12)), 1+r.Intn(28), r.Intn(24), r.Intn(60), r.Intn(60), 0, time.UTC)
}

func afInt(r *rand.Rand) int {
	return []int{0, 1, -1, 127, 128, -129, 65535, 1 << 40, -(1 << 62), r.Intn(1000)}[r.Intn(10)]
}

func afInnerGen(r *rand.Rand) afInner { return afInner{X: afInt(r), Y: afStr(r, true)} }

func afGen(r *rand.Rand, kind byte) []byte {
	var v any
	switch kind {
	case 'A':
		n := new(big.Int).Lsh(big.NewInt(int64(r.Intn(1000))), uint(r.Intn(100)))
		if r.Intn(3) == 0 {
			n.Neg(n)
		}
		bits := r.Intn(20)
		fb := make([]byte, (bits+7)/8)
		r.Read(fb)
		if bits%8 != 0 {
			fb[len(fb)-1] &^= byte(1<<uint(8-bits%8)) - 1
		}
		key := make([]byte, r.Intn(6))
		r.Read(key)
		when := afTime(r, 1950, 2050)
		if r.Intn(4) == 0 {
			when = afTime(r, 1700, 2200)
		}
		v = afA{Serial: n, Alg: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, r.Intn(20)}, Name: afStr(r, false), Mail: []string{"", "a@b.c", "x"}[r.Intn(3)],
			When: when, Until: afTime(r, 1700, 2200), Flags: asn1.BitString{Bytes: fb, BitLength: bits}, Key: key, OK: r.Intn(2) == 0, Count: afInt(r)}
	case 'B':
		var c []int
		for i, n := 0, r.Intn(4); i < n; i++ {
			c = append(c, afInt(r))
		}
		var d []string
		for i, n := 0, r.Intn(3); i < n; i++ {
			d = append(d, afStr(r, false))
		}
		f := afInner{}
		if r.Intn(2) == 0 {
			f = afInnerGen(r)
		}
		v = afB{Version: []int{0, 1, 2}[r.Intn(3)], A: afInt(r), B: afStr(r, false), C: c, D: d, F: f, G: []int{0, 3}[r.Intn(2)], H: []int{0, -4}[r.Intn(2)], I: r.Intn(2) == 0}
	case 'C':
		var items []afInner
		for i, n := 0, r.Intn(3); i < n; i++ {
			items = append(items, afInnerGen(r))
		}
		var raws []asn1.RawValue
		for i, n := 0, r.Intn(3); i < n; i++ {
			raws = append(raws, asn1.RawValue{Class: r.Intn(4), Tag: r.Intn(40), IsCompound: r.Intn(2) == 0, Bytes: []byte{byte(r.Intn(256))}})
		}
		at := time.Time{}
		if r.Intn(2) == 0 {
			at = afTime(r, 1950, 2050)
		}
		v = afC{Items: items, Raws: raws, Any: asn1.RawValue{Tag: 2, Bytes: []byte{byte(r.Intn(128))}}, Nest: afInnerGen(r), At: at, Tail: []int{0, 9}[r.Intn(2)]}
	case 'E':
		en := func() asn1.Enumerated {
			return asn1.Enumerated([]int{0, 1, 2, 3, -1, 1<<31 - 1, -(1 << 31), 1 << 31, 1 << 40}[r.Intn(9)])
		}
		never := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
		switch r.Intn(4) {
		case 1:
			never = afTime(r, 1, 9999)
		case 2:
			never = afTime(r, 1950, 2050)
		case 3:
			never = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		v = afE{St: en(), Opt: en(), Never: never, At: afTime(r, 1700, 2200)}
	default:
		v = afD{N: afInt(r), S: afStr(r, true)}
	}
	b, err := asn1.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func afFixed() [][2]string {
	// (type, the SEQUENCE's contents as hex, or `=` and the whole DER)
	return [][2]string{
		{"B", "8101053100"},                                     // A only, C an empty SET
		{"B", "a003020101810105"},                               // version given as its default, C missing
		{"B", "810105310002010a"},                               // an INTEGER where the optional BOOLEAN is: unread, allowed
		{"B", "81010531000101ff"},                               // I true
		{"B", "8101053100a403020101"},                           // F explicit holding an INTEGER: optional, so absent
		{"B", "8101053100a4033001"},                             // F's inner value truncated
		{"B", "a0008101053100"},                                 // zero length explicit
		{"B", "8201418101053100"},                               // an implicit [2] where A's [1] must be
		{"B", "8101053100450103c6010c"},                         // G application, H private
		{"B", "8101053103020180"},                               // C holds -128
		{"B", "81010531050c03616263"},                           // C holds a string: sequence tag mismatch
		{"B", "8101053104020101"},                               // C's element runs past it
		{"B", "8101053100a30330"},                               // D's inner header truncated
		{"B", "8101053100a30a30080c03616263130161"},             // D with a UTF8String and a PrintableString
		{"D", "0201050c00"},                                     // S untagged: tags don't match
		{"D", "020105800161"},                                   // S implicit [0] printable
		{"D", "0201058002c3a9"},                                 // S implicit with a byte PrintableString refuses
		{"D", "=3006020105800161ff"},                            // trailing data
		{"C", "3000300002010531050201011300"},                   // Nest a SET
		{"C", "3000300002010530050201011300"},                   // Nest a SEQUENCE where a SET is wanted
		{"C", "30003000020105310502010113000200"},               // an empty INTEGER as Tail
		{"C", "30003000020105310502010113008703"},               // At truncated
		{"C", "300030000201053105020101130087023030"},           // At not a UTCTime
		{"C", "3006300402010113003000020105310502010113000209"}, // Items, Tail 9
		{"E", "0a0105180f39393939313233313233353935395a170d3730303130313030303030305a"},         // 9999-12-31 GeneralizedTime (RFC 5280's never)
		{"E", "0a0105800103180f39393939313233313233353935395a170d3730303130313030303030305a"},   // Opt [0] given
		{"E", "0a0105800102180f39393939313233313233353935395a170d3730303130313030303030305a"},   // Opt given as its default
		{"E", "020105180f39393939313233313233353935395a170d3730303130313030303030305a"},         // an INTEGER where the ENUMERATED is
		{"E", "0a050100000000180f39393939313233313233353935395a170d3730303130313030303030305a"}, // ENUMERATED past 32 bits
		{"E", "0a0105181339393939313233313233353935392b30313030170d3730303130313030303030305a"}, // never at +0100
		{"E", "0a0105180f30303031303130313030303030305a170d3730303130313030303030305a"},         // year 1
		{"E", "0a0105181330303031303130313030303030302b30313030170d3730303130313030303030305a"}, // year 1 at +0100: year 0 in UTC
		{"A", ""},
		{"A", "="},
		{"C", ""},
	}
}

func asn1fieldsCorpus() string {
	r := rand.New(rand.NewSource(661))
	var o strings.Builder
	o.WriteString(header("encoding/asn1's Unmarshal and Marshal of five struct types (every field tag option, ENUMERATED, times of any year) on DER from seeded random values, their mutations, and hand-made cases"))
	o.WriteString("use std/asn1\nuse std/big\nuse std/time\n\n")
	o.WriteString("/// (struct A to E, DER, Go's Unmarshal as the corpus's text or `err TEXT`, Go's Marshal of the result as hex, `err TEXT` or `-`)\nconst CASES: List[(Str, Bytes, Str, Str)] = [\n")
	add := func(kind byte, der []byte) {
		t, enc := afCase(kind, der)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(string(kind)), xbytes(der), xq(t), xq(enc))
	}
	for _, c := range afFixed() {
		h, whole := strings.CutPrefix(c[1], "=")
		b, err := hex.DecodeString(h)
		if err != nil {
			panic(err)
		}
		if !whole {
			b, _ = asn1.Marshal(asn1.RawValue{Tag: 16, IsCompound: true, Bytes: b})
		}
		add(c[0][0], b)
	}
	kinds := []byte("ABCD")
	for i := 0; i < 800; i++ {
		k := kinds[r.Intn(4)]
		b := afGen(r, k)
		if b == nil {
			continue
		}
		switch r.Intn(5) {
		case 0, 1:
		case 2:
			c := append([]byte(nil), b...)
			c[r.Intn(len(c))] ^= byte(1 << uint(r.Intn(8)))
			b = c
		case 3:
			b = b[:r.Intn(len(b))]
		case 4:
			c := append([]byte(nil), b...)
			c[r.Intn(len(c))] = byte(r.Intn(256))
			b = c
		}
		add(k, b)
	}
	// struct E (derive only): its own source, so A to D keep their cases
	re := rand.New(rand.NewSource(6610))
	for i := 0; i < 200; i++ {
		b := afGen(re, 'E')
		if b == nil {
			continue
		}
		switch re.Intn(4) {
		case 2:
			c := append([]byte(nil), b...)
			c[re.Intn(len(c))] ^= byte(1 << uint(re.Intn(8)))
			b = c
		case 3:
			b = b[:re.Intn(len(b))]
		}
		add('E', b)
	}
	o.WriteString("]\n")
	o.WriteString(asn1fieldsTests)
	return o.String()
}

const asn1fieldsTests = `
// The four structs as derive Asn1 reads and writes them (decision 0661):
// Option for an optional field whose zero Go omits.

struct Inner {
  x: Int
  y: Str
}

struct SA {
  serial: big.BigInt
  alg: asn1.Oid
  name: Str
  mail: Str
  when: Time
  until: Time
  flags: asn1.BitString
  key: Bytes
  ok: Bool
  count: Int
}

struct SB {
  version: Int
  a: Int
  b: Str
  c: List[Int]
  d: List[Str]
  f: Inner?
  g: Int
  h: Int
  i: Bool
}

struct SC {
  items: List[Inner]
  raws: List[asn1.Value]
  any: asn1.Value
  nest: Inner
  at: Time?
  tail: Int
}

struct SD {
  raw: Bytes
  n: Int
  s: Str
}

fn inner_fields() -> List[asn1.Field] { [asn1.field("", asn1.KIND_INT), asn1.field("printable", asn1.KIND_STR)] }

fn rd_inner(var p: asn1.FieldReader, v: asn1.Value) -> Inner ! asn1.Asn1Err {
  let fs = inner_fields()
  var r = p.open(v)
  Inner{x: r.need(fs[0])?.to_int()?, y: r.need(fs[1])?.to_str()}
}

fn wr_inner(x: Inner) -> asn1.Value ! asn1.Asn1Err {
  let fs = inner_fields()
  asn1.Value.Sequence(items: [asn1.int_value(x.x).with_field(fs[0])?, asn1.str_value(x.y, fs[1].string_type).with_field(fs[1])?])
}

fn a_fields() -> List[asn1.Field] {
  [asn1.field("", asn1.KIND_INT), asn1.field("", asn1.KIND_OID), asn1.field("", asn1.KIND_STR), asn1.field("ia5", asn1.KIND_STR),
   asn1.field("", asn1.KIND_TIME), asn1.field("generalized", asn1.KIND_TIME), asn1.field("", asn1.KIND_BIT_STRING), asn1.field("", asn1.KIND_BYTES),
   asn1.field("", asn1.KIND_BOOL), asn1.field("", asn1.KIND_INT)]
}

fn rd_a(var p: asn1.FieldReader, v: asn1.Value) -> SA ! asn1.Asn1Err {
  let fs = a_fields()
  var r = p.open(v)
  SA{serial: r.need(fs[0])?.to_big(), alg: r.need(fs[1])?.to_oid(), name: r.need(fs[2])?.to_str(), mail: r.need(fs[3])?.to_str(),
     when: r.need(fs[4])?.to_time()?, until: r.need(fs[5])?.to_time()?, flags: r.need(fs[6])?.to_bit_string(), key: r.need(fs[7])?.to_bytes(),
     ok: r.need(fs[8])?.to_bool(), count: r.need(fs[9])?.to_int()?}
}

fn wr_a(x: SA) -> asn1.Value ! asn1.Asn1Err {
  let fs = a_fields()
  asn1.Value.Sequence(items: [asn1.Value.Int(v: x.serial).with_field(fs[0])?, asn1.Value.Oid(v: x.alg).with_field(fs[1])?,
    asn1.str_value(x.name, fs[2].string_type).with_field(fs[2])?, asn1.str_value(x.mail, fs[3].string_type).with_field(fs[3])?,
    asn1.time_value(x.when, fs[4].time_type).with_field(fs[4])?, asn1.time_value(x.until, fs[5].time_type).with_field(fs[5])?,
    asn1.bit_string_value(x.flags).with_field(fs[6])?, asn1.Value.OctetString(v: x.key).with_field(fs[7])?,
    asn1.Value.Bool(v: x.ok).with_field(fs[8])?, asn1.int_value(x.count).with_field(fs[9])?])
}

fn b_fields() -> List[asn1.Field] {
  [asn1.field("optional,explicit,default:1,tag:0", asn1.KIND_INT), asn1.field("tag:1", asn1.KIND_INT), asn1.field("optional,tag:2,utf8", asn1.KIND_STR),
   asn1.field("set", asn1.KIND_SEQUENCE), asn1.field("optional,omitempty,explicit,tag:3", asn1.KIND_SEQUENCE), asn1.field("optional,explicit,tag:4", asn1.KIND_SEQUENCE),
   asn1.field("optional,application,tag:5", asn1.KIND_INT), asn1.field("optional,private,tag:6", asn1.KIND_INT), asn1.field("optional", asn1.KIND_BOOL)]
}

fn rd_b(var p: asn1.FieldReader, v: asn1.Value) -> SB ! asn1.Asn1Err {
  let fs = b_fields()
  var r = p.open(v)
  let version = match r.next(fs[0])? {
    Some(x) => x.to_int()?
    None => fs[0].default_value ?? 0
  }
  let a = r.need(fs[1])?.to_int()?
  let b = match r.next(fs[2])? {
    Some(x) => x.to_str()
    None => ""
  }
  var c: List[Int] = []
  var cr = r.open_list(r.need(fs[3])?, asn1.KIND_INT)?
  while !cr.done() {
    c.push(cr.need(asn1.Field{kind: asn1.KIND_INT})?.to_int()?)
  }
  var d: List[Str] = []
  match r.next(fs[4])? {
    Some(x) => {
      var dr = r.open_list(x, asn1.KIND_STR)?
      while !dr.done() {
        d.push(dr.need(asn1.Field{kind: asn1.KIND_STR})?.to_str())
      }
    }
    None => {}
  }
  let f = match r.next(fs[5])? {
    Some(x) => Some(rd_inner(r, x)?)
    None => None
  }
  let g = match r.next(fs[6])? {
    Some(x) => x.to_int()?
    None => 0
  }
  let h = match r.next(fs[7])? {
    Some(x) => x.to_int()?
    None => 0
  }
  let i = match r.next(fs[8])? {
    Some(x) => x.to_bool()
    None => false
  }
  SB{version: version, a: a, b: b, c: c, d: d, f: f, g: g, h: h, i: i}
}

fn wr_b(x: SB) -> asn1.Value ! asn1.Asn1Err {
  let fs = b_fields()
  var items: List[asn1.Value] = []
  if x.version != (fs[0].default_value ?? 0) {
    items.push(asn1.int_value(x.version).with_field(fs[0])?)
  }
  items.push(asn1.int_value(x.a).with_field(fs[1])?)
  if x.b != "" {
    items.push(asn1.str_value(x.b, fs[2].string_type).with_field(fs[2])?)
  }
  items.push(asn1.Value.Set(items: x.c.map(asn1.int_value)).with_field(asn1.Field{kind: asn1.KIND_SEQUENCE})?)
  if !x.d.is_empty() {
    var ds: List[asn1.Value] = []
    for s in x.d {
      ds.push(asn1.str_value(s))
    }
    items.push(asn1.Value.Sequence(items: ds).with_field(fs[4])?)
  }
  match x.f {
    Some(f) => items.push(wr_inner(f)?.with_field(fs[5])?)
    None => {}
  }
  if x.g != 0 {
    items.push(asn1.int_value(x.g).with_field(fs[6])?)
  }
  if x.h != 0 {
    items.push(asn1.int_value(x.h).with_field(fs[7])?)
  }
  if x.i {
    items.push(asn1.Value.Bool(v: x.i).with_field(fs[8])?)
  }
  asn1.Value.Sequence(items: items)
}

fn c_fields() -> List[asn1.Field] {
  [asn1.field("", asn1.KIND_SEQUENCE), asn1.field("", asn1.KIND_SEQUENCE), asn1.field("", asn1.KIND_ANY), asn1.field("set", asn1.KIND_SEQUENCE),
   asn1.field("optional,tag:7", asn1.KIND_TIME), asn1.field("optional", asn1.KIND_INT)]
}

fn rd_c(var p: asn1.FieldReader, v: asn1.Value) -> SC ! asn1.Asn1Err {
  let fs = c_fields()
  var r = p.open(v)
  var items: List[Inner] = []
  var ir = r.open_list(r.need(fs[0])?, asn1.KIND_SEQUENCE)?
  while !ir.done() {
    let e = ir.need(asn1.Field{kind: asn1.KIND_SEQUENCE})?
    items.push(rd_inner(ir, e)?)
  }
  var raws: List[asn1.Value] = []
  var rr = r.open_list(r.need(fs[1])?, asn1.KIND_ANY)?
  while !rr.done() {
    raws.push(rr.need(asn1.Field{})?)
  }
  let any = r.need(fs[2])?
  let nv = r.need(fs[3])?
  let nest = rd_inner(r, nv)?
  let at = match r.next(fs[4])? {
    Some(x) => Some(x.to_time()?)
    None => None
  }
  let tail = match r.next(fs[5])? {
    Some(x) => x.to_int()?
    None => 0
  }
  SC{items: items, raws: raws, any: any, nest: nest, at: at, tail: tail}
}

fn wr_c(x: SC) -> asn1.Value ! asn1.Asn1Err {
  let fs = c_fields()
  var its: List[asn1.Value] = []
  for i in x.items {
    its.push(wr_inner(i)?)
  }
  var items: List[asn1.Value] = [asn1.Value.Sequence(items: its).with_field(fs[0])?, asn1.Value.Sequence(items: x.raws).with_field(fs[1])?,
    x.any.with_field(fs[2])?, wr_inner(x.nest)?.with_field(fs[3])?]
  match x.at {
    Some(t) => items.push(asn1.time_value(t, fs[4].time_type).with_field(fs[4])?)
    None => {}
  }
  if x.tail != 0 {
    items.push(asn1.int_value(x.tail).with_field(fs[5])?)
  }
  asn1.Value.Sequence(items: items)
}

fn d_fields() -> List[asn1.Field] { [asn1.field("", asn1.KIND_INT), asn1.field("tag:0", asn1.KIND_STR)] }

fn rd_d(var p: asn1.FieldReader, v: asn1.Value) -> SD ! asn1.Asn1Err {
  let fs = d_fields()
  let raw = p.last()
  var r = p.open(v)
  SD{raw: raw, n: r.need(fs[0])?.to_int()?, s: r.need(fs[1])?.to_str()}
}

fn wr_d(x: SD) -> asn1.Value ! asn1.Asn1Err {
  let fs = d_fields()
  if !x.raw.is_empty() {
    return asn1.raw_struct(x.raw)
  }
  asn1.Value.Sequence(items: [asn1.int_value(x.n).with_field(fs[0])?, asn1.str_value(x.s, fs[1].string_type).with_field(fs[1])?])
}

fn hex_str(s: Str) -> Str { s.bytes().to_hex() }

fn when(t: Time) -> Str {
  let s = t.unix()
  let frac = (t - Time.from_unix(s)) / 1ns
  let f = "${frac}"
  "${s}.${"0".repeat(9 - f.len())}${f}"
}

fn class_num(c: asn1.Class) -> Int {
  match c {
    asn1.Class.Universal => 0
    asn1.Class.Application => 1
    asn1.Class.ContextSpecific => 2
    asn1.Class.Private => 3
  }
}

fn raw_text(v: asn1.Value) -> Str {
  match v {
    asn1.Value.Raw(c, t, k, b) => "raw:${class_num(c)}:${t}:${k}:${b.to_hex()}"
    _ => "?"
  }
}

fn inner_text(x: Inner) -> Str { "{${x.x},${hex_str(x.y)}}" }

fn int_list(xs: List[Int]) -> Str { "[${xs.map(fn(n: Int) -> Str { "${n}" }).join(" ")}]" }

fn err_text(e: asn1.Asn1Err) -> Str {
  let t = "${e}"
  if t.starts_with("asn1: invalid UTCTime") || t.starts_with("asn1: invalid GeneralizedTime") {
    return "time"
  }
  if t.contains("outside the years a Time holds") {
    return "wide"
  }
  match t.find("tags don't match") {
    Some(i) => t[..i + "tags don't match".len()]
    None => t
  }
}

/// Reads der as struct kind like Unmarshal (trailing data refused) and
/// writes the result back: (text, Marshal hex or err).
fn round(kind: Str, der: Bytes) -> (Str, Str) ! asn1.Asn1Err {
  var top = asn1.field_reader(der)
  let v = top.need(asn1.field("", asn1.KIND_SEQUENCE))?
  // the fields first, then the trailing data (Go's Unmarshal reads the
  // struct before its caller sees rest)
  let (text, out) = match kind {
    "A" => {
      let x = rd_a(top, v)?
      ("A ${x.serial}|${x.alg}|${hex_str(x.name)}|${hex_str(x.mail)}|${when(x.when)}|${when(x.until)}|${x.flags.bytes.to_hex()}:${x.flags.bit_len}|${x.key.to_hex()}|${x.ok}|${x.count}", wr_a(x))
    }
    "B" => {
      let x = rd_b(top, v)?
      let f = match x.f {
        Some(i) => if i.x == 0 && i.y == "" { "-" } else { inner_text(i) }
        None => "-"
      }
      ("B ${x.version}|${x.a}|${hex_str(x.b)}|${int_list(x.c)}|[${x.d.map(hex_str).join(",")}]|${f}|${x.g}|${x.h}|${x.i}", wr_b(x))
    }
    "C" => {
      let x = rd_c(top, v)?
      let at = match x.at {
        Some(t) => when(t)
        None => "-"
      }
      ("C [${x.items.map(inner_text).join(",")}]|[${x.raws.map(raw_text).join(",")}]|${raw_text(x.any)}|${inner_text(x.nest)}|${at}|${x.tail}", wr_c(x))
    }
    _ => {
      let x = rd_d(top, v)?
      ("D ${x.raw.to_hex()}|${x.n}|${hex_str(x.s)}", wr_d(x))
    }
  }
  if !top.done() {
    return Err(asn1.Asn1Err.Syntax(msg: "trailing data"))
  }
  let enc = match out {
    Ok(val) => match val.der() {
      Ok(b) => b.to_hex()
      Err(e) => "err ${err_text(e)}"
    }
    Err(e) => "err ${err_text(e)}"
  }
  (text, enc)
}

// The same four structs with derive Asn1 (decision 0661): the generated
// reader and writer must give the hand-written ones' results.

struct DInner {
  x: Int
  y asn1 "printable": Str
} derive Asn1

struct DA {
  serial: big.BigInt
  alg: asn1.Oid
  name: Str
  mail asn1 "ia5": Str
  when: Time
  until asn1 "generalized": Time
  flags: asn1.BitString
  key: Bytes
  ok: Bool
  count: Int
} derive Asn1

struct DB {
  version asn1 "optional,explicit,default:1,tag:0": Int
  a asn1 "tag:1": Int
  b asn1 "optional,tag:2,utf8": Str
  c asn1 "set": List[Int]
  d asn1 "optional,omitempty,explicit,tag:3": List[Str]
  f asn1 "optional,explicit,tag:4": DInner?
  g asn1 "optional,application,tag:5": Int
  h asn1 "optional,private,tag:6": Int
  i asn1 "optional": Bool
} derive Asn1

struct DC {
  items: List[DInner]
  raws: List[asn1.Value]
  any: asn1.Value
  nest asn1 "set": DInner
  at asn1 "optional,tag:7": Time?
  tail asn1 "optional": Int
} derive Asn1

struct DD {
  raw: asn1.RawContent
  n: Int
  s asn1 "tag:0": Str
} derive Asn1

struct DE {
  st: asn1.Enumerated
  opt asn1 "optional,tag:0,default:2": asn1.Enumerated
  never asn1 "generalized": time.Civil
  at: time.Civil
} derive Asn1

fn pad(n: Int, w: Int) -> Str {
  let t = "${n}"
  "${"0".repeat((w - t.len()).max(0))}${t}"
}

/// Go's t.UTC().Format("2006-01-02T15:04:05.000000000").
fn civil_text(c: time.Civil) -> Str {
  "${pad(c.date.year, 4)}-${pad(c.date.month, 2)}-${pad(c.date.day, 2)}T${pad(c.time.hour, 2)}:${pad(c.time.minute, 2)}:${pad(c.time.second, 2)}.${pad(c.time.nanos, 9)}"
}

fn hi(x: DInner) -> Inner { Inner{x: x.x, y: x.y} }

fn enc_text(r: Result[Bytes, asn1.Asn1Err]) -> Str {
  match r {
    Ok(b) => b.to_hex()
    Err(e) => "err ${err_text(e)}"
  }
}

/// round with the derived structs and asn1.unmarshal and asn1.marshal.
fn round_derived(kind: Str, der: Bytes) -> (Str, Str) ! asn1.Asn1Err {
  match kind {
    "A" => {
      let d = asn1.unmarshal[DA](der)?
      let x = SA{serial: d.serial, alg: d.alg, name: d.name, mail: d.mail, when: d.when, until: d.until, flags: d.flags, key: d.key, ok: d.ok, count: d.count}
      Ok(("A ${x.serial}|${x.alg}|${hex_str(x.name)}|${hex_str(x.mail)}|${when(x.when)}|${when(x.until)}|${x.flags.bytes.to_hex()}:${x.flags.bit_len}|${x.key.to_hex()}|${x.ok}|${x.count}", enc_text(asn1.marshal(d))))
    }
    "B" => {
      let d = asn1.unmarshal[DB](der)?
      let f = match d.f {
        Some(i) => if i.x == 0 && i.y == "" { "-" } else { inner_text(hi(i)) }
        None => "-"
      }
      Ok(("B ${d.version}|${d.a}|${hex_str(d.b)}|${int_list(d.c)}|[${d.d.map(hex_str).join(",")}]|${f}|${d.g}|${d.h}|${d.i}", enc_text(asn1.marshal(d))))
    }
    "C" => {
      let d = asn1.unmarshal[DC](der)?
      let at = match d.at {
        Some(t) => when(t)
        None => "-"
      }
      Ok(("C [${d.items.map(hi).map(inner_text).join(",")}]|[${d.raws.map(raw_text).join(",")}]|${raw_text(d.any)}|${inner_text(hi(d.nest))}|${at}|${d.tail}", enc_text(asn1.marshal(d))))
    }
    "E" => {
      let d = asn1.unmarshal[DE](der)?
      Ok(("E ${d.st.0}|${d.opt.0}|${civil_text(d.never)}|${civil_text(d.at)}", enc_text(asn1.marshal(d))))
    }
    _ => {
      let d = asn1.unmarshal[DD](der)?
      Ok(("D ${d.raw.0.to_hex()}|${d.n}|${hex_str(d.s)}", enc_text(asn1.marshal(d))))
    }
  }
}

test "derive Asn1 reads and writes as Go's Unmarshal and Marshal" {
  var bad: List[(Str, Bytes, Str, Str)] = []
  for (kind, der, want, wenc) in CASES {
    match round_derived(kind, der) {
      Ok((text, enc)) => {
        if text != want {
          bad.push((kind, der, text, want))
        } else if enc != wenc {
          bad.push((kind, der, "marshal ${enc}", wenc))
        }
      }
      Err(e) => {
        let got = "err ${err_text(e)}"
        if got != want {
          bad.push((kind, der, got, want))
        }
      }
    }
  }
  expect(bad[..bad.len().min(5)]) == []
}

test "struct fields read and write as Go's Unmarshal and Marshal" {
  var bad: List[(Str, Bytes, Str, Str)] = []
  for (kind, der, want, wenc) in CASES {
    if kind == "E" {
      continue
    }
    match round(kind, der) {
      Ok((text, enc)) => {
        if text != want {
          bad.push((kind, der, text, want))
        } else if enc != wenc {
          bad.push((kind, der, "marshal ${enc}", wenc))
        }
      }
      Err(e) => {
        let got = "err ${err_text(e)}"
        if got != want {
          bad.push((kind, der, got, want))
        }
      }
    }
  }
  expect(bad[..bad.len().min(5)]) == []
}
`
