package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
)

// lcgData is the Xo test's data(seed, n, k): bytes 97 + v % k for k < 256,
// else v & 255, v the 31-bit LCG's next value >> 8.
func lcgData(seed, n, k int) []byte {
	x := seed & 0x7fffffff
	out := make([]byte, n)
	for i := range out {
		x = (x*1103515245 + 12345) & 0x7fffffff
		v := x >> 8
		if k < 256 {
			out[i] = byte(97 + v%k)
		} else {
			out[i] = byte(v & 255)
		}
	}
	return out
}

// chunks are the Xo test's chunks(seed, n): sizes 1 + v % 5000 from the
// same LCG, the last cut to n.
func chunks(seed, n int) []int {
	x := seed & 0x7fffffff
	var out []int
	for left := n; left > 0; {
		x = (x*1103515245 + 12345) & 0x7fffffff
		c := 1 + (x>>8)%5000
		if c > left {
			c = left
		}
		out = append(out, c)
		left -= c
	}
	return out
}

// streamWrite is a Compressor over Go's writer: the data in chunks, a
// Flush after every third chunk when flush, then Close.
func streamWrite(format string, data []byte, level, seed int, flush bool) []byte {
	var b bytes.Buffer
	var w io.WriteCloser
	switch format {
	case "gzip":
		w, _ = gzip.NewWriterLevel(&b, level)
	case "deflate":
		w, _ = flate.NewWriter(&b, level)
	default:
		w, _ = zlib.NewWriterLevel(&b, level)
	}
	at := 0
	for i, c := range chunks(seed, len(data)) {
		w.Write(data[at : at+c])
		at += c
		if flush && i%3 == 2 {
			w.(interface{ Flush() error }).Flush()
		}
	}
	w.Close()
	return b.Bytes()
}

func sha8(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:8]) }

// compressWrite is std/rt's: Go's writer at level, written once, closed.
func compressWrite(format string, data []byte, level int) []byte {
	var b bytes.Buffer
	var w io.WriteCloser
	switch format {
	case "gzip":
		w, _ = gzip.NewWriterLevel(&b, level)
	case "deflate":
		w, _ = flate.NewWriter(&b, level)
	default:
		w, _ = zlib.NewWriterLevel(&b, level)
	}
	w.Write(data)
	w.Close()
	return b.Bytes()
}

// compressRead is std/rt's decoder with max_size: ok LEN SHA or err TEXT.
func compressRead(format string, data []byte, max int64) string {
	var r io.Reader
	var err error
	name := map[string]string{"gzip": "gzip", "deflate": "flate", "zlib": "zlib"}[format]
	switch format {
	case "gzip":
		r, err = gzip.NewReader(bytes.NewReader(data))
	case "deflate":
		r = flate.NewReader(bytes.NewReader(data))
	default:
		r, err = zlib.NewReader(bytes.NewReader(data))
	}
	if err != nil {
		return "err " + err.Error()
	}
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return "err " + err.Error()
	}
	if int64(len(b)) > max {
		return "err " + name + ": output exceeds max_size (" + strconv.FormatInt(max, 10) + " bytes)"
	}
	return fmt.Sprintf("ok %d %s", len(b), sha8(b))
}

func compressCorpus() string {
	r := rand.New(rand.NewSource(2))
	formats := []string{"gzip", "deflate", "zlib"}
	var o strings.Builder
	o.WriteString(header("compress/gzip, compress/flate, and compress/zlib as std/rt/compress.go calls them, on generated data at every level and on Go-written, corrupted, and bounded streams"))
	o.WriteString("use std/compress\nuse std/crypto\n\n")
	o.WriteString("/// (format, level, seed, n, k, Go's output: length and sha256 prefix)\nconst WRITE: List[(Str, Int, Int, Int, Int, Str)] = [\n")
	nw := 0
	for _, n := range []int{0, 1, 5, 16, 17, 100, 127, 128, 1000, 4096, 32768, 65535, 65536, 70000, 140000} {
		for _, k := range []int{1, 3, 26, 256} {
			for lv := -2; lv <= 9; lv++ {
				f := formats[r.Intn(3)]
				seed := r.Intn(1 << 30)
				out := compressWrite(f, lcgData(seed, n, k), lv)
				fmt.Fprintf(&o, "  (%q, %d, %d, %d, %d, %q),\n", f, lv, seed, n, k, fmt.Sprintf("%d %s", len(out), sha8(out)))
				nw++
			}
		}
	}
	o.WriteString("]\n\n/// (format, level, seed, n, k, chunk seed, flush, Go's streamed output: length and sha256 prefix)\nconst STREAM: List[(Str, Int, Int, Int, Int, Int, Bool, Str)] = [\n")
	for i := 0; i < 160; i++ {
		f := formats[r.Intn(3)]
		lv := r.Intn(12) - 2
		seed, cs := r.Intn(1<<30), r.Intn(1<<30)
		n := []int{0, 7, 300, 5000, 40000, 100000}[r.Intn(6)]
		k := []int{1, 3, 26, 256}[r.Intn(4)]
		fl := r.Intn(2) == 0
		out := streamWrite(f, lcgData(seed, n, k), lv, cs, fl)
		fmt.Fprintf(&o, "  (%q, %d, %d, %d, %d, %d, %v, %q),\n", f, lv, seed, n, k, cs, fl, fmt.Sprintf("%d %s", len(out), sha8(out)))
	}
	o.WriteString("]\n\n/// (format, stream, max_size, Go's result: ok LEN SHA or err TEXT)\nconst READ: List[(Str, Bytes, Int, Str)] = [\n")
	nr := 0
	for i := 0; i < 400; i++ {
		f := formats[r.Intn(3)]
		data := lcgData(r.Intn(1<<30), r.Intn(400), []int{1, 4, 26, 256}[r.Intn(4)])
		s := compressWrite(f, data, r.Intn(12)-2)
		switch i % 7 {
		case 2:
			s = s[:r.Intn(len(s))]
		case 3:
			s = append([]byte{}, s...)
			s[r.Intn(len(s))] ^= byte(1 << r.Intn(8))
		case 4:
			s = append(append([]byte{}, s...), compressWrite(f, lcgData(i, 30, 3), 6)...)
		case 5:
			s = append(append([]byte{}, s...), []byte("junk")[:1+r.Intn(4)]...)
		}
		max := int64(1 << 30)
		if i%6 == 1 {
			max = int64(len(data)) - 1
		} else if i%6 == 3 {
			max = int64(len(data))
		}
		if max < 0 {
			max = 0
		}
		fmt.Fprintf(&o, "  (%q, %s, %d, %q),\n", f, xbytes(s), max, compressRead(f, s, max))
		nr++
	}
	// a bomb: 2 MB of zeros, refused past 64 KB
	bomb := compressWrite("gzip", make([]byte, 2<<20), 9)
	fmt.Fprintf(&o, "  (\"gzip\", %s, 65536, %q),\n", xbytes(bomb), compressRead("gzip", bomb, 65536))
	for _, s := range [][]byte{{}, {0x78}, {0x78, 0x9c}, {0x78, 0x9d}, {0x79, 0x9c}, {0x78, 0xbb, 0, 0, 0, 1}, {0x78, 0xbb, 0, 0, 0, 2}, {0x1f}, {0x1f, 0x8b}, {0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 255}} {
		for _, f := range formats {
			fmt.Fprintf(&o, "  (%q, %s, 1073741824, %q),\n", f, xbytes(s), compressRead(f, s, 1<<30))
		}
	}
	o.WriteString("]\n")
	o.WriteString(compressTests)
	return o.String()
}

const compressTests = `
fn chunks(seed: Int, n: Int) -> List[Int] {
  var x = seed & 0x7fffffff
  var out: List[Int] = []
  var left = n
  while left > 0 {
    x = (x * 1103515245 + 12345) & 0x7fffffff
    var c = 1 + (x >> 8) % 5000
    if c > left {
      c = left
    }
    out.push(c)
    left -= c
  }
  out
}

fn format_of(f: Str) -> compress.Format {
  match f {
    "gzip" => compress.Format.Gzip
    "deflate" => compress.Format.Deflate
    _ => compress.Format.Zlib
  }
}

test "a Compressor writes Go's bytes for the same writes and flushes" {
  var bad: List[(Str, Int, Int, Int, Str, Str)] = []
  for (f, lv, seed, n, k, cs, fl, want) in STREAM {
    let d = data(seed, n, k)
    var c = compress.compressor(format_of(f), level: level(lv))
    var out = BytesBuilder.new()
    var at = 0
    for (i, n2) in chunks(cs, n).indexed() {
      out.write(c.write(d[at..at + n2]))
      at += n2
      if fl && i % 3 == 2 {
        out.write(c.flush())
      }
    }
    out.write(c.finish())
    let got = out.freeze()
    let g = "${got.len()} ${sha8(got)}"
    if g != want {
      bad.push((f, lv, seed, n, g, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "a Decompressor in pieces gives the whole decoder's result" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (i, (f, s, max, want)) in READ.indexed() {
    if max != 1073741824 {
      continue
    }
    var d = compress.decompressor(format_of(f))
    var out = BytesBuilder.new()
    var err = ""
    var at = 0
    for n2 in chunks(i + 7, s.len()) {
      match d.write(s[at..at + n2]) {
        Ok(b) => out.write(b)
        Err(e) => {
          err = e.display()
          break
        }
      }
      at += n2
    }
    if err.is_empty() {
      match d.finish() {
        Ok(b) => out.write(b)
        Err(e) => err = e.display()
      }
    }
    let b = out.freeze()
    let got = if err.is_empty() { "ok ${b.len()} ${sha8(b)}" } else { "err ${err}" }
    if got != want {
      bad.push((f, i, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

fn data(seed: Int, n: Int, k: Int) -> Bytes {
  var x = seed & 0x7fffffff
  var b = BytesBuilder.with_capacity(n)
  var i = 0
  while i < n {
    x = (x * 1103515245 + 12345) & 0x7fffffff
    let v = x >> 8
    b.write_byte(UInt8(if k < 256 { 97 + v % k } else { v & 255 }))
    i += 1
  }
  b.freeze()
}

fn sha8(b: Bytes) -> Str { crypto.sha256(b)[0..8].to_hex() }

fn level(n: Int) -> compress.Level { compress.Level.Of(n) }

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "every level writes Go's bytes" {
  var bad: List[(Str, Int, Int, Int, Int, Str, Str)] = []
  for (f, lv, seed, n, k, want) in WRITE {
    let d = data(seed, n, k)
    let out = match f {
      "gzip" => compress.gzip(d, level(lv))
      "deflate" => compress.deflate(d, level(lv))
      _ => compress.zlib(d, level(lv))
    }
    let got = "${out.len()} ${sha8(out)}"
    if got != want {
      bad.push((f, lv, seed, n, k, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "the decoders give Go's output or error, bounded by max_size" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (i, (f, s, max, want)) in READ.indexed() {
    let r = match f {
      "gzip" => compress.gunzip(s, max_size: max)
      "deflate" => compress.inflate(s, max_size: max)
      _ => compress.unzlib(s, max_size: max)
    }
    let got = match r {
      Ok(b) => "ok ${b.len()} ${sha8(b)}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((f, i, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
