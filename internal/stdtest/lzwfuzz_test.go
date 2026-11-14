// A fuzz harness for std/compress's LZW (lzw_encode, lzw_decode) against
// Go's compress/lzw (2026-11-14, decision 0616: a Proven module's parsers
// of untrusted input are fuzzed; the gzip, flate and zlib harness is
// compressfuzz_test.go).
//
// Each case picks a bit order and a literal width (2 to 8, sometimes 0, 1,
// 9 or 12 to reach Go's range errors) and data: random bytes that fit the
// width, long runs and short periods (to fill the 12 bit code table and
// reach Go's table reset), or bytes too large for the width. The encoder
// must give Go's bytes or Go's error text. The decoder then reads either
// Go's stream unchanged, cut short, with a bit flipped, with garbage after
// the end code, or random bytes, and must give Go's bytes (what Go's reader
// returns up to the end code) or Go's error text; a stream that decodes is
// also decoded with max_size one byte short.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/lzwfuzz/lzwfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestLzwFuzzCorpusCurrent checks it is what this
// generator writes (XO_LZWFUZZ_WRITE=1 rewrites it). TestLzwFuzz runs a
// fresh seed (XO_LZWFUZZ_SEED, default 2; XO_LZWFUZZ_N cases, default
// 2000) on XO_LZWFUZZ_BACKENDS (default arm64,llvm).
package stdtest

import (
	"bytes"
	"compress/lzw"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type lzCase struct {
	msb     bool
	width   int
	data    []byte
	encOK   bool
	enc     []byte // Go's encoding when encOK
	encErr  string
	stream  []byte // what the decoder reads
	decOK   bool
	dec     []byte
	decErr  string
	changed string // how stream differs from enc
}

func lzData(r *rand.Rand, width, maxLen int) []byte {
	n := r.Intn(maxLen)
	if r.Intn(4) == 0 {
		n = r.Intn(40)
	}
	lim := 256
	if width >= 2 && width < 8 {
		lim = 1 << width
	}
	b := make([]byte, 0, n)
	switch r.Intn(4) {
	case 0: // random
		for len(b) < n {
			b = append(b, byte(r.Intn(lim)))
		}
	case 1: // runs
		for len(b) < n {
			c := byte(r.Intn(lim))
			for k := r.Intn(300); k > 0 && len(b) < n; k-- {
				b = append(b, c)
			}
		}
	case 2: // short periods
		p := make([]byte, 1+r.Intn(7))
		for i := range p {
			p[i] = byte(r.Intn(lim))
		}
		for len(b) < n {
			b = append(b, p[len(b)%len(p)])
		}
	default: // a few distinct symbols, random order
		k := 1 + r.Intn(4)
		for len(b) < n {
			b = append(b, byte(r.Intn(k)%lim))
		}
	}
	if r.Intn(12) == 0 && width >= 2 && width < 8 && len(b) > 0 { // a byte too large
		b[r.Intn(len(b))] = byte(lim + r.Intn(256-lim))
	}
	return b
}

func lzOrder(msb bool) lzw.Order {
	if msb {
		return lzw.MSB
	}
	return lzw.LSB
}

func lzEncode(d []byte, msb bool, width int) ([]byte, error) {
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, lzOrder(msb), width)
	_, err := w.Write(d)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return buf.Bytes(), err
}

func lzDecode(s []byte, msb bool, width int) ([]byte, error) {
	r := lzw.NewReader(bytes.NewReader(s), lzOrder(msb), width)
	defer r.Close()
	return io.ReadAll(r)
}

// lzCases makes n cases with data up to maxLen bytes (9000 reaches the
// table reset; the committed corpus keeps to 1500, the lzw corpus has resets).
func lzCases(r *rand.Rand, n, maxLen int) []lzCase {
	var cases []lzCase
	for len(cases) < n {
		var c lzCase
		c.msb = r.Intn(2) == 0
		c.width = 2 + r.Intn(7)
		if r.Intn(25) == 0 {
			c.width = []int{0, 1, 9, 12}[r.Intn(4)]
		}
		c.data = lzData(r, c.width, maxLen)
		enc, err := lzEncode(c.data, c.msb, c.width)
		if err != nil {
			c.encErr = err.Error()
		} else {
			c.encOK, c.enc = true, enc
		}
		s := append([]byte{}, enc...)
		switch k := r.Intn(6); {
		case !c.encOK || k == 0:
			s = make([]byte, r.Intn(200))
			r.Read(s)
			c.changed = "random"
		case k == 1 && len(s) > 0:
			s = s[:r.Intn(len(s))]
			c.changed = "cut"
		case k == 2 && len(s) > 0:
			s[r.Intn(len(s))] ^= 1 << r.Intn(8)
			c.changed = "bit"
		case k == 3:
			g := make([]byte, 1+r.Intn(20))
			r.Read(g)
			s = append(s, g...)
			c.changed = "after"
		default:
			c.changed = "none"
		}
		c.stream = s
		dec, err := lzDecode(s, c.msb, c.width)
		if err != nil {
			c.decErr = err.Error()
		} else {
			c.decOK, c.dec = true, dec
		}
		cases = append(cases, c)
	}
	return cases
}

const lzRenderXo = `use std/compress

fn lz_order(msb: Bool) -> compress.LzwOrder {
  if msb { compress.LzwOrder.MSB } else { compress.LzwOrder.LSB }
}

fn lz_enc(data: Bytes, msb: Bool, width: Int) -> Str {
  match compress.lzw_encode(data, lz_order(msb), width) {
    Ok(b) => "ok " + b.to_hex()
    Err(e) => "err " + e.display()
  }
}

fn lz_dec(s: Bytes, msb: Bool, width: Int, max: Int) -> Str {
  match compress.lzw_decode(s, lz_order(msb), width, max_size: max) {
    Ok(b) => "ok " + b.to_hex()
    Err(e) => "err " + e.display()
  }
}

fn expect(what: Str, got: Str, want: Str) {
  if got != want {
    fault(what + ": got " + got)
  }
}

`

// lzXoFile is the generated test file: the helpers and one test per case.
func lzXoFile(seed int64, cases []lzCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/lzwfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// compress/lzw; do not edit: XO_LZWFUZZ_WRITE=1 go test ./internal/stdtest\n")
	b.WriteString("// -run TestLzwFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(lzRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"lzw fuzz %d\" {\n", i)
		want := "err " + c.encErr
		if c.encOK {
			want = fmt.Sprintf("ok %x", c.enc)
		}
		fmt.Fprintf(&b, "  expect(\"encode\", lz_enc(%s, %v, %d), %s)\n", ufBytes(c.data), c.msb, c.width, xoStr(want))
		want = "err " + c.decErr
		if c.decOK {
			want = fmt.Sprintf("ok %x", c.dec)
		}
		fmt.Fprintf(&b, "  let s = %s // %s\n", ufBytes(c.stream), c.changed)
		fmt.Fprintf(&b, "  expect(\"decode\", lz_dec(s, %v, %d, 1073741824), %s)\n", c.msb, c.width, xoStr(want))
		if c.decOK && len(c.dec) > 0 {
			fmt.Fprintf(&b, "  expect(\"max_size\", lz_dec(s, %v, %d, %d), %s)\n", c.msb, c.width, len(c.dec)-1, xoStr(fmt.Sprintf("err lzw: output exceeds max_size (%d bytes)", len(c.dec)-1)))
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// lzCoverage fails when the cases stop reaching the coder's parts.
func lzCoverage(t *testing.T, cases []lzCase, wantLong bool) {
	t.Helper()
	encErr, decOK, big := 0, 0, 0
	msgs := map[string]int{}
	for _, c := range cases {
		if !c.encOK {
			encErr++
			msgs["enc: "+c.encErr]++
		}
		if c.decOK {
			decOK++
		} else {
			msgs["dec: "+c.decErr]++
		}
		if len(c.data) > 5000 && c.encOK {
			big++ // past 4096 codes for most data: the table resets
		}
	}
	t.Logf("%d cases: %d encode errors, %d decodes ok, %d long inputs, %d distinct errors", len(cases), encErr, decOK, big, len(msgs))
	for m, n := range msgs {
		t.Logf("  %4d %s", n, m)
	}
	if encErr == 0 || decOK < len(cases)/4 || len(cases)-decOK < len(cases)/5 || (wantLong && big < len(cases)/10) || len(msgs) < 5 {
		t.Fatalf("the cases stopped covering the coder: %d encode errors, %d decodes ok, %d long, %d messages", encErr, decOK, big, len(msgs))
	}
}

const lzCorpus = "testdata/corpus/lzwfuzz/lzwfuzz_test.xo"

// TestLzwFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestLzwFuzzCorpusCurrent(t *testing.T) {
	cases := lzCases(rand.New(rand.NewSource(1)), 300, 1500)
	lzCoverage(t, cases, false)
	text := lzXoFile(1, cases)
	if os.Getenv("XO_LZWFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(lzCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lzCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(lzCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's compress/lzw or the generator changed): XO_LZWFUZZ_WRITE=1 go test ./internal/stdtest -run TestLzwFuzzCorpusCurrent", lzCorpus)
	}
}

// TestLzwFuzz runs a fresh seed's cases.
func TestLzwFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the lzw fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_LZWFUZZ_SEED", 2))
	n := envInt(t, "XO_LZWFUZZ_N", 2000)
	cases := lzCases(rand.New(rand.NewSource(seed)), n, 9000)
	lzCoverage(t, cases, true)
	runFuzzDir(t, "lzwfuzz_test.xo", lzXoFile(seed, cases), envBackends("XO_LZWFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
