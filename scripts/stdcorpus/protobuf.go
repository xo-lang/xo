package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	o.WriteString("use std/protobuf\n\n")
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
  "unknown op ${op}"
}

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
