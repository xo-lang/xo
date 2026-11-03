// A fuzz harness for std/compress against Go's compress/flate, gzip and
// zlib (2026-11-02, decision 0461: a parser of untrusted input is Early on
// testing until it is fuzzed).
//
// std/compress decodes as Go does: the same bytes, Go's error texts
// (decision 0305). The corpus testdata/corpus/compress checks Go-written
// streams with simple corruption; this harness builds the streams a hostile
// upload carries, bit by bit: DEFLATE stored blocks (LEN and NLEN that
// disagree, too few bytes), fixed Huffman blocks (every literal and length
// symbol, distances back past the start of the output, the reserved
// length symbols 286 and 287 and distance codes 30 and 31, no end of
// block), dynamic blocks (a Go-written header with bits changed, or a
// random header), the reserved block type 3, a missing final block; each
// sent raw, in a zlib wrapper (a bad header check, a preset dictionary, a
// wrong Adler-32) or a gzip member (FEXTRA, FNAME, FCOMMENT and FHCRC set
// at random, a bad magic, method or header CRC, a wrong CRC-32 or size, a
// second member, trailing garbage). Go-written streams at every level, cut
// short or with a bit flipped, are a quarter of the cases.
//
// Each case is checked as Go answers it: the one-shot decoder (gunzip,
// inflate, unzlib) gives "ok" and the bytes, or "err" and Go's error text
// exactly; when it succeeds, a Decompressor fed in chunks gives the same
// bytes, and a max_size one byte short fails with std's limit error; a
// Go-written stream that was not changed is also what std's one-shot and
// streaming compressor write at that level, byte for byte.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/compressfuzz/compressfuzz_test.xo, which TestStdCorpora
// runs on every corpus backend; TestCompressFuzzCorpusCurrent checks it is
// what this generator writes (XO_COMPRESSFUZZ_WRITE=1 rewrites it).
// TestCompressFuzz runs a fresh seed (XO_COMPRESSFUZZ_SEED, default 2;
// XO_COMPRESSFUZZ_N cases, default 2000) on XO_COMPRESSFUZZ_BACKENDS
// (default arm64 and llvm).
package stdtest

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"hash/crc32"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var czFormats = []string{"gzip", "deflate", "zlib"}

// czCase is one stream and Go's answer for it.
type czCase struct {
	format string
	stream []byte
	// level and data are set for a Go-written stream left unchanged: std
	// must write the same bytes.
	level int
	data  []byte
	plain bool
	ok    bool
	out   []byte
	err   string
	chunk int
}

// czBits writes DEFLATE's bit order: values least significant bit first,
// Huffman codes most significant bit first.
type czBits struct {
	out  []byte
	acc  uint64
	nacc uint
}

func (w *czBits) bits(v uint64, n uint) {
	w.acc |= v << w.nacc
	w.nacc += n
	for w.nacc >= 8 {
		w.out = append(w.out, byte(w.acc))
		w.acc >>= 8
		w.nacc -= 8
	}
}

func (w *czBits) code(c uint64, n uint) {
	var r uint64
	for i := uint(0); i < n; i++ {
		r = r<<1 | (c>>i)&1
	}
	w.bits(r, n)
}

func (w *czBits) align() {
	if w.nacc > 0 {
		w.bits(0, 8-w.nacc)
	}
}

func (w *czBits) bytes() []byte {
	w.align()
	return w.out
}

// fixedLit writes symbol s (0 to 287) with the fixed literal/length code.
func (w *czBits) fixedLit(s int) {
	switch {
	case s < 144:
		w.code(uint64(0x30+s), 8)
	case s < 256:
		w.code(uint64(0x190+s-144), 9)
	case s < 280:
		w.code(uint64(s-256), 7)
	default:
		w.code(uint64(0xC0+s-280), 8)
	}
}

// Length symbols 257 to 285: base length and extra bits.
var czLenBase = []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
var czLenExtra = []uint{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}

// Distance codes 0 to 29: base distance and extra bits.
var czDistBase = []int{1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577}
var czDistExtra = []uint{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}

// czData is data worth compressing: text with repeats, runs, or random
// bytes, 0 to about 400 bytes.
func czData(r *rand.Rand) []byte {
	var b []byte
	switch r.Intn(5) {
	case 0:
		n := r.Intn(64)
		for i := 0; i < n; i++ {
			b = append(b, byte(r.Intn(256)))
		}
	case 1:
		b = bytes.Repeat([]byte{byte('a' + r.Intn(3))}, r.Intn(400))
	default:
		words := []string{"the ", "xo ", "stream ", "abc", "abcabc", "\n", "zz", "0123456789", "é"}
		for i, n := 0, r.Intn(40); i < n; i++ {
			b = append(b, words[r.Intn(len(words))]...)
		}
	}
	return b
}

// czRawBlocks is hostile raw DEFLATE: one to four blocks.
func czRawBlocks(r *rand.Rand) []byte {
	w := &czBits{}
	written := 0
	nblocks := 1 + r.Intn(4)
	for bi := 0; bi < nblocks; bi++ {
		final := bi == nblocks-1
		if final && r.Intn(25) == 0 {
			final = false // the stream never ends
		}
		fin := uint64(0)
		if final {
			fin = 1
		}
		w.bits(fin, 1)
		switch k := r.Intn(20); {
		case k < 6: // stored
			w.bits(0, 2)
			w.align()
			n := r.Intn(40)
			nlen := ^uint16(n)
			if r.Intn(12) == 0 {
				nlen ^= uint16(1 << r.Intn(16))
			}
			w.bits(uint64(n), 16)
			w.bits(uint64(nlen), 16)
			have := n
			if r.Intn(16) == 0 {
				have = r.Intn(n + 1)
			}
			for i := 0; i < have; i++ {
				w.bits(uint64('a'+r.Intn(26)), 8)
			}
			written += have
		case k < 17: // fixed Huffman
			w.bits(1, 2)
			for i, n := 0, r.Intn(30); i < n; i++ {
				switch r.Intn(4) {
				case 0, 1:
					w.fixedLit(r.Intn(256))
					written++
				case 2:
					if written == 0 && r.Intn(8) != 0 {
						w.fixedLit('x')
						written++
					}
					s := 257 + r.Intn(29)
					if r.Intn(50) == 0 {
						s = 286 + r.Intn(2)
					}
					w.fixedLit(s)
					if s > 285 {
						break
					}
					ext := czLenExtra[s-257]
					e := uint64(r.Intn(1 << ext))
					w.bits(e, ext)
					d := r.Intn(30)
					if r.Intn(12) != 0 && written > 0 {
						// a distance that reaches back into the output
						for d > 0 && czDistBase[d] > written {
							d--
						}
					}
					if r.Intn(60) == 0 {
						d = 30 + r.Intn(2)
					}
					w.code(uint64(d), 5)
					if d < 30 {
						w.bits(uint64(r.Intn(1<<czDistExtra[d])), czDistExtra[d])
					}
					written += czLenBase[s-257] + int(e)
				default:
					w.fixedLit('a' + r.Intn(3))
					written++
				}
			}
			if r.Intn(30) != 0 {
				w.fixedLit(256)
			}
		case k < 19: // dynamic: a random header and a little more
			w.bits(2, 2)
			for i, n := 0, 2+r.Intn(30); i < n; i++ {
				w.bits(uint64(r.Intn(256)), 8)
			}
		default: // reserved
			w.bits(3, 2)
		}
	}
	return w.bytes()
}

// czGoWrite is Go's stream for data at level.
func czGoWrite(format string, level int, data []byte) []byte {
	var b bytes.Buffer
	var w io.WriteCloser
	var err error
	switch format {
	case "gzip":
		w, err = gzip.NewWriterLevel(&b, level)
	case "deflate":
		w, err = flate.NewWriter(&b, level)
	default:
		w, err = zlib.NewWriterLevel(&b, level)
	}
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// czWrap puts raw DEFLATE in format's wrapper, sometimes wrong.
func czWrap(r *rand.Rand, format string, raw, data []byte) []byte {
	switch format {
	case "zlib":
		cmf, flg := byte(0x78), byte(0x9c)
		switch r.Intn(20) {
		case 0:
			flg++ // the header check fails
		case 1:
			flg = 0xbb // FDICT, header check still right
		case 2:
			cmf = 0x88 // a window over 32 KiB
			flg = byte(31 - (int(cmf)*256)%31)
		}
		out := append([]byte{cmf, flg}, raw...)
		sum := adler32.Checksum(data)
		if r.Intn(6) == 0 {
			sum ^= 1 << r.Intn(32)
		}
		out = binary.BigEndian.AppendUint32(out, sum)
		if r.Intn(8) == 0 {
			out = out[:len(out)-r.Intn(5)]
		}
		return out
	case "gzip":
		h := []byte{0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 255}
		switch r.Intn(14) {
		case 0:
			h[1] = 0x8c
		case 1:
			h[2] = 7
		case 2:
			h[3] = 0xe0 // reserved flags
		}
		flags := byte(r.Intn(16)) << 1
		if r.Intn(2) == 0 {
			flags = 0
		}
		h[3] |= flags
		if flags&4 != 0 {
			n := r.Intn(8)
			h = append(h, byte(n), 0)
			for i := 0; i < n; i++ {
				h = append(h, byte(r.Intn(256)))
			}
			if r.Intn(5) == 0 {
				h = h[:len(h)-r.Intn(n+1)]
			}
		}
		if flags&8 != 0 {
			h = append(h, "name.txt"[:r.Intn(9)]...)
			if r.Intn(6) != 0 {
				h = append(h, 0)
			}
		}
		if flags&16 != 0 {
			h = append(h, "a comment"[:r.Intn(10)]...)
			if r.Intn(6) != 0 {
				h = append(h, 0)
			}
		}
		if flags&2 != 0 {
			sum := uint16(crc32.ChecksumIEEE(h))
			if r.Intn(4) == 0 {
				sum ^= 1
			}
			h = binary.LittleEndian.AppendUint16(h, sum)
		}
		out := append(h, raw...)
		sum, size := crc32.ChecksumIEEE(data), uint32(len(data))
		switch r.Intn(10) {
		case 0:
			sum ^= 1 << r.Intn(32)
		case 1:
			size++
		}
		out = binary.LittleEndian.AppendUint32(out, sum)
		out = binary.LittleEndian.AppendUint32(out, size)
		switch r.Intn(10) {
		case 0:
			out = out[:len(out)-r.Intn(9)]
		case 1:
			out = append(out, czGoWrite("gzip", r.Intn(10), czData(r))...)
		case 2:
			out = append(out, byte(r.Intn(256)), byte(r.Intn(256)))
		}
		return out
	}
	return raw
}

// czGoRead is Go's answer for a stream.
func czGoRead(format string, s []byte) ([]byte, error) {
	var rd io.Reader
	switch format {
	case "gzip":
		z, err := gzip.NewReader(bytes.NewReader(s))
		if err != nil {
			return nil, err
		}
		rd = z
	case "deflate":
		rd = flate.NewReader(bytes.NewReader(s))
	default:
		z, err := zlib.NewReader(bytes.NewReader(s))
		if err != nil {
			return nil, err
		}
		rd = z
	}
	return io.ReadAll(rd)
}

// czCases builds the seeded cases.
func czCases(r *rand.Rand, n int) []czCase {
	cases := make([]czCase, 0, n)
	for len(cases) < n {
		c := czCase{format: czFormats[r.Intn(3)], level: -2 + r.Intn(12), chunk: 1 + r.Intn(9)}
		switch k := r.Intn(8); {
		case k < 2: // Go-written, maybe changed
			c.data = czData(r)
			c.stream = czGoWrite(c.format, c.level, c.data)
			switch r.Intn(3) {
			case 0:
				c.plain = true
			case 1:
				c.stream = c.stream[:r.Intn(len(c.stream))]
			default:
				i := r.Intn(len(c.stream))
				c.stream[i] ^= 1 << r.Intn(8)
			}
		case k < 3: // a Go-written dynamic block with header bits changed, rewrapped
			data := czData(r)
			raw := czGoWrite("deflate", 6, data)
			for i, m := 0, 1+r.Intn(3); i < m && len(raw) > 0; i++ {
				j := r.Intn(min(len(raw), 12))
				raw[j] ^= 1 << r.Intn(8)
			}
			c.stream = czWrap(r, c.format, raw, data)
		default: // hand-built blocks; the checksums cover what Go decodes
			raw := czRawBlocks(r)
			data, _ := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
			c.stream = czWrap(r, c.format, raw, data)
		}
		out, err := czGoRead(c.format, c.stream)
		if err != nil {
			c.err = err.Error()
		} else {
			c.ok, c.out = true, out
		}
		cases = append(cases, c)
	}
	return cases
}

const czRenderXo = `use std/compress

fn cz_format(f: Str) -> compress.Format {
  match f {
    "gzip" => compress.Format.Gzip
    "deflate" => compress.Format.Deflate
    _ => compress.Format.Zlib
  }
}

fn cz_unpack(f: Str, s: Bytes, max: Int) -> Str {
  let r = match f {
    "gzip" => compress.gunzip(s, max_size: max)
    "deflate" => compress.inflate(s, max_size: max)
    _ => compress.unzlib(s, max_size: max)
  }
  match r {
    Ok(b) => "ok " + b.to_hex()
    Err(e) => "err " + e.display()
  }
}

fn cz_stream_unpack(f: Str, s: Bytes, k: Int) -> Str {
  var d = compress.decompressor(cz_format(f))
  var out = BytesBuilder.new()
  var i = 0
  while i < s.len() {
    let j = if i + k < s.len() { i + k } else { s.len() }
    match d.write(s[i..j]) {
      Ok(b) => out.write(b)
      Err(e) => return "err " + e.display()
    }
    i = j
  }
  match d.finish() {
    Ok(b) => out.write(b)
    Err(e) => return "err " + e.display()
  }
  "ok " + out.freeze().to_hex()
}

fn cz_pack(f: Str, level: Int, data: Bytes) -> Bytes {
  match f {
    "gzip" => compress.gzip(data, level: compress.Level.Of(level))
    "deflate" => compress.deflate(data, level: compress.Level.Of(level))
    _ => compress.zlib(data, level: compress.Level.Of(level))
  }
}

fn cz_stream_pack(f: Str, level: Int, data: Bytes, k: Int) -> Bytes {
  var c = compress.compressor(cz_format(f), level: compress.Level.Of(level))
  var out = BytesBuilder.new()
  var i = 0
  while i < data.len() {
    let j = if i + k < data.len() { i + k } else { data.len() }
    out.write(c.write(data[i..j]))
    i = j
  }
  out.write(c.finish())
  out.freeze()
}

fn expect(what: Str, got: Str, want: Str) {
  if got != want {
    fault(what + ": got " + got)
  }
}

`

// czXoFile is the generated test file: the helpers and one test per case.
func czXoFile(seed int64, cases []czCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/compressfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// compress/flate, gzip and zlib; do not edit: XO_COMPRESSFUZZ_WRITE=1 go test ./internal/stdtest\n")
	b.WriteString("// -run TestCompressFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(czRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"compress fuzz %d\" {\n  let s = %s\n", i, ufBytes(c.stream))
		if !c.ok {
			fmt.Fprintf(&b, "  expect(\"one shot\", cz_unpack(%q, s, 1073741824), %s)\n", c.format, xoStr("err "+c.err))
			fmt.Fprintf(&b, "  expect(\"stream\", cz_stream_unpack(%q, s, %d).split(\" \")[0], \"err\")\n", c.format, c.chunk)
		} else {
			want := fmt.Sprintf("ok %x", c.out)
			fmt.Fprintf(&b, "  expect(\"one shot\", cz_unpack(%q, s, 1073741824), %s)\n", c.format, xoStr(want))
			fmt.Fprintf(&b, "  expect(\"stream\", cz_stream_unpack(%q, s, %d), %s)\n", c.format, c.chunk, xoStr(want))
			if len(c.out) > 0 {
				name := strings.Replace(c.format, "deflate", "flate", 1) // Go's package names
				fmt.Fprintf(&b, "  expect(\"max_size\", cz_unpack(%q, s, %d), %s)\n", c.format, len(c.out)-1, xoStr(fmt.Sprintf("err %s: output exceeds max_size (%d bytes)", name, len(c.out)-1)))
			}
		}
		if c.plain {
			hex := fmt.Sprintf("%x", c.stream)
			fmt.Fprintf(&b, "  expect(\"write\", cz_pack(%q, %d, %s).to_hex(), %s)\n", c.format, c.level, ufBytes(c.data), xoStr(hex))
			fmt.Fprintf(&b, "  expect(\"stream write\", cz_stream_pack(%q, %d, %s, %d).to_hex(), %s)\n", c.format, c.level, ufBytes(c.data), c.chunk, xoStr(hex))
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// czCoverage fails when the cases stop reaching the decoders' parts.
func czCoverage(t *testing.T, cases []czCase) {
	t.Helper()
	oks, plain := 0, 0
	msgs := map[string]int{}
	for _, c := range cases {
		if c.ok {
			oks++
		} else {
			msgs[c.err]++
		}
		if c.plain {
			plain++
		}
	}
	t.Logf("answers: %d ok, %d err (%d distinct messages), %d unchanged Go-written", oks, len(cases)-oks, len(msgs), plain)
	for m, n := range msgs {
		t.Logf("  %4d %s", n, m)
	}
	if oks < len(cases)/5 || len(cases)-oks < len(cases)/4 || len(msgs) < 12 || plain < len(cases)/20 {
		t.Fatalf("the cases stopped covering the decoders: %d ok, %d messages, %d plain", oks, len(msgs), plain)
	}
}

const czCorpus = "testdata/corpus/compressfuzz/compressfuzz_test.xo"

// TestCompressFuzzCorpusCurrent: the committed corpus is what seed 1
// writes.
func TestCompressFuzzCorpusCurrent(t *testing.T) {
	cases := czCases(rand.New(rand.NewSource(1)), 2000)
	czCoverage(t, cases)
	text := czXoFile(1, cases)
	if os.Getenv("XO_COMPRESSFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(czCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(czCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(czCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's compress packages or the generator changed): XO_COMPRESSFUZZ_WRITE=1 go test ./internal/stdtest -run TestCompressFuzzCorpusCurrent", czCorpus)
	}
}

// TestCompressFuzz runs a fresh seed's cases.
func TestCompressFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the compress fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_COMPRESSFUZZ_SEED", 2))
	n := envInt(t, "XO_COMPRESSFUZZ_N", 2000)
	cases := czCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d %s %x", i, c.format, c.stream)
	}
	runFuzzDir(t, "compressfuzz_test.xo", czXoFile(seed, cases), envBackends("XO_COMPRESSFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
