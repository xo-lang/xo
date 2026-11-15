package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"hash/crc32"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"time"
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
	dictCorpus(&o)
	gzipHeaderCorpus(&o)
	o.WriteString(compressTests)
	o.WriteString(compressDictTests)
	return o.String()
}

// dictWrite is std/rt's deflate or zlib with a preset dictionary (nil when
// empty), written in chunks of cs (one write when cs is 0), a Flush after
// every third chunk when flush.
func dictWrite(format string, data, dict []byte, level, cs int, flush bool) []byte {
	if len(dict) == 0 {
		dict = nil
	}
	var b bytes.Buffer
	var w io.WriteCloser
	if format == "deflate" {
		w, _ = flate.NewWriterDict(&b, level, dict)
	} else {
		w, _ = zlib.NewWriterLevelDict(&b, level, dict)
	}
	if cs == 0 {
		w.Write(data)
	} else {
		at := 0
		for i, c := range chunks(cs, len(data)) {
			w.Write(data[at : at+c])
			at += c
			if flush && i%3 == 2 {
				w.(interface{ Flush() error }).Flush()
			}
		}
	}
	w.Close()
	return b.Bytes()
}

// dictRead is std/rt's inflate or unzlib with a preset dictionary: ok LEN
// SHA or err TEXT.
func dictRead(format string, data, dict []byte) string {
	if len(dict) == 0 {
		dict = nil
	}
	var r io.Reader
	var err error
	if format == "deflate" {
		r = flate.NewReaderDict(bytes.NewReader(data), dict)
	} else {
		r, err = zlib.NewReaderDict(bytes.NewReader(data), dict)
	}
	if err != nil {
		return "err " + err.Error()
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return "err " + err.Error()
	}
	return fmt.Sprintf("ok %d %s", len(b), sha8(b))
}

// dictCorpus: preset dictionaries (flate.NewWriterDict and NewReaderDict,
// zlib.NewWriterLevelDict and NewReaderDict) on generated data, the
// dictionary from the data's own generator (a long match at the start),
// another seed with the same alphabet, or longer than the window.
func dictCorpus(o *strings.Builder) {
	r := rand.New(rand.NewSource(3))
	formats := []string{"deflate", "zlib"}
	o.WriteString("\n/// (format, level, seed, n, k, dict seed, dict n, chunk seed (0: one write), flush, Go's output: length and sha256 prefix)\nconst DICT_WRITE: List[(Str, Int, Int, Int, Int, Int, Int, Int, Bool, Str)] = [\n")
	type rd struct {
		f    string
		s    []byte
		ds   int
		dn   int
		k    int
		want string
	}
	var reads []rd
	for i := 0; i < 240; i++ {
		f := formats[r.Intn(2)]
		lv := r.Intn(12) - 2
		seed := r.Intn(1 << 30)
		n := []int{0, 1, 20, 300, 5000, 40000, 70000}[r.Intn(7)]
		k := []int{1, 3, 26, 256}[r.Intn(4)]
		dn := []int{0, 1, 4, 100, 4000, 32768, 40000}[r.Intn(7)]
		ds := seed
		if i%3 == 1 {
			ds = r.Intn(1 << 30)
		}
		cs, fl := 0, false
		if i%4 == 3 {
			cs, fl = r.Intn(1<<30), r.Intn(2) == 0
		}
		d := lcgData(seed, n, k)
		dict := lcgData(ds, dn, k)
		out := dictWrite(f, d, dict, lv, cs, fl)
		fmt.Fprintf(o, "  (%q, %d, %d, %d, %d, %d, %d, %d, %v, %q),\n", f, lv, seed, n, k, ds, dn, cs, fl, fmt.Sprintf("%d %s", len(out), sha8(out)))
		if n <= 300 {
			reads = append(reads, rd{f, out, ds, dn, k, ""})
		}
	}
	o.WriteString("]\n\n/// (format, stream, dict seed, dict n, k, Go's result with that dictionary: ok LEN SHA or err TEXT)\nconst DICT_READ: List[(Str, Bytes, Int, Int, Int, Str)] = [\n")
	for i, c := range reads {
		s, ds, dn := c.s, c.ds, c.dn
		switch i % 5 {
		case 1:
			// another dictionary
			ds = ds + 1
		case 2:
			// none
			dn = 0
		case 3:
			if len(s) > 1 {
				s = s[:r.Intn(len(s))]
			}
		}
		fmt.Fprintf(o, "  (%q, %s, %d, %d, %d, %q),\n", c.f, xbytes(s), ds, dn, c.k, dictRead(c.f, s, lcgData(ds, dn, c.k)))
	}
	// compress/zlib TestDecompressor dictionary case: "Hello, World!\n" against
	// the dictionary "Hello, World!\n" and against "Hell"
	hw := []byte("Hello, World!\n")
	z := dictWrite("zlib", hw, hw, 6, 0, false)
	fmt.Fprintf(o, "]\n\n/// zlib with the dictionary \"Hello, World!\\n\" (Go's zlib.NewWriterLevelDict at level 6)\nconst HELLO_Z: Bytes = %s\n", xbytes(z))
	fmt.Fprintf(o, "/// Go's results with that dictionary, with \"Hell\", and with none\nconst HELLO_READ: List[Str] = [%q, %q, %q]\n", dictRead("zlib", z, hw), dictRead("zlib", z, hw[:4]), dictRead("zlib", z, nil))
}

// ghWrite is std/rt's gzip_with: Go's gzip.Writer with its Header set.
func ghWrite(data []byte, name, comment string, extra []byte, mtime int64, os, level int) ([]byte, error) {
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, level)
	w.Name, w.Comment = name, comment
	if len(extra) > 0 {
		w.Extra = extra
	}
	if mtime > 0 {
		w.ModTime = time.Unix(mtime, 0)
	}
	w.OS = byte(os)
	_, err := w.Write(data)
	if err == nil {
		err = w.Close()
	}
	return b.Bytes(), err
}

// ghRead is std/rt's gzip_header: "ok NAME|COMMENT|EXTRA HEX|MTIME|OS" or
// "err TEXT".
func ghRead(s []byte) string {
	r, err := gzip.NewReader(bytes.NewReader(s))
	if err != nil {
		return "err " + err.Error()
	}
	mt := int64(0)
	if !r.ModTime.IsZero() {
		mt = r.ModTime.Unix()
	}
	return fmt.Sprintf("ok %s|%s|%x|%d|%d", r.Name, r.Comment, r.Extra, mt, r.OS)
}

// ghStr is a header string: Latin-1 runes (printable ASCII and U+00A0 to
// U+00FF), sometimes with a rune gzip.Writer refuses (U+0000, U+0100,
// U+20AC).
func ghStr(r *rand.Rand) string {
	if r.Intn(3) == 0 {
		return ""
	}
	var b strings.Builder
	n := 1 + r.Intn(12)
	for i := 0; i < n; i++ {
		if r.Intn(3) == 0 {
			b.WriteRune(rune(0xa0 + r.Intn(0x60)))
		} else {
			b.WriteRune(rune(0x20 + r.Intn(0x5f)))
		}
	}
	if r.Intn(10) == 0 {
		b.WriteRune([]rune{0, 0x100, 0x20ac}[r.Intn(3)])
	}
	return b.String()
}

// gzipHeaderCorpus: gzip.Writer's Header fields written at random levels
// and read back by gzip.NewReader, and the reader on headers cut short,
// with FHCRC, and with a bad header CRC.
func gzipHeaderCorpus(o *strings.Builder) {
	r := rand.New(rand.NewSource(4))
	o.WriteString("\n/// (name, comment, extra, mtime, os, level, data seed, n, Go's output (length and sha256 prefix, or err TEXT), Go's header read back (\"\" when the write failed))\nconst GZH_WRITE: List[(Str, Str, Bytes, Int, Int, Int, Int, Int, Str, Str)] = [\n")
	var reads [][]byte
	for i := 0; i < 120; i++ {
		name, comment := ghStr(r), ghStr(r)
		var extra []byte
		switch r.Intn(5) {
		case 1:
			extra = lcgData(i, 1+r.Intn(40), 256)
		case 2:
			extra = lcgData(i, []int{65535, 65536}[r.Intn(2)], 3)
		}
		mtime := []int64{0, -5, 1, 1700000000, 4294967295, 4294967296 + 7}[r.Intn(6)]
		os := []int{255, 0, 3, 11}[r.Intn(4)]
		lv := r.Intn(12) - 2
		seed, n := r.Intn(1<<30), r.Intn(300)
		out, err := ghWrite(lcgData(seed, n, 26), name, comment, extra, mtime, os, lv)
		res, back := "", ""
		if err != nil {
			res = "err " + err.Error()
		} else {
			res = fmt.Sprintf("%d %s", len(out), sha8(out))
			back = ghRead(out)
			if len(extra) < 1000 {
				reads = append(reads, out)
			}
		}
		fmt.Fprintf(o, "  (%s, %s, %s, %d, %d, %d, %d, %d, %q, %s),\n", xq(name), xq(comment), xbytes(extra), mtime, os, lv, seed, n, res, xq(back))
	}
	o.WriteString("]\n\n/// (stream, Go's header: ok NAME|COMMENT|EXTRA HEX|MTIME|OS or err TEXT)\nconst GZH_READ: List[(Bytes, Str)] = [\n")
	for i, s := range reads {
		if i%3 == 0 {
			s = s[:r.Intn(len(s))]
		}
		fmt.Fprintf(o, "  (%s, %s),\n", xbytes(s), xq(ghRead(s)))
	}
	// FHCRC: the header's CRC-32 low 16 bits after it, right and wrong
	hd := []byte{0x1f, 0x8b, 8, 2 | 8, 1, 2, 3, 4, 0, 3, 'n', 0}
	c := crc32.ChecksumIEEE(hd)
	body := compressWrite("deflate", []byte("hcrc"), 6)
	for _, d := range []uint32{c, c ^ 1} {
		s := append(append(append([]byte{}, hd...), byte(d), byte(d>>8)), body...)
		fmt.Fprintf(o, "  (%s, %s),\n", xbytes(s), xq(ghRead(s)))
	}
	for _, s := range [][]byte{{}, {0x1f}, {0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0}, {0x1f, 0x8b, 7, 0, 0, 0, 0, 0, 0, 255}, {0x1f, 0x8b, 8, 4, 0, 0, 0, 0, 0, 255, 5, 0, 1}, {0x1f, 0x8b, 8, 8, 0, 0, 0, 0, 0, 255, 'a', 'b'}} {
		fmt.Fprintf(o, "  (%s, %s),\n", xbytes(s), xq(ghRead(s)))
	}
	o.WriteString("]\n")
}

const compressDictTests = `
fn dict_out(f: Str, d: Bytes, dict: Bytes, lv: Int, cs: Int, fl: Bool) -> Bytes {
  if cs == 0 {
    return if f == "deflate" { compress.deflate(d, level(lv), dict: dict) } else { compress.zlib(d, level(lv), dict: dict) }
  }
  var c = compress.compressor(format_of(f), level: level(lv), dict: dict)
  var out = BytesBuilder.new()
  var at = 0
  for (i, n2) in chunks(cs, d.len()).indexed() {
    out.write(c.write(d[at..at + n2]))
    at += n2
    if fl && i % 3 == 2 {
      out.write(c.flush())
    }
  }
  out.write(c.finish())
  out.freeze()
}

test "a preset dictionary writes Go's bytes (flate.NewWriterDict, zlib.NewWriterLevelDict)" {
  var bad: List[(Str, Int, Int, Int, Int, Str, Str)] = []
  for (f, lv, seed, n, k, ds, dn, cs, fl, want) in DICT_WRITE {
    let out = dict_out(f, data(seed, n, k), data(ds, dn, k), lv, cs, fl)
    let got = "${out.len()} ${sha8(out)}"
    if got != want {
      bad.push((f, lv, seed, n, dn, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "a preset dictionary reads as Go's (flate.NewReaderDict, zlib.NewReaderDict), whole and in pieces" {
  var bad: List[(Str, Int, Str, Str)] = []
  for (i, (f, s, ds, dn, k, want)) in DICT_READ.indexed() {
    let dict = data(ds, dn, k)
    let r = if f == "deflate" { compress.inflate(s, dict: dict) } else { compress.unzlib(s, dict: dict) }
    let got = match r {
      Ok(b) => "ok ${b.len()} ${sha8(b)}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((f, i, got, want))
    }
    var d = compress.decompressor(format_of(f), dict: dict)
    var out = BytesBuilder.new()
    var err = ""
    var at = 0
    for n2 in chunks(i + 11, s.len()) {
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
    let got2 = if err.is_empty() { "ok ${b.len()} ${sha8(b)}" } else { "err ${err}" }
    if got2 != want {
      bad.push((f, i + 100000, got2, want))
    }
  }
  expect(first_bad(bad)) == []
}

fn header_str(h: compress.GzipHeader) -> Str { "ok ${h.name}|${h.comment}|${h.extra.to_hex()}|${h.mtime}|${h.os}" }

test "gzip_with writes Go's bytes for gzip.Header's fields, and gzip_header reads them back" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (name, comment, extra, mtime, os, lv, seed, n, want, back)) in GZH_WRITE.indexed() {
    let h = compress.GzipHeader{name: name, comment: comment, extra: extra, mtime: mtime, os: os}
    let (got, gback) = match compress.gzip_with(data(seed, n, 26), h, level: level(lv)) {
      Ok(b) => ("${b.len()} ${sha8(b)}", match compress.gzip_header(b) {
        Ok(h2) => header_str(h2)
        Err(e) => "err ${e.display()}"
      })
      Err(e) => ("err ${e.display()}", "")
    }
    if got != want || gback != back {
      bad.push((i, "${got} ${gback}", "${want} ${back}"))
    }
  }
  expect(first_bad(bad)) == []
}

test "gzip_header reads Go's header or gives its error" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (s, want)) in GZH_READ.indexed() {
    let got = match compress.gzip_header(s) {
      Ok(h) => header_str(h)
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "gzip_with an empty header is gzip" {
  let d = data(5, 1000, 26)
  for lv in [-2, 0, 1, 6, 9] {
    expect(compress.gzip_with(d, compress.GzipHeader{}, level: level(lv)).ok()) == Some(compress.gzip(d, level(lv)))
  }
}

test "the zlib dictionary case: the right dictionary, another (zlib: invalid dictionary), none" {
  let hw = b"Hello, World!\n"
  expect(compress.zlib(hw, dict: hw)) == HELLO_Z
  var got: List[Str] = []
  for dict in [hw, hw[0..4], b""] {
    got.push(match compress.unzlib(HELLO_Z, dict: dict) {
      Ok(b) => "ok ${b.len()} ${sha8(b)}"
      Err(e) => "err ${e.display()}"
    })
  }
  expect(got) == HELLO_READ
}
`

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
