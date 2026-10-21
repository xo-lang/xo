package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"strings"
)

func binInt(r *rand.Rand) int64 {
	switch r.Intn(6) {
	case 0:
		return []int64{0, 1, -1, 127, 128, 255, 256, 65535, 65536, -32768, 32767, math.MaxInt64, math.MinInt64, 1 << 32, -(1 << 31), 1<<63 - 1}[r.Intn(16)]
	case 1:
		return int64(r.Intn(1 << 16))
	case 2:
		return -int64(r.Uint64() >> uint(r.Intn(64)))
	}
	return int64(r.Uint64() >> uint(r.Intn(64)))
}

func binaryCorpus() string {
	r := rand.New(rand.NewSource(12))
	var o strings.Builder
	o.WriteString(header("encoding/binary's BigEndian and LittleEndian Put and Uint (with int16 and int32 views) and its varints as std/rt/binary.go calls them, on seeded random values and bytes"))
	o.WriteString("use std/binary\n\n/// (n, Go's u16_be u32_be u64_be u16_le u32_le u64_le uvarint varint in hex)\nconst PUT: List[(Int, Str)] = [\n")
	for i := 0; i < 800; i++ {
		n := binInt(r)
		u := uint64(n)
		b2, b4, b8 := make([]byte, 2), make([]byte, 4), make([]byte, 8)
		parts := []string{}
		binary.BigEndian.PutUint16(b2, uint16(u))
		parts = append(parts, hex.EncodeToString(b2))
		binary.BigEndian.PutUint32(b4, uint32(u))
		parts = append(parts, hex.EncodeToString(b4))
		binary.BigEndian.PutUint64(b8, u)
		parts = append(parts, hex.EncodeToString(b8))
		binary.LittleEndian.PutUint16(b2, uint16(u))
		parts = append(parts, hex.EncodeToString(b2))
		binary.LittleEndian.PutUint32(b4, uint32(u))
		parts = append(parts, hex.EncodeToString(b4))
		binary.LittleEndian.PutUint64(b8, u)
		parts = append(parts, hex.EncodeToString(b8))
		parts = append(parts, hex.EncodeToString(binary.AppendUvarint(nil, u)), hex.EncodeToString(binary.AppendVarint(nil, n)))
		fmt.Fprintf(&o, "  (%d, %q),\n", n, strings.Join(parts, " "))
	}
	o.WriteString("]\n\n/// (bytes, at, Go's reads: u16 u32 u64 i16 i32 big-endian, then little-endian, - past the end; uvarint and varint: VALUE/N or none)\nconst GET: List[(Bytes, Int, Str)] = [\n")
	for i := 0; i < 1200; i++ {
		b := make([]byte, r.Intn(14))
		r.Read(b)
		if r.Intn(3) == 0 {
			for k := range b {
				if r.Intn(3) > 0 {
					b[k] |= 0x80
				}
			}
		}
		at := 0
		if len(b) > 0 {
			at = r.Intn(len(b) + 1)
		}
		w := []string{}
		for _, ord := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
			for _, sz := range []int{2, 4, 8, 2, 4} {
				if at+sz > len(b) {
					w = append(w, "-")
					continue
				}
				s := b[at : at+sz]
				switch len(w) % 5 {
				case 0:
					w = append(w, fmt.Sprint(ord.Uint16(s)))
				case 1:
					w = append(w, fmt.Sprint(ord.Uint32(s)))
				case 2:
					w = append(w, fmt.Sprint(int64(ord.Uint64(s))))
				case 3:
					w = append(w, fmt.Sprint(int16(ord.Uint16(s))))
				default:
					w = append(w, fmt.Sprint(int32(ord.Uint32(s))))
				}
			}
		}
		if v, n := binary.Uvarint(b[at:]); n > 0 {
			w = append(w, fmt.Sprintf("%d/%d", int64(v), n))
		} else {
			w = append(w, "none")
		}
		if v, n := binary.Varint(b[at:]); n > 0 {
			w = append(w, fmt.Sprintf("%d/%d", v, n))
		} else {
			w = append(w, "none")
		}
		fmt.Fprintf(&o, "  (%s, %d, %q),\n", xbytes(b), at, strings.Join(w, " "))
	}
	o.WriteString("]\n")
	o.WriteString(binaryTests)
	return o.String()
}

const binaryTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "the puts and varints write Go's bytes" {
  var bad: List[(Int, Str, Str)] = []
  for (n, want) in PUT {
    let got = [binary.u16_be(n), binary.u32_be(n), binary.u64_be(n), binary.u16_le(n), binary.u32_le(n), binary.u64_le(n), binary.uvarint(n), binary.varint(n)].map(fn(b) { b.to_hex() }).join(" ")
    if got != want {
      bad.push((n, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

fn var_text(r: (Int, Int)?) -> Str {
  match r {
    Some((v, n)) => "${v}/${n}"
    None => "none"
  }
}

test "the reads give Go's values" {
  var bad: List[(Bytes, Int, Str, Str)] = []
  for (b, at, want) in GET {
    var w: List[Str] = []
    let fits = fn(sz: Int) -> Bool { at + sz <= b.len() }
    w.push(if fits(2) { "${binary.get_u16_be(b, at)}" } else { "-" })
    w.push(if fits(4) { "${binary.get_u32_be(b, at)}" } else { "-" })
    w.push(if fits(8) { "${binary.get_u64_be(b, at)}" } else { "-" })
    w.push(if fits(2) { "${binary.get_i16_be(b, at)}" } else { "-" })
    w.push(if fits(4) { "${binary.get_i32_be(b, at)}" } else { "-" })
    w.push(if fits(2) { "${binary.get_u16_le(b, at)}" } else { "-" })
    w.push(if fits(4) { "${binary.get_u32_le(b, at)}" } else { "-" })
    w.push(if fits(8) { "${binary.get_u64_le(b, at)}" } else { "-" })
    w.push(if fits(2) { "${binary.get_i16_le(b, at)}" } else { "-" })
    w.push(if fits(4) { "${binary.get_i32_le(b, at)}" } else { "-" })
    w.push(var_text(binary.get_uvarint(b, at)))
    w.push(var_text(binary.get_varint(b, at)))
    let got = w.join(" ")
    if got != want {
      bad.push((b, at, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
