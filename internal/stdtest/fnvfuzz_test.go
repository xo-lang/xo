// A fuzz harness for std/fnv against Go's hash/fnv (decision 0616's fuzz
// kind). Each case is random bytes (0 to 600 of them, from runs of one
// byte, all 256 values, text and long runs of 0x00 and 0xff, so the
// multiply chains see every byte class) and a split point:
//
//   - sum32, sum32a, sum64, sum64a, sum128 and sum128a give what New32,
//     New32a, New64, New64a, New128 and New128a give;
//   - each update form continued from the sum of the bytes before the
//     split gives the sum of the whole, as Go's Write in two parts.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/fnvfuzz/fnvfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestFnvFuzzCorpusCurrent checks it is what this
// generator writes (XO_FNVFUZZ_WRITE=1 rewrites it). TestFnvFuzz runs a
// fresh seed (XO_FNVFUZZ_SEED, default 2; XO_FNVFUZZ_N cases, default
// 2000) on XO_FNVFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"encoding"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/fnv"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

// fvCase is one input, its split, and Go's six sums.
type fvCase struct {
	data                     []byte
	split                    int
	s32, s32a                uint32
	s64, s64a                uint64
	s128, s128a, part, parta string // hex; part and parta: New128 and New128a over data[:split]
	// hostile, when set: each update from a hostile state (start, its
	// low 32 bits for the 32 bit sums; state128, 16 bytes hex) gives
	// Go's Write on the hash restored to it
	hostile     bool
	start       int64
	state128    string
	f32, f32a   uint32
	f64, f64a   uint64
	f128, f128a string
}

// fvFrom is Go's Write of p on a fresh h restored (UnmarshalBinary, with
// h's own magic) to state.
func fvFrom(h hash.Hash, state, p []byte) string {
	m, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		panic(err)
	}
	if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(append(m[:4:4], state...)); err != nil {
		panic(err)
	}
	return fvSum(h, p)
}

func fvData(r *rand.Rand) []byte {
	n := r.Intn(601)
	if r.Intn(4) == 0 {
		n = r.Intn(9)
	}
	b := make([]byte, 0, n)
	for len(b) < n {
		switch r.Intn(5) {
		case 0:
			c := byte(r.Intn(256))
			for k := r.Intn(40); k > 0 && len(b) < n; k-- {
				b = append(b, c)
			}
		case 1:
			b = append(b, []byte("the quick brown fox é 日本 ")[:1+r.Intn(20)]...)
		case 2:
			b = append(b, 0x00, 0xff)
		default:
			b = append(b, byte(r.Intn(256)))
		}
	}
	return b[:n]
}

func fvSum(h hash.Hash, b []byte) string {
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// fvCases: with hostileStarts, every eighth case also updates from a
// hostile state (task #352), chosen by the case's index so the random
// stream, and the seed 1 corpus, are unchanged.
func fvCases(r *rand.Rand, n int, hostileStarts bool) []fvCase {
	starts := hostile.Ints()
	us := hostile.Uints()
	cs := make([]fvCase, 0, n)
	for len(cs) < n {
		d := fvData(r)
		c := fvCase{data: d, split: r.Intn(len(d) + 1)}
		h32, h32a, h64, h64a := fnv.New32(), fnv.New32a(), fnv.New64(), fnv.New64a()
		h32.Write(d)
		h32a.Write(d)
		h64.Write(d)
		h64a.Write(d)
		c.s32, c.s32a, c.s64, c.s64a = h32.Sum32(), h32a.Sum32(), h64.Sum64(), h64a.Sum64()
		c.s128, c.s128a = fvSum(fnv.New128(), d), fvSum(fnv.New128a(), d)
		c.part, c.parta = fvSum(fnv.New128(), d[:c.split]), fvSum(fnv.New128a(), d[:c.split])
		if hostileStarts && len(cs)%8 == 7 {
			i := len(cs) / 8
			c.hostile = true
			c.start = starts[i%len(starts)]
			s32 := binary.BigEndian.AppendUint32(nil, uint32(c.start))
			s64 := binary.BigEndian.AppendUint64(nil, uint64(c.start))
			s128 := binary.BigEndian.AppendUint64(binary.BigEndian.AppendUint64(nil, us[i%len(us)]), uint64(c.start))
			c.state128 = hex.EncodeToString(s128)
			word := func(h string) uint64 {
				b, _ := hex.DecodeString(h)
				for len(b) < 8 {
					b = append([]byte{0}, b...)
				}
				return binary.BigEndian.Uint64(b)
			}
			c.f32 = uint32(word(fvFrom(fnv.New32(), s32, d)))
			c.f32a = uint32(word(fvFrom(fnv.New32a(), s32, d)))
			c.f64 = word(fvFrom(fnv.New64(), s64, d))
			c.f64a = word(fvFrom(fnv.New64a(), s64, d))
			c.f128, c.f128a = fvFrom(fnv.New128(), s128, d), fvFrom(fnv.New128a(), s128, d)
		}
		cs = append(cs, c)
	}
	return cs
}

const fvCheckXo = `use std/fnv

test "std/fnv sums as Go's hash/fnv" {
  var bad: List[Str] = []
  for (h, _, s32, s32a, s64, s64a, s128, s128a, _, _) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    if fnv.sum32(d) != s32 || fnv.sum32a(d) != s32a || fnv.sum64(d) != s64 || fnv.sum64a(d) != s64a || fnv.sum128(d).to_hex() != s128 || fnv.sum128a(d).to_hex() != s128a {
      bad.push(h)
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} inputs sum differently, first: ${bad[0]}")
  }
}

test "std/fnv updates from a split as Go's Write in two parts" {
  var bad: List[Str] = []
  for (h, k, s32, s32a, s64, s64a, s128, s128a, p128, p128a) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    let a = d[0..k]
    let b = d[k..d.len()]
    let ok = fnv.update32(fnv.sum32(a), b) == s32 && fnv.update32a(fnv.sum32a(a), b) == s32a && fnv.update64(fnv.sum64(a), b) == s64 && fnv.update64a(fnv.sum64a(a), b) == s64a
    let ok128 = fnv.sum128(a).to_hex() == p128 && fnv.sum128a(a).to_hex() == p128a && fnv.update128(fnv.sum128(a), b).to_hex() == s128 && fnv.update128a(fnv.sum128a(a), b).to_hex() == s128a
    if !ok || !ok128 {
      bad.push("${h} at ${k}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} splits differ, first: ${bad[0]}")
  }
}
`

// fvXoFile is the generated test file. Sums are Int: a 64 bit sum is its
// bits as a signed Int, as std/fnv returns it.
func fvXoFile(seed int64, cases []fvCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/fnvfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// hash/fnv; do not edit: XO_FNVFUZZ_WRITE=1 go test ./internal/stdtest -run TestFnvFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(fvCheckXo)
	b.WriteString("\n/// (input hex, split, sum32, sum32a, sum64, sum64a, sum128 hex, sum128a hex, sum128 and sum128a of the input before the split)\nconst CASES: List[(Str, Int, Int, Int, Int, Int, Str, Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%q, %d, %d, %d, %d, %d, %q, %q, %q, %q),\n", hex.EncodeToString(c.data), c.split, c.s32, c.s32a, int64(c.s64), int64(c.s64a), c.s128, c.s128a, c.part, c.parta)
	}
	b.WriteString("]\n")
	var hs []fvCase
	for _, c := range cases {
		if c.hostile {
			hs = append(hs, c)
		}
	}
	if len(hs) > 0 {
		b.WriteString(fvHostileXo)
		b.WriteString("\n/// (start, 128 bit state hex, input hex, Go's Write from each state: 32, 32a, 64, 64a, 128, 128a)\nconst STARTS: List[(Int, Str, Str, Int, Int, Int, Int, Str, Str)] = [\n")
		for _, c := range hs {
			fmt.Fprintf(&b, "  (%d, %q, %q, %d, %d, %d, %d, %q, %q),\n", c.start, c.state128, hex.EncodeToString(c.data), c.f32, c.f32a, int64(c.f64), int64(c.f64a), c.f128, c.f128a)
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// fvCoverage fails when the cases stop reaching empty, short and long
// inputs and splits at both ends.
func fvCoverage(t *testing.T, cases []fvCase) {
	t.Helper()
	empty, short, long, atStart, atEnd := 0, 0, 0, 0, 0
	for _, c := range cases {
		switch {
		case len(c.data) == 0:
			empty++
		case len(c.data) < 9:
			short++
		case len(c.data) > 300:
			long++
		}
		if c.split == 0 {
			atStart++
		}
		if c.split == len(c.data) {
			atEnd++
		}
	}
	n := len(cases)
	if empty < n/100 || short < n/10 || long < n/5 || atStart < n/50 || atEnd < n/50 {
		t.Fatalf("coverage fell: %d empty, %d short, %d long, %d split at 0, %d split at the end of %d", empty, short, long, atStart, atEnd, n)
	}
}

const fvCorpus = "testdata/corpus/fnvfuzz/fnvfuzz_test.xo"

// TestFnvFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestFnvFuzzCorpusCurrent(t *testing.T) {
	cases := fvCases(rand.New(rand.NewSource(1)), 500, false)
	fvCoverage(t, cases)
	text := fvXoFile(1, cases)
	if os.Getenv("XO_FNVFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(fvCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fvCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(fvCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's hash/fnv or the generator changed): XO_FNVFUZZ_WRITE=1 go test ./internal/stdtest -run TestFnvFuzzCorpusCurrent", fvCorpus)
	}
}

// TestFnvFuzz runs a fresh seed's cases.
func TestFnvFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the fnv fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_FNVFUZZ_SEED", 2))
	n := envInt(t, "XO_FNVFUZZ_N", 2000)
	cases := fvCases(rand.New(rand.NewSource(seed)), n, seed != 1)
	runFuzzDir(t, "fnvfuzz_test.xo", fvXoFile(seed, cases), envBackends("XO_FNVFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}

// fvHostileXo checks each update from a hostile state (task #352): Int's
// edges, whose high bits the 32 bit sums drop, and 128 bit states with
// the top bit set.
const fvHostileXo = `
test "std/fnv updates from a hostile state as Go's Write on the restored hash" {
  var bad: List[Str] = []
  for (start, st, h, s32, s32a, s64, s64a, s128, s128a) in STARTS {
    let d = Bytes.from_hex(h).or_fault("hex")
    let s = Bytes.from_hex(st).or_fault("hex")
    let ok = fnv.update32(start, d) == s32 && fnv.update32a(start, d) == s32a && fnv.update64(start, d) == s64 && fnv.update64a(start, d) == s64a
    if !ok || fnv.update128(s, d).to_hex() != s128 || fnv.update128a(s, d).to_hex() != s128a {
      bad.push("from ${start} (${st}), ${d.len()} bytes ${h}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${STARTS.len()} updates from a hostile state differ, first: ${bad[0]}")
  }
}
`
