// Command protowireoracle prints the std/protowire corpus: seeded random
// protobuf wire messages (every wire type, nested groups, edge field numbers,
// varints of every length) and changed ones (cut short, a reserved wire type,
// field number 0, an overlong varint, a length past the end, an end group
// that does not match, groups nested past the limit), each walked with
// protobuf-go v1.36.12's protowire.ConsumeTag and ConsumeFieldValue; and
// varints and tags written with AppendVarint and AppendTag, as an in-module
// test of std/protowire (its Fld fields are std's own). std/protowire's
// walk must give the same fields or the same error, and wv and wtag the same
// bytes. XO_PROTOWIREFUZZ_SEED (default 0, the committed corpus) is added to
// the seed, for the fresh cases of TestProtowireFuzz (task #116). It is its
// own module so the repository's go.mod has no third-party requirement.
package main

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

func fuzzSeed() int64 {
	seed, _ := strconv.ParseInt(os.Getenv("XO_PROTOWIREFUZZ_SEED"), 10, 64)
	return seed
}

// errName is the std/protobuf ProtoErr variant for protowire's error code.
func errName(n int) string {
	switch n {
	case -1:
		return "err:Truncated"
	case -2:
		return "err:BadTag"
	case -3:
		return "err:Overflow"
	case -4:
		return "err:BadWire"
	case -5:
		return "err:EndGroup"
	case -6:
		return "err:Recursion"
	}
	return "err:code" + strconv.Itoa(n)
}

// walk lists b's fields as num/wire/value/data hex/raw length, or the error.
func walk(b []byte) string {
	var sb strings.Builder
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return errName(n)
		}
		m := protowire.ConsumeFieldValue(num, typ, b[n:])
		if m < 0 {
			return errName(m)
		}
		var v uint64
		var d []byte
		switch typ {
		case protowire.VarintType:
			v, _ = protowire.ConsumeVarint(b[n:])
		case protowire.Fixed64Type:
			v, _ = protowire.ConsumeFixed64(b[n:])
		case protowire.Fixed32Type:
			x, _ := protowire.ConsumeFixed32(b[n:])
			v = uint64(x)
		case protowire.BytesType:
			d, _ = protowire.ConsumeBytes(b[n:])
		case protowire.StartGroupType:
			d, _ = protowire.ConsumeGroup(num, b[n:])
		}
		fmt.Fprintf(&sb, "%d/%d/%d/%x/%d;", num, typ, int64(v), d, n+m)
		b = b[n+m:]
	}
	return sb.String()
}

func num(r *rand.Rand) protowire.Number {
	switch r.Intn(4) {
	case 0:
		return []protowire.Number{1, 2, 15, 16, 2047, 2048, 18999, 19000, 19999, 20000, protowire.MaxValidNumber}[r.Intn(11)]
	case 1:
		return protowire.Number(1 + r.Intn(int(protowire.MaxValidNumber)))
	}
	return protowire.Number(1 + r.Intn(40))
}

func varint(r *rand.Rand) uint64 {
	switch r.Intn(4) {
	case 0:
		return []uint64{0, 1, 127, 128, 16383, 16384, math.MaxUint32, 1 << 63, math.MaxUint64, math.MaxInt64}[r.Intn(10)]
	case 1:
		return r.Uint64() >> uint(r.Intn(64))
	}
	return uint64(r.Intn(300))
}

func msg(r *rand.Rand, depth int) []byte {
	var b []byte
	for k := r.Intn(6); k > 0; k-- {
		n := num(r)
		switch r.Intn(6) {
		case 0, 1:
			b = protowire.AppendTag(b, n, protowire.VarintType)
			b = protowire.AppendVarint(b, varint(r))
		case 2:
			b = protowire.AppendTag(b, n, protowire.Fixed64Type)
			b = protowire.AppendFixed64(b, r.Uint64())
		case 3:
			b = protowire.AppendTag(b, n, protowire.Fixed32Type)
			b = protowire.AppendFixed32(b, r.Uint32())
		case 4:
			var d []byte
			if depth < 3 && r.Intn(2) == 0 {
				d = msg(r, depth+1)
			} else {
				d = make([]byte, r.Intn(20))
				r.Read(d)
				if r.Intn(2) == 0 {
					d = []byte([]string{"", "hello", "é日本", "a\x00b"}[r.Intn(4)])
				}
			}
			b = protowire.AppendTag(b, n, protowire.BytesType)
			b = protowire.AppendBytes(b, d)
		case 5:
			if depth < 3 {
				b = protowire.AppendTag(b, n, protowire.StartGroupType)
				b = append(b, msg(r, depth+1)...)
				b = protowire.AppendTag(b, n, protowire.EndGroupType)
			}
		}
	}
	return b
}

// change makes b invalid, or valid in an unusual way.
func change(r *rand.Rand, b []byte) []byte {
	at := 0
	if len(b) > 0 {
		at = r.Intn(len(b) + 1)
	}
	head, tail := append([]byte{}, b[:at]...), b[at:]
	switch r.Intn(9) {
	case 0:
		return b[:at]
	case 1:
		if len(b) > 0 {
			c := append([]byte{}, b...)
			c[r.Intn(len(c))] = byte(r.Intn(256))
			return c
		}
	case 2:
		head = protowire.AppendVarint(head, uint64(num(r))<<3|uint64(6+r.Intn(2)))
	case 3:
		head = protowire.AppendVarint(head, uint64(r.Intn(8)))
	case 4:
		// an overlong varint: ten bytes with a last byte past 1, or eleven
		head = append(head, 8)
		for i := 0; i < 9; i++ {
			head = append(head, 0x80|byte(r.Intn(128)))
		}
		head = append(head, []byte{0x02, 0x01, 0x80, 0x00}[r.Intn(4)])
	case 5:
		head = protowire.AppendTag(head, num(r), protowire.BytesType)
		head = protowire.AppendVarint(head, []uint64{1000, 1 << 40, math.MaxUint64, math.MaxInt64}[r.Intn(4)])
	case 6:
		head = protowire.AppendTag(head, num(r), protowire.EndGroupType)
	case 7:
		n := num(r)
		head = protowire.AppendTag(head, n, protowire.StartGroupType)
		head = protowire.AppendTag(head, n%protowire.MaxValidNumber+1, protowire.EndGroupType)
	case 8:
		// a tag past MaxValidNumber, or one past int32
		head = protowire.AppendVarint(head, uint64([]int64{int64(protowire.MaxValidNumber) + 1, math.MaxInt32, math.MaxInt32 + 1, 1 << 40}[r.Intn(4)])<<3)
		head = append(head, 0)
	}
	return append(head, tail...)
}

func main() {
	r := rand.New(rand.NewSource(616 + fuzzSeed()))
	var o strings.Builder
	fmt.Fprintf(&o, "// Generated by scripts/stdcorpus/protowireoracle (seed %d) from protobuf-go v1.36.12's\n// protowire (Copyright 2018 The Go Authors, BSD-style license). Do not edit:\n// XO_PROTOWIREFUZZ_WRITE=1 go test ./internal/stdtest -run TestProtowireFuzzCorpusCurrent\n// rewrites it.\n", fuzzSeed())
	o.WriteString(`
use std/protobuf

fn walked(b: Bytes) -> Str {
  match walk(b) {
    Ok(fs) => fs.map(fn(f: Fld) -> Str { "${f.num}/${f.wire}/${f.v}/${f.data.to_hex()}/${f.raw.len()};" }).join("")
    Err(e) => "err:" + match e {
      protobuf.ProtoErr.Truncated => "Truncated"
      protobuf.ProtoErr.Overflow => "Overflow"
      protobuf.ProtoErr.BadTag => "BadTag"
      protobuf.ProtoErr.BadWire(_) => "BadWire"
      protobuf.ProtoErr.EndGroup => "EndGroup"
      protobuf.ProtoErr.Recursion => "Recursion"
      protobuf.ProtoErr.BadLength => "BadLength"
      protobuf.ProtoErr.Utf8 => "Utf8"
      _ => "other"
    }
  }
}

test "walk reads protowire's fields, or fails where ConsumeTag and ConsumeFieldValue do" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (b, want)) in WALK.indexed() {
    var got = ""
    if b.starts_with("deep:") {
      let n = Int.parse(b[5..]).or_fault("deep")
      var o = BytesBuilder.new()
      for _ in 0..n {
        o.write_byte(0x0b)
      }
      for _ in 0..n {
        o.write_byte(0x0c)
      }
      got = walked(o.freeze())
      if !got.starts_with("err:") {
        got = "ok"
      }
    } else {
      got = walked(Bytes.from_hex(b).or_fault("hex"))
    }
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "wv and wtag write protowire's AppendVarint and AppendTag" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (n, v, want)) in WRITE.indexed() {
    var o = BytesBuilder.new()
    wv(o, v)
    if n > 0 {
      wtag(o, n, i % 6)
    }
    let got = o.freeze().to_hex()
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

/// (the message in hex, protowire's walk: num/wire/value/data/raw length per field, or err:Kind)
const WALK: List[(Str, Str)] = [
`)
	// groups nested n deep (field 1), around protowire's limit: the row's
	// input is deep:n, which walked expands
	for _, n := range []int{1, 2, 100, 9998, 9999, 10000, 10001, 10002, 9990 + r.Intn(20)} {
		var b []byte
		for i := 0; i < n; i++ {
			b = protowire.AppendTag(b, 1, protowire.StartGroupType)
		}
		for i := 0; i < n; i++ {
			b = protowire.AppendTag(b, 1, protowire.EndGroupType)
		}
		w := walk(b)
		if !strings.HasPrefix(w, "err:") {
			w = "ok"
		}
		fmt.Fprintf(&o, "  (\"deep:%d\", %q),\n", n, w)
	}
	for i := 0; i < 1500; i++ {
		b := msg(r, 0)
		if r.Intn(2) == 0 {
			b = change(r, b)
		}
		fmt.Fprintf(&o, "  (%q, %q),\n", hex.EncodeToString(b), walk(b))
	}
	o.WriteString("]\n\n/// (field number or 0, a varint as Int, protowire's AppendVarint then AppendTag(num, row index % 6) in hex)\nconst WRITE: List[(Int, Int, Str)] = [\n")
	for i := 0; i < 600; i++ {
		v := varint(r)
		n := protowire.Number(0)
		if r.Intn(4) != 0 {
			n = num(r)
		}
		b := protowire.AppendVarint(nil, v)
		if n > 0 {
			b = protowire.AppendTag(b, n, protowire.Type(i%6))
		}
		fmt.Fprintf(&o, "  (%d, %d, %q),\n", n, int64(v), hex.EncodeToString(b))
	}
	o.WriteString("]\n")
	fmt.Print(o.String())
}
