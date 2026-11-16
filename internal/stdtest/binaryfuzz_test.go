// A fuzz harness for std/binary against Go's encoding/binary (decision
// 0616's fuzz kind). Each case is either
//
//   - a value (small, near a power of two, negative, the Int extremes or
//     any 64 bit word), whose u16/u32/u64 big and little endian puts and
//     uvarint and varint must write Go's bytes (BigEndian.PutUintN,
//     LittleEndian.PutUintN, AppendUvarint, AppendVarint); or
//   - bytes and an offset (random bytes, Go's varints of random values
//     cut short or not, the 10 byte overflow forms, runs of 0x80 and
//     0xff), whose u16/u32/u64 and i16/i32 reads in both orders must give
//     Go's values ("-" past the end, where both fault) and whose
//     get_uvarint and get_varint must give Go's Uvarint and Varint value
//     and length, or none where Go's length is 0 or negative.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/binaryfuzz/binaryfuzz_test.xo, which TestStdCorpora runs
// on every corpus backend; TestBinaryFuzzCorpusCurrent checks it is what
// this generator writes (XO_BINARYFUZZ_WRITE=1 rewrites it).
// TestBinaryFuzz runs a fresh seed (XO_BINARYFUZZ_SEED, default 2;
// XO_BINARYFUZZ_N cases, default 2000) on XO_BINARYFUZZ_BACKENDS (default
// arm64 and llvm).
package stdtest

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
)

// bnPut is a value and Go's puts and varints in hex, space separated.
type bnPut struct {
	n    int64
	want string
}

// bnGet is bytes, an offset and Go's reads.
type bnGet struct {
	data []byte
	at   int
	want string
}

func bnValue(r *rand.Rand) int64 {
	switch r.Intn(7) {
	case 0:
		return int64(r.Intn(300)) - 150
	case 1:
		k := uint(r.Intn(64))
		return int64(uint64(1)<<k) + int64(r.Intn(5)) - 2
	case 2:
		return []int64{0, 1, -1, math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1, 127, 128, -64, -65, 1 << 63 >> 7}[r.Intn(12)]
	case 3:
		return -int64(r.Uint64() >> uint(r.Intn(64)))
	default:
		return int64(r.Uint64() >> uint(r.Intn(64)))
	}
}

func bnPutOf(n int64) bnPut {
	u := uint64(n)
	var b2 [2]byte
	var b4 [4]byte
	var b8 [8]byte
	var parts []string
	binary.BigEndian.PutUint16(b2[:], uint16(u))
	parts = append(parts, hex.EncodeToString(b2[:]))
	binary.BigEndian.PutUint32(b4[:], uint32(u))
	parts = append(parts, hex.EncodeToString(b4[:]))
	binary.BigEndian.PutUint64(b8[:], u)
	parts = append(parts, hex.EncodeToString(b8[:]))
	binary.LittleEndian.PutUint16(b2[:], uint16(u))
	parts = append(parts, hex.EncodeToString(b2[:]))
	binary.LittleEndian.PutUint32(b4[:], uint32(u))
	parts = append(parts, hex.EncodeToString(b4[:]))
	binary.LittleEndian.PutUint64(b8[:], u)
	parts = append(parts, hex.EncodeToString(b8[:]))
	parts = append(parts, hex.EncodeToString(binary.AppendUvarint(nil, u)))
	parts = append(parts, hex.EncodeToString(binary.AppendVarint(nil, n)))
	return bnPut{n, strings.Join(parts, " ")}
}

func bnBytes(r *rand.Rand) []byte {
	var b []byte
	for k := r.Intn(4); k >= 0; k-- {
		switch r.Intn(6) {
		case 0:
			v := binary.AppendUvarint(nil, uint64(bnValue(r)))
			b = append(b, v[:r.Intn(len(v))+1]...) // cut short or whole
		case 1:
			b = binary.AppendVarint(b, bnValue(r))
		case 2:
			// the 10 byte forms: a last byte of 0, 1 (in range) or more (overflow), or none
			for i := 0; i < 9; i++ {
				b = append(b, 0x80|byte(r.Intn(128)))
			}
			if r.Intn(4) != 0 {
				b = append(b, byte(r.Intn(4)))
			}
		case 3:
			c := []byte{0x80, 0xff, 0x00, 0x7f}[r.Intn(4)]
			for i := r.Intn(14); i > 0; i-- {
				b = append(b, c)
			}
		default:
			for i := r.Intn(12); i > 0; i-- {
				b = append(b, byte(r.Intn(256)))
			}
		}
	}
	return b
}

func bnVar(v int64, n int) string {
	if n <= 0 {
		return "none"
	}
	return fmt.Sprintf("%d/%d", v, n)
}

func bnGetOf(b []byte, at int) bnGet {
	var w []string
	rd := func(sz int, f func([]byte) int64) {
		if at+sz <= len(b) {
			w = append(w, fmt.Sprint(f(b[at:])))
		} else {
			w = append(w, "-")
		}
	}
	for _, o := range []binary.ByteOrder{binary.BigEndian, binary.LittleEndian} {
		rd(2, func(x []byte) int64 { return int64(o.Uint16(x)) })
		rd(4, func(x []byte) int64 { return int64(o.Uint32(x)) })
		rd(8, func(x []byte) int64 { return int64(o.Uint64(x)) })
		rd(2, func(x []byte) int64 { return int64(int16(o.Uint16(x))) })
		rd(4, func(x []byte) int64 { return int64(int32(o.Uint32(x))) })
	}
	uv, un := binary.Uvarint(b[at:])
	w = append(w, bnVar(int64(uv), un))
	sv, sn := binary.Varint(b[at:])
	w = append(w, bnVar(sv, sn))
	return bnGet{b, at, strings.Join(w, " ")}
}

func bnCases(r *rand.Rand, n int) ([]bnPut, []bnGet) {
	var puts []bnPut
	var gets []bnGet
	for len(puts)+len(gets) < n {
		if r.Intn(2) == 0 {
			puts = append(puts, bnPutOf(bnValue(r)))
			continue
		}
		b := bnBytes(r)
		at := 0
		if len(b) > 0 && r.Intn(3) == 0 {
			at = r.Intn(len(b) + 1)
		}
		gets = append(gets, bnGetOf(b, at))
	}
	return puts, gets
}

const bnCheckXo = `use std/binary

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "std/binary puts and varints write Go's encoding/binary bytes" {
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

test "std/binary reads give Go's encoding/binary values" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (h, at, want) in GET {
    let b = Bytes.from_hex(h).or_fault("hex")
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
      bad.push((h, at, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`

func bnXoFile(seed int64, puts []bnPut, gets []bnGet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/binaryfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(puts)+len(gets))
	b.WriteString("// encoding/binary; do not edit: XO_BINARYFUZZ_WRITE=1 go test ./internal/stdtest -run TestBinaryFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(bnCheckXo)
	b.WriteString("\n/// (n, Go's u16_be u32_be u64_be u16_le u32_le u64_le uvarint varint in hex)\nconst PUT: List[(Int, Str)] = [\n")
	for _, c := range puts {
		fmt.Fprintf(&b, "  (%d, %q),\n", c.n, c.want)
	}
	b.WriteString("]\n\n/// (bytes hex, at, Go's reads: u16 u32 u64 i16 i32 big-endian, then little-endian, - past the end; uvarint and varint: VALUE/N or none)\nconst GET: List[(Str, Int, Str)] = [\n")
	for _, c := range gets {
		fmt.Fprintf(&b, "  (%q, %d, %q),\n", hex.EncodeToString(c.data), c.at, c.want)
	}
	b.WriteString("]\n")
	return b.String()
}

// bnCoverage fails when the cases stop reaching the varint outcomes (a
// value, cut short, overflow) and reads past the end.
func bnCoverage(t *testing.T, gets []bnGet) {
	t.Helper()
	ok, none, overflow, past := 0, 0, 0, 0
	for _, g := range gets {
		_, un := binary.Uvarint(g.data[g.at:])
		switch {
		case un > 0:
			ok++
		case un == 0:
			none++
		default:
			overflow++
		}
		if g.at+8 > len(g.data) {
			past++
		}
	}
	n := len(gets)
	if ok < n/5 || none < n/20 || overflow < n/50 || past < n/10 {
		t.Fatalf("coverage fell: %d uvarints read, %d cut short, %d overflow, %d reads past the end of %d", ok, none, overflow, past, n)
	}
}

const bnCorpus = "testdata/corpus/binaryfuzz/binaryfuzz_test.xo"

// TestBinaryFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestBinaryFuzzCorpusCurrent(t *testing.T) {
	puts, gets := bnCases(rand.New(rand.NewSource(1)), 400)
	bnCoverage(t, gets)
	bxCorpusCurrent(t, bnCorpus, "XO_BINARYFUZZ_WRITE", bnXoFile(1, puts, gets))
}

// TestBinaryFuzz runs a fresh seed's cases.
func TestBinaryFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the binary fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_BINARYFUZZ_SEED", 2))
	n := envInt(t, "XO_BINARYFUZZ_N", 2000)
	puts, gets := bnCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "binaryfuzz_test.xo", bnXoFile(seed, puts, gets), envBackends("XO_BINARYFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(puts)+len(gets))
}
