package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// protoRef is what the oracle (internal/stdtest/protoref, google.golang.org/protobuf's protowire) makes of one case.
type protoRef struct {
	Err string `json:"err"`
	V   string `json:"v"`
	N   int    `json:"n"`
}

func (r protoRef) String() string {
	if r.Err != "" {
		return "err " + r.Err
	}
	return fmt.Sprintf("ok %s %d", r.V, r.N)
}

type protoCase struct {
	op string
	in []byte
}

func protoOracle(cases []protoCase) []protoRef {
	var in bytes.Buffer
	for _, c := range cases {
		h := "-"
		if len(c.in) > 0 {
			h = hex.EncodeToString(c.in)
		}
		fmt.Fprintf(&in, "%s %s\n", c.op, h)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("internal", "stdtest", "protoref")
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the protobuf oracle (internal/stdtest/protoref) failed: %v\n", err)
		os.Exit(1)
	}
	var refs []protoRef
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var r protoRef
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			panic(err)
		}
		refs = append(refs, r)
	}
	if len(refs) != len(cases) {
		panic("protobuf oracle: wrong number of answers")
	}
	return refs
}

// protoVarint is v as a varint, optionally padded with redundant continuation bytes.
func protoVarint(v uint64, pad int) []byte {
	var b []byte
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	b = append(b, byte(v))
	for i := 0; i < pad && len(b) < 10; i++ {
		b[len(b)-1] |= 0x80
		b = append(b, 0)
	}
	return b
}

// protoField is one random field: tag, then a value of its wire type (a group nests fields).
func protoField(r *rand.Rand, depth int) []byte {
	num := uint64(1 + r.Intn(20))
	if r.Intn(30) == 0 {
		num = uint64(r.Int63n(1 << 33))
	}
	wire := uint64(r.Intn(8))
	if r.Intn(4) != 0 {
		wire = []uint64{0, 1, 2, 5, 3}[r.Intn(5)]
	}
	b := protoVarint(num<<3|wire, 0)
	switch wire {
	case 0:
		v := r.Uint64() >> uint(r.Intn(64))
		b = append(b, protoVarint(v, r.Intn(3)/2)...)
	case 1:
		b = append(b, make([]byte, 8)...)
		r.Read(b[len(b)-8:])
	case 5:
		b = append(b, make([]byte, 4)...)
		r.Read(b[len(b)-4:])
	case 2:
		n := r.Intn(12)
		b = append(b, protoVarint(uint64(n), 0)...)
		p := make([]byte, n)
		r.Read(p)
		b = append(b, p...)
	case 3:
		if depth < 3 {
			for i, n := 0, r.Intn(3); i < n; i++ {
				b = append(b, protoField(r, depth+1)...)
			}
		}
		end := num
		if r.Intn(8) == 0 {
			end++
		}
		b = append(b, protoVarint(end<<3|4, 0)...)
	}
	return b
}

// protoMutate changes a few bytes of in, or cuts it.
func protoMutate(r *rand.Rand, in []byte) []byte {
	b := append([]byte(nil), in...)
	if len(b) == 0 {
		return b
	}
	switch r.Intn(4) {
	case 0:
		return b[:r.Intn(len(b))]
	case 1:
		b[r.Intn(len(b))] = byte(r.Intn(256))
	case 2:
		b[r.Intn(len(b))] |= 0x80
	default:
		b = append(b, byte(r.Intn(256)))
	}
	return b
}

func protobufCorpus() string {
	var cases []protoCase
	add := func(op string, in []byte) { cases = append(cases, protoCase{op, in}) }
	hexb := func(s string) []byte { b, _ := hex.DecodeString(s); return b }
	for _, h := range []string{"", "00", "01", "7f", "8001", "96 01", "ac02", "808001", "80", "ff", "ffffffffffffffffff01", "ffffffffffffffffff02",
		"80808080808080808080 01", "80808080808080808001", "ffffffffffffffffff7f", "0000", "e58e26"} {
		add("varint", hexb(strings.ReplaceAll(h, " ", "")))
	}
	for _, h := range []string{"", "00", "08", "0a", "0b", "0c", "0e", "0f", "f8ffffff0f", "f8ffffff07", "f8ffffff1f", "ffffffffff01", "8001", "9601"} {
		add("tag", hexb(h))
		add("field", hexb(h))
	}
	for _, h := range []string{"", "00", "0568656c6c6f", "05686c6c6f", "ff", "80", "8001", "ffffffffffffffffff01", "ffffffffffffffff7f"} {
		add("bytes", hexb(h))
	}
	for _, h := range []string{"", "01020304", "010203", "01020304ff", "0000000000000000", "00000000000000"} {
		add("fixed32", hexb(h))
		add("fixed64", hexb(h))
	}
	for _, h := range []string{"0b0c", "0b100c", "0b130c", "0b0d", "0c", "0b0b0c0c", "0b0b0b0c0c0c", "0a03616263", "08ff01", "0d01020304", "09010203040506070809", "0e00", "0f00"} {
		add("field", hexb(h))
	}
	// a deep chain of groups, past the recursion limit
	deep := bytes.Repeat([]byte{0x0b}, 10002)
	deep = append(deep, bytes.Repeat([]byte{0x0c}, 10002)...)
	add("field", deep)
	ok := bytes.Repeat([]byte{0x0b}, 10000)
	ok = append(ok, bytes.Repeat([]byte{0x0c}, 10000)...)
	add("field", ok)
	// typed cases: derive Proto's marshal (msg) and unmarshal (unmsg) against a dynamicpb message
	tr := rand.New(rand.NewSource(5301))
	for i := 0; i < 400; i++ {
		id := int64(0)
		name := []byte(nil)
		z := int64(0)
		okb := 0
		fb := uint64(0)
		raw := []byte(nil)
		if tr.Intn(3) != 0 {
			id = int64(tr.Uint64() >> uint(tr.Intn(64)))
			if tr.Intn(2) == 0 {
				id = -id
			}
		}
		if tr.Intn(3) != 0 {
			name = make([]byte, tr.Intn(20))
			for j := range name {
				name[j] = byte('a' + tr.Intn(26))
			}
		}
		if tr.Intn(3) != 0 {
			z = int64(tr.Uint64() >> uint(tr.Intn(64)))
			if tr.Intn(2) == 0 {
				z = -z
			}
		}
		if tr.Intn(2) == 0 {
			okb = 1
		}
		if tr.Intn(3) != 0 {
			fb = tr.Uint64()
			if tr.Intn(2) == 0 {
				fb = math.Float64bits(float64(tr.Intn(2000)-1000) / 8)
			}
		}
		if tr.Intn(3) != 0 {
			raw = make([]byte, tr.Intn(20))
			tr.Read(raw)
		}
		h := func(b []byte) string {
			if len(b) == 0 {
				return "-"
			}
			return hex.EncodeToString(b)
		}
		cases = append(cases, protoCase{fmt.Sprintf("msg %d %s %d %d %d %s", id, h(name), z, okb, fb, h(raw)), nil})
	}
	for i := 0; i < 300; i++ {
		var b []byte
		for j, n := 0, tr.Intn(6); j < n; j++ {
			b = append(b, protoField(tr, 0)...)
		}
		if tr.Intn(4) == 0 {
			b = protoMutate(tr, b)
		}
		add("unmsg", b)
	}
	// Option, List and nested fields: marshal (rmsg) and unmarshal (runmsg) against dynamicpb
	join := func(p []string) string {
		if len(p) == 0 {
			return "-"
		}
		return strings.Join(p, ",")
	}
	for i := 0; i < 400; i++ {
		var nums, tags, inners, ds []string
		for j, n := 0, tr.Intn(5); j < n; j++ {
			v := int64(tr.Uint64() >> uint(tr.Intn(64)))
			if tr.Intn(3) == 0 {
				v = -v
			}
			nums = append(nums, fmt.Sprint(v))
		}
		for j, n := 0, tr.Intn(4); j < n; j++ {
			t := make([]byte, tr.Intn(6))
			for k := range t {
				t[k] = byte('a' + tr.Intn(26))
			}
			if len(t) == 0 {
				tags = append(tags, "~")
			} else {
				tags = append(tags, hex.EncodeToString(t))
			}
		}
		for j, n := 0, tr.Intn(4); j < n; j++ {
			inners = append(inners, fmt.Sprint(tr.Intn(5)-2))
		}
		for j, n := 0, tr.Intn(4); j < n; j++ {
			ds = append(ds, fmt.Sprint(math.Float64bits(float64(tr.Intn(100))/4)))
		}
		maybe, mo := "n", "n"
		if tr.Intn(2) == 0 {
			maybe = fmt.Sprint(tr.Intn(3))
		}
		if tr.Intn(2) == 0 {
			mo = fmt.Sprint(tr.Intn(3))
		}
		id := fmt.Sprint(tr.Intn(3))
		var ms, mn, mb []string
		seen := map[string]bool{}
		for j, n := 0, tr.Intn(4); j < n; j++ {
			k := string(rune('a' + tr.Intn(5)))
			if !seen["s"+k] {
				seen["s"+k] = true
				ms = append(ms, fmt.Sprintf("%s=%d", k, tr.Intn(5)-2))
			}
		}
		for j, n := 0, tr.Intn(4); j < n; j++ {
			k := tr.Intn(7) - 3
			if !seen[fmt.Sprint("n", k)] {
				seen[fmt.Sprint("n", k)] = true
				mn = append(mn, fmt.Sprintf("%d=%d", k, tr.Intn(3)))
			}
		}
		for _, k := range []string{"false", "true"} {
			if tr.Intn(2) == 0 {
				v := "~"
				if tr.Intn(2) == 0 {
					v = "7a"
				}
				mb = append(mb, k+"="+v)
			}
		}
		cases = append(cases, protoCase{fmt.Sprintf("rmsg %s %s %s %s %d %s %s %s %s %s %s", id, join(nums), join(tags), maybe, tr.Intn(3), join(inners), mo, join(ds), join(ms), join(mn), join(mb)), nil})
	}
	for i := 0; i < 400; i++ {
		var b []byte
		in := func() []byte {
			var x []byte
			for j, n := 0, tr.Intn(3); j < n; j++ {
				if tr.Intn(3) == 0 {
					x = append(x, protoField(tr, 0)...)
				} else {
					x = append(x, 0x08)
					x = append(x, protoVarint(uint64(tr.Intn(4)), 0)...)
				}
			}
			return x
		}
		for j, n := 0, 1+tr.Intn(7); j < n; j++ {
			num := uint64(1 + tr.Intn(11))
			switch num {
			case 9, 10, 11:
				var ent []byte
				for q, m := 0, tr.Intn(4); q < m; q++ {
					en := uint64(1 + tr.Intn(2))
					switch {
					case num == 9 && en == 1, num == 11 && en == 2:
						t := []byte{byte('a' + tr.Intn(3))}
						ent = append(ent, protoVarint(en<<3|2, 0)...)
						ent = append(ent, byte(len(t)))
						ent = append(ent, t...)
					case num == 10 && en == 2:
						x := in()
						ent = append(ent, protoVarint(en<<3|2, 0)...)
						ent = append(ent, protoVarint(uint64(len(x)), 0)...)
						ent = append(ent, x...)
					default:
						ent = append(ent, protoVarint(en<<3, 0)...)
						ent = append(ent, protoVarint(uint64(tr.Intn(4)), 0)...)
					}
				}
				b = append(b, protoVarint(num<<3|2, 0)...)
				b = append(b, protoVarint(uint64(len(ent)), 0)...)
				b = append(b, ent...)
			case 1, 4:
				b = append(b, protoVarint(num<<3, 0)...)
				b = append(b, protoVarint(tr.Uint64()>>uint(tr.Intn(64)), 0)...)
			case 2, 8:
				if tr.Intn(2) == 0 {
					b = append(b, protoVarint(num<<3|2, 0)...)
					var p []byte
					for k, m := 0, tr.Intn(4); k < m; k++ {
						if num == 2 {
							p = append(p, protoVarint(tr.Uint64()>>uint(tr.Intn(64)), 0)...)
						} else {
							p = append(p, make([]byte, 8)...)
							tr.Read(p[len(p)-8:])
						}
					}
					b = append(b, protoVarint(uint64(len(p)), 0)...)
					b = append(b, p...)
				} else if num == 2 {
					b = append(b, protoVarint(num<<3, 0)...)
					b = append(b, protoVarint(tr.Uint64()>>uint(tr.Intn(64)), 0)...)
				} else {
					b = append(b, protoVarint(num<<3|1, 0)...)
					p := make([]byte, 8)
					tr.Read(p)
					b = append(b, p...)
				}
			case 3:
				b = append(b, protoVarint(num<<3|2, 0)...)
				t := make([]byte, tr.Intn(5))
				for k := range t {
					t[k] = byte('a' + tr.Intn(26))
				}
				if tr.Intn(10) == 0 && len(t) > 0 {
					t[0] = 0xff
				}
				b = append(b, protoVarint(uint64(len(t)), 0)...)
				b = append(b, t...)
			default:
				x := in()
				b = append(b, protoVarint(num<<3|2, 0)...)
				b = append(b, protoVarint(uint64(len(x)), 0)...)
				b = append(b, x...)
			}
		}
		if tr.Intn(6) == 0 {
			b = protoMutate(tr, b)
		}
		add("runmsg", b)
	}
	// oneof: marshal (omsg) and unmarshal (ounmsg) against dynamicpb
	for i := 0; i < 300; i++ {
		kind := []string{"n", "c", "s", "l", "i"}[tr.Intn(5)]
		val := "-"
		switch kind {
		case "c":
			val = fmt.Sprint(tr.Intn(3))
		case "s":
			val = fmt.Sprint(math.Float64bits(float64(tr.Intn(3))))
		case "l":
			val = []string{"~", "7a", "6162"}[tr.Intn(3)]
		case "i":
			val = fmt.Sprint(tr.Intn(3))
		}
		tail := []string{"~", "7a"}[tr.Intn(2)]
		cases = append(cases, protoCase{fmt.Sprintf("omsg %d %s %s %s", tr.Intn(3), kind, val, tail), nil})
	}
	for i := 0; i < 300; i++ {
		var b []byte
		for j, n := 0, 1+tr.Intn(6); j < n; j++ {
			switch num := 1 + tr.Intn(6); num {
			case 1, 2:
				b = append(b, protoVarint(uint64(num)<<3, 0)...)
				b = append(b, protoVarint(uint64(tr.Intn(3)), 0)...)
			case 3:
				b = append(b, protoVarint(3<<3|1, 0)...)
				b = append(b, make([]byte, 8)...)
				b[len(b)-1] = byte(tr.Intn(2))
			case 4, 6:
				b = append(b, protoVarint(uint64(num)<<3|2, 0)...)
				l := tr.Intn(3)
				b = append(b, byte(l))
				for k := 0; k < l; k++ {
					b = append(b, byte('a'+tr.Intn(3)))
				}
			default:
				b = append(b, protoVarint(5<<3|2, 0)...)
				if tr.Intn(3) == 0 {
					b = append(b, 0)
				} else {
					b = append(b, 2, 0x08, byte(tr.Intn(3)))
				}
			}
		}
		if tr.Intn(6) == 0 {
			b = protoMutate(tr, b)
		}
		add("ounmsg", b)
	}
	for _, v := range []uint64{0, 1, 127, 128, 255, 300, 16383, 16384, 1<<32 - 1, 1 << 32, 1<<63 - 1, 1 << 63, 1<<64 - 1} {
		cases = append(cases, protoCase{"encvarint " + fmt.Sprint(v), nil})
	}
	for _, v := range []int64{0, 1, -1, 2, -2, 63, -64, 1<<31 - 1, -1 << 31, 1<<63 - 1, -1 << 63} {
		cases = append(cases, protoCase{"zigzag " + fmt.Sprint(v), nil})
	}
	r := rand.New(rand.NewSource(530))
	for i := 0; i < 1200; i++ {
		f := protoField(r, 0)
		add("field", f)
		add("field", protoMutate(r, f))
		v := make([]byte, 1+r.Intn(11))
		r.Read(v)
		for j := 0; j < len(v)-1; j++ {
			if r.Intn(3) != 0 {
				v[j] |= 0x80
			}
		}
		add("varint", v)
		add("tag", protoMutate(r, f))
		add("bytes", protoMutate(r, f))
	}
	refs := protoOracle(cases)
	var o strings.Builder
	o.WriteString(header("google.golang.org/protobuf's protowire: ConsumeVarint, ConsumeTag, ConsumeFixed32 and 64, ConsumeBytes, ConsumeFieldValue (groups, the recursion limit), AppendVarint, SizeVarint and the zigzag forms, on curated and seeded random inputs, against internal/stdtest/protoref"))
	o.WriteString("use std/protobuf\n\nstruct In {\n  a proto \"1\": Int\n} derive Proto\n\nstruct R {\n  id proto \"1\": Int\n  nums proto \"2\": List[Int]\n  tags proto \"3\": List[Str]\n  maybe proto \"4\": Int?\n  inner proto \"5\": In\n  inners proto \"6\": List[In]\n  mo proto \"7\": In?\n  ds proto \"8\": List[Float]\n  ms proto \"9\": Map[Str, Int]\n  mn proto \"10\": Map[Int, In]\n  mb proto \"11\": Map[Bool, Str]\n} derive Proto\n\nenum Choice {\n  Circle as \"2\"(r: Int)\n  Sq as \"3\"(side: Float)\n  Lbl as \"4\"(text: Str)\n  Inn as \"5\"(v: In)\n} derive Proto\n\nstruct O {\n  id proto \"1\": Int\n  ch proto \"oneof\": Choice?\n  tail proto \"6\": Str\n} derive Proto\n\nstruct M {\n  id proto \"1\": Int\n  name proto \"2\": Str\n  z proto \"3,zigzag\": Int\n  ok proto \"4\": Bool\n  f proto \"5\": Float\n  raw proto \"6\": Bytes\n} derive Proto\n\n")
	o.WriteString("/// (operation, input in hex or the number, what Go's protowire answers: `ok VALUE N` or `err KIND`)\nconst CASES: List[(Str, Bytes, Str)] = [\n")
	for i, c := range cases {
		op, arg := c.op, ""
		if k := strings.IndexByte(op, ' '); k >= 0 {
			op, arg = op[:k], op[k+1:]
		}
		in := c.in
		if arg != "" {
			in = []byte(arg)
		}
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(op), xbytes(in), xq(refs[i].String()))
	}
	o.WriteString("]\n")
	o.WriteString(protobufCheckXo)
	return o.String()
}

const protobufCheckXo = `
fn err_kind(e: protobuf.ProtoErr) -> Str {
  match e {
    Truncated => "Truncated"
    Overflow => "Overflow"
    BadTag => "BadTag"
    BadWire(_) => "BadWire"
    EndGroup => "EndGroup"
    Recursion => "Recursion"
    BadLength => "BadLength"
    Utf8 => "Utf8"
  }
}

fn show[T: Display](r: Result[(T, Int), protobuf.ProtoErr]) -> Str {
  match r {
    Ok((v, n)) => "ok ${v} ${n}"
    Err(e) => "err ${err_kind(e)}"
  }
}

fn num_of(data: Bytes) -> Int { Int.parse(data.to_str_lossy()) ?? 0 }

/// The bits of an unsigned decimal up to 2^64-1.
fn ubits_of(data: Bytes) -> Int {
  var acc = 0
  for c in data {
    acc = acc.wrapping_mul(10).wrapping_add(Int(c) - 48)
  }
  acc
}

/// What this module answers for one case, in the oracle's form.
fn run(op: Str, data: Bytes) -> Str {
  if op == "varint" {
    return show(protobuf.read_varint(data, 0))
  }
  if op == "tag" {
    return match protobuf.read_tag(data, 0) {
      Ok((num, wire, n)) => "ok ${num} ${wire} ${n}"
      Err(e) => "err ${err_kind(e)}"
    }
  }
  if op == "fixed32" {
    return show(protobuf.read_fixed32(data, 0))
  }
  if op == "fixed64" {
    return show(protobuf.read_fixed64(data, 0))
  }
  if op == "bytes" {
    return match protobuf.read_bytes(data, 0) {
      Ok((v, n)) => "ok ${v.to_hex()} ${n}"
      Err(e) => "err ${err_kind(e)}"
    }
  }
  if op == "field" {
    return match protobuf.read_tag(data, 0) {
      Err(e) => "err ${err_kind(e)}"
      Ok((num, wire, n)) => {
        match protobuf.skip_field(data, n, num, wire) {
          Ok(end) => "ok ${num} ${wire} ${end}"
          Err(e) => "err ${err_kind(e)}"
        }
      }
    }
  }
  if op == "encvarint" {
    let v = UInt64.from_bits(ubits_of(data))
    let b = protobuf.append_varint(b"", v)
    return "ok ${b.to_hex()} ${protobuf.size_varint(v)}"
  }
  if op == "zigzag" {
    let z = protobuf.encode_zigzag(num_of(data))
    return "ok ${z} ${protobuf.decode_zigzag(z)}"
  }
  if op == "msg" {
    let f = data.to_str_lossy().split(" ")
    let m = M{id: Int.parse(f[0]) ?? 0, name: unhex(f[1]).to_str_lossy(), z: Int.parse(f[2]) ?? 0, ok: f[3] == "1", f: Float.from_bits(ubits_of(f[4].bytes())), raw: unhex(f[5])}
    return "ok ${protobuf.marshal(m).to_hex()} 0"
  }
  if op == "unmsg" {
    return match protobuf.unmarshal[M](data) {
      Ok(r) => "ok ${r.id} ${r.name.bytes().to_hex()} ${r.z} ${r.ok} ${UInt64.from_bits(r.f.to_bits())} ${r.raw.to_hex()} 0"
      Err(_) => "err err"
    }
  }
  if op == "rmsg" {
    let f = data.to_str_lossy().split(" ")
    var r = R{id: Int.parse(f[0]) ?? 0, nums: [], tags: [], maybe: None, inner: In{a: Int.parse(f[4]) ?? 0}, inners: [], mo: None, ds: [], ms: {}, mn: {}, mb: {}}
    if f[1] != "-" {
      for x in f[1].split(",") { r.nums.push(Int.parse(x) ?? 0) }
    }
    if f[2] != "-" {
      for x in f[2].split(",") { r.tags.push(if x == "~" { "" } else { unhex(x).to_str_lossy() }) }
    }
    if f[3] != "n" { r.maybe = Some(Int.parse(f[3]) ?? 0) }
    if f[5] != "-" {
      for x in f[5].split(",") { r.inners.push(In{a: Int.parse(x) ?? 0}) }
    }
    if f[6] != "n" { r.mo = Some(In{a: Int.parse(f[6]) ?? 0}) }
    if f[7] != "-" {
      for x in f[7].split(",") { r.ds.push(Float.from_bits(ubits_of(x.bytes()))) }
    }
    if f[8] != "-" {
      for kv in f[8].split(",") {
        let p = kv.split("=")
        r.ms[p[0]] = Int.parse(p[1]) ?? 0
      }
    }
    if f[9] != "-" {
      for kv in f[9].split(",") {
        let p = kv.split("=")
        r.mn[Int.parse(p[0]) ?? 0] = In{a: Int.parse(p[1]) ?? 0}
      }
    }
    if f[10] != "-" {
      for kv in f[10].split(",") {
        let p = kv.split("=")
        r.mb[p[0] == "true"] = if p[1] == "~" { "" } else { unhex(p[1]).to_str_lossy() }
      }
    }
    return "ok ${protobuf.marshal(r).to_hex()} 0"
  }
  if op == "runmsg" {
    return match protobuf.unmarshal[R](data) {
      Ok(r) => "ok ${show_rep(r)} 0"
      Err(_) => "err err"
    }
  }
  if op == "omsg" {
    let f = data.to_str_lossy().split(" ")
    var ch: Choice? = None
    if f[1] == "c" { ch = Some(Choice.Circle(Int.parse(f[2]) ?? 0)) }
    if f[1] == "s" { ch = Some(Choice.Sq(Float.from_bits(ubits_of(f[2].bytes())))) }
    if f[1] == "l" { ch = Some(Choice.Lbl(if f[2] == "~" { "" } else { unhex(f[2]).to_str_lossy() })) }
    if f[1] == "i" { ch = Some(Choice.Inn(In{a: Int.parse(f[2]) ?? 0})) }
    let o = O{id: Int.parse(f[0]) ?? 0, ch: ch, tail: if f[3] == "~" { "" } else { unhex(f[3]).to_str_lossy() }}
    return "ok ${protobuf.marshal(o).to_hex()} 0"
  }
  if op == "ounmsg" {
    return match protobuf.unmarshal[O](data) {
      Ok(o) => "ok ${show_one(o)} 0"
      Err(_) => "err err"
    }
  }
  "unknown op ${op}"
}

fn show_one(o: O) -> Str {
  var kind = "n"
  var val = "-"
  if let Some(c) = o.ch {
    match c {
      Circle(r) => {
        kind = "c"
        val = "${r}"
      }
      Sq(s) => {
        kind = "s"
        val = "${UInt64.from_bits(s.to_bits())}"
      }
      Lbl(t) => {
        kind = "l"
        val = if t.is_empty() { "~" } else { t.bytes().to_hex() }
      }
      Inn(i) => {
        kind = "i"
        val = "${i.a}"
      }
    }
  }
  "${o.id} ${kind} ${val} ${if o.tail.is_empty() { "~" } else { o.tail.bytes().to_hex() }}"
}

fn dash(p: List[Str]) -> Str { if p.is_empty() { "-" } else { p.join(",") } }

fn show_rep(r: R) -> Str {
  var nums: List[Str] = []
  for x in r.nums { nums.push("${x}") }
  var tags: List[Str] = []
  for t in r.tags { tags.push(if t.is_empty() { "~" } else { t.bytes().to_hex() }) }
  var inners: List[Str] = []
  for i in r.inners { inners.push("${i.a}") }
  var ds: List[Str] = []
  for d in r.ds { ds.push("${UInt64.from_bits(d.to_bits())}") }
  let maybe = if let Some(m) = r.maybe { "${m}" } else { "n" }
  let mo = if let Some(m) = r.mo { "${m.a}" } else { "n" }
  var ms: List[Str] = []
  for (k, v) in r.ms.entries() { ms.push("${k}=${v}") }
  var mn: List[Str] = []
  for (k, v) in r.mn.entries() { mn.push("${k}=${v.a}") }
  var mb: List[Str] = []
  for (k, v) in r.mb.entries() { mb.push("${k}=${if v.is_empty() { "~" } else { v.bytes().to_hex() }}") }
  "${r.id} ${dash(nums)} ${dash(tags)} ${maybe} ${r.inner.a} ${dash(inners)} ${mo} ${dash(ds)} ${dash(ms.sort())} ${dash(mn.sort())} ${dash(mb.sort())}"
}

fn unhex(s: Str) -> Bytes { if s == "-" { b"" } else { Bytes.from_hex(s) ?? b"" } }

test "std/protobuf answers what Go's protowire answers on every case" {
  var bad: List[Str] = []
  for (op, data, want) in CASES {
    let got = run(op, data)
    if got != want {
      bad.push("${op} ${data.to_hex()}: got ${got} want ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} cases differ, first: ${bad[0]}")
  }
}
`
