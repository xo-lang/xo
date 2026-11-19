// A fuzz harness for std/crc32 against Go's hash/crc32 (decision 0616's
// fuzz kind). Each case is random bytes (0 to 600 of them, a tenth 600 to
// 9,000 so Castagnoli's three way paths over 504 and 4,032 bytes run, from
// runs of one byte, all 256 values, text and 0x00 0xff pairs), a split
// point and a chunk size:
//
//   - checksum with Ieee (the default) and Castagnoli gives what
//     ChecksumIEEE and Checksum(MakeTable(Castagnoli)) give;
//   - update continued from the checksum of the bytes before the split
//     gives the checksum of the whole, as Go's Update, and update from 0
//     gives the checksum;
//   - update over the bytes in chunks of the chunk size, from 0, gives
//     the one-shot checksum, as Go's hash.Hash32 written in those chunks.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/crc32fuzz/crc32fuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestCrc32FuzzCorpusCurrent checks it is what this
// generator writes (XO_CRC32FUZZ_WRITE=1 rewrites it). TestCrc32Fuzz runs a
// fresh seed (XO_CRC32FUZZ_SEED, default 2; XO_CRC32FUZZ_N cases, default
// 2000) on XO_CRC32FUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"math/rand"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

// crCase is one input, its split and chunk, and Go's two checksums.
type crCase struct {
	data         []byte
	split, chunk int
	ieee, cast   uint32
	// hostile, when set: update from start (a hostile Int, its low 32
	// bits the crc Go's Update takes) gives startIeee and startCast
	hostile              bool
	start                int64
	startIeee, startCast uint32
}

var crCastagnoli = crc32.MakeTable(crc32.Castagnoli)

func crData(r *rand.Rand, n int) []byte {
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

// crCases: with hostileStarts, every eighth case also updates from a
// hostile start (task #352), chosen by the case's index so the random
// stream, and the seed 1 corpus, are unchanged.
func crCases(r *rand.Rand, n, longMax int, hostileStarts bool) []crCase {
	starts := append(hostile.Ints(), 0xffffffff, 0x80000000, 0x7fffffff)
	cs := make([]crCase, 0, n)
	for len(cs) < n {
		k := r.Intn(601)
		switch r.Intn(10) {
		case 0:
			k = 600 + r.Intn(longMax-600+1)
		case 1, 2:
			k = r.Intn(17)
		}
		d := crData(r, k)
		c := crCase{data: d, split: r.Intn(len(d) + 1), chunk: []int{1, 3, 7, 8, 16, 63, 64, 512, 4096}[r.Intn(9)]}
		c.ieee, c.cast = crc32.ChecksumIEEE(d), crc32.Checksum(d, crCastagnoli)
		// Go's hash.Hash32 written in chunks gives the one-shot checksum
		hi, hc := crc32.NewIEEE(), crc32.New(crCastagnoli)
		for i := 0; i < len(d); i += c.chunk {
			hi.Write(d[i:min(i+c.chunk, len(d))])
			hc.Write(d[i:min(i+c.chunk, len(d))])
		}
		if hi.Sum32() != c.ieee || hc.Sum32() != c.cast {
			panic("hash/crc32: chunked Write differs from the checksum")
		}
		if hostileStarts && len(cs)%8 == 7 {
			c.hostile = true
			c.start = starts[(len(cs)/8)%len(starts)]
			c.startIeee = crc32.Update(uint32(c.start), crc32.IEEETable, d)
			c.startCast = crc32.Update(uint32(c.start), crCastagnoli, d)
		}
		cs = append(cs, c)
	}
	return cs
}

const crCheckXo = `use std/crc32

test "std/crc32 checksums as Go's hash/crc32" {
  var bad: List[Str] = []
  for (h, _, _, ieee, cast) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    if crc32.checksum(d) != ieee || crc32.checksum(d, crc32.Poly.Ieee) != ieee || crc32.checksum(d, crc32.Poly.Castagnoli) != cast {
      bad.push("${d.len()} bytes ${h}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} inputs sum differently, first: ${bad[0]}")
  }
}

test "std/crc32 updates from a split and in chunks as Go's Update" {
  var bad: List[Str] = []
  for (h, k, chunk, ieee, cast) in CASES {
    let d = Bytes.from_hex(h).or_fault("hex")
    let n = d.len()
    let a = d[0..k]
    let b = d[k..n]
    var ok = crc32.update(crc32.checksum(a), b) == ieee && crc32.update(crc32.checksum(a, crc32.Poly.Castagnoli), b, crc32.Poly.Castagnoli) == cast
    ok = ok && crc32.update(0, d) == ieee && crc32.update(0, d, crc32.Poly.Castagnoli) == cast
    var ci = 0
    var cc = 0
    var i = 0
    while i < n {
      let e = (i + chunk).min(n)
      ci = crc32.update(ci, d[i..e])
      cc = crc32.update(cc, d[i..e], crc32.Poly.Castagnoli)
      i = e
    }
    if !ok || ci != ieee || cc != cast {
      bad.push("${n} bytes at ${k}, chunks of ${chunk}: ${h}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} updates differ, first: ${bad[0]}")
  }
}
`

func crXoFile(seed int64, cases []crCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/crc32fuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// hash/crc32; do not edit: XO_CRC32FUZZ_WRITE=1 go test ./internal/stdtest -run TestCrc32FuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(crCheckXo)
	b.WriteString("\n/// (input hex, split, chunk, IEEE checksum, Castagnoli checksum)\nconst CASES: List[(Str, Int, Int, Int, Int)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%q, %d, %d, %d, %d),\n", hex.EncodeToString(c.data), c.split, c.chunk, c.ieee, c.cast)
	}
	b.WriteString("]\n")
	var hs []crCase
	for _, c := range cases {
		if c.hostile {
			hs = append(hs, c)
		}
	}
	if len(hs) > 0 {
		b.WriteString(crHostileXo)
		b.WriteString("\n/// (start, input hex, Go's Update from the start's low 32 bits: IEEE, Castagnoli)\nconst STARTS: List[(Int, Str, Int, Int)] = [\n")
		for _, c := range hs {
			fmt.Fprintf(&b, "  (%d, %q, %d, %d),\n", c.start, hex.EncodeToString(c.data), c.startIeee, c.startCast)
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// crHostileXo checks update from a hostile start (task #352): Int's edges
// and the 32 bit ones.
const crHostileXo = `
test "std/crc32 updates from a hostile start as Go's Update from its low 32 bits" {
  var bad: List[Str] = []
  for (start, h, ieee, cast) in STARTS {
    let d = Bytes.from_hex(h).or_fault("hex")
    if crc32.update(start, d) != ieee || crc32.update(start, d, crc32.Poly.Castagnoli) != cast {
      bad.push("from ${start}, ${d.len()} bytes ${h}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${STARTS.len()} updates from a hostile start differ, first: ${bad[0]}")
  }
}
`

// crCoverage fails when the cases stop reaching empty, short and long
// inputs (past Castagnoli's 4,032 byte blocks) and splits at both ends.
func crCoverage(t *testing.T, cases []crCase, longMin int) {
	t.Helper()
	empty, short, long, atStart, atEnd := 0, 0, 0, 0, 0
	for _, c := range cases {
		switch {
		case len(c.data) == 0:
			empty++
		case len(c.data) < 17:
			short++
		case len(c.data) > longMin:
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
	if empty < n/100 || short < n/10 || long < n/100 || atStart < n/50 || atEnd < n/50 {
		t.Fatalf("coverage fell: %d empty, %d short, %d over %d bytes, %d split at 0, %d split at the end of %d", empty, short, long, longMin, atStart, atEnd, n)
	}
}

const crCorpus = "testdata/corpus/crc32fuzz/crc32fuzz_test.xo"

// TestCrc32FuzzCorpusCurrent: the committed corpus is what seed 1 writes.
// Its long inputs stop at 5,000 bytes, past the 4,032 byte block, to keep
// the file small.
func TestCrc32FuzzCorpusCurrent(t *testing.T) {
	cases := crCases(rand.New(rand.NewSource(1)), 300, 5000, false)
	crCoverage(t, cases, 4032)
	bxCorpusCurrent(t, crCorpus, "XO_CRC32FUZZ_WRITE", crXoFile(1, cases))
}

// TestCrc32Fuzz runs a fresh seed's cases.
func TestCrc32Fuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the crc32 fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_CRC32FUZZ_SEED", 2))
	n := envInt(t, "XO_CRC32FUZZ_N", 2000)
	cases := crCases(rand.New(rand.NewSource(seed)), n, 9000, seed != 1)
	runFuzzDir(t, "crc32fuzz_test.xo", crXoFile(seed, cases), envBackends("XO_CRC32FUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
