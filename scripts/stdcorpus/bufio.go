package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"unicode/utf8"
)

// chunkReader returns data in the scripted chunk sizes, as a network
// connection does.
type chunkReader struct {
	data   []byte
	sizes  []int
	k      int
	offset int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.offset >= len(c.data) {
		return 0, io.EOF
	}
	n := c.sizes[c.k%len(c.sizes)]
	c.k++
	if n > len(p) {
		n = len(p)
	}
	if n > len(c.data)-c.offset {
		n = len(c.data) - c.offset
	}
	copy(p, c.data[c.offset:c.offset+n])
	c.offset += n
	return n, nil
}

// lossyUTF8 is s with each invalid byte U+FFFD, as Go ranges over it.
func lossyUTF8(b []byte) string {
	var o strings.Builder
	for _, r := range string(b) {
		o.WriteRune(r)
	}
	return o.String()
}

// The std/bufio corpus: Go's bufio.Reader (ReadBytes, Peek, Read) and
// io.ReadFull on scripted data, read through chunks of scripted sizes, the
// operations' results written as the expected results of the same
// operations on a std/bufio Reader over a stream that returns the same
// chunks.
func bufioCorpus() string {
	rg := rand.New(rand.NewSource(55))
	var o strings.Builder
	o.WriteString(header("Go's bufio.Reader (ReadBytes, Peek, Read) and io.ReadFull on scripted data and chunk sizes, against std/bufio's Reader over a stream that returns those chunks"))
	o.WriteString("use std/bufio\n\n")
	alphabet := []byte("ab,\n\r x\xff\xc3\xa9")
	type op struct {
		kind string // line, until, exact, peek, read
		n    int
	}
	o.WriteString("/// (data in hex, chunk sizes, operations as kind and argument, Go's results: one text each)\nconst CASES: List[(Str, List[Int], List[(Str, Int)], List[Str])] = [\n")
	for i := 0; i < 700; i++ {
		n := rg.Intn(60)
		data := make([]byte, n)
		for k := range data {
			data[k] = alphabet[rg.Intn(len(alphabet))]
		}
		sizes := make([]int, 1+rg.Intn(4))
		for k := range sizes {
			sizes[k] = 1 + rg.Intn(9)
		}
		if i%7 == 0 {
			sizes = []int{1}
		}
		var ops []op
		for k := 0; k < 4+rg.Intn(14); k++ {
			switch rg.Intn(5) {
			case 0:
				ops = append(ops, op{"line", 0})
			case 1:
				ops = append(ops, op{"until", []int{',', '\n', 'x', ' '}[rg.Intn(4)]})
			case 2:
				ops = append(ops, op{"exact", rg.Intn(14)})
			case 3:
				ops = append(ops, op{"peek", rg.Intn(14)})
			default:
				ops = append(ops, op{"read", 1 + rg.Intn(12)})
			}
		}
		br := bufio.NewReaderSize(&chunkReader{data: data, sizes: sizes}, 4096)
		var res []string
		for _, p := range ops {
			switch p.kind {
			case "line":
				b, err := br.ReadBytes('\n')
				if err != nil && len(b) == 0 {
					res = append(res, "none")
					break
				}
				m := len(b)
				if m > 0 && b[m-1] == '\n' {
					m--
					if m > 0 && b[m-1] == '\r' {
						m--
					}
				}
				res = append(res, "s:"+hex.EncodeToString([]byte(lossyUTF8(b[:m]))))
			case "until":
				b, err := br.ReadBytes(byte(p.n))
				if err != nil && len(b) == 0 {
					res = append(res, "none")
					break
				}
				res = append(res, "b:"+hex.EncodeToString(b))
			case "exact":
				buf := make([]byte, p.n)
				if _, err := io.ReadFull(br, buf); err != nil {
					res = append(res, "eof")
					break
				}
				res = append(res, "b:"+hex.EncodeToString(buf))
			case "peek":
				b, _ := br.Peek(p.n)
				res = append(res, "b:"+hex.EncodeToString(b))
			default:
				buf := make([]byte, p.n)
				m, err := br.Read(buf)
				if m == 0 && err != nil {
					res = append(res, "none")
					break
				}
				res = append(res, "b:"+hex.EncodeToString(buf[:m]))
			}
		}
		var ss, os, rs []string
		for _, s := range sizes {
			ss = append(ss, fmt.Sprint(s))
		}
		for _, p := range ops {
			os = append(os, fmt.Sprintf("(%q, %d)", p.kind, p.n))
		}
		for _, r := range res {
			rs = append(rs, fmt.Sprintf("%q", r))
		}
		o.WriteString(fmt.Sprintf("  (%q, [%s], [%s], [%s]),\n", hex.EncodeToString(data), strings.Join(ss, ", "), strings.Join(os, ", "), strings.Join(rs, ", ")))
	}
	o.WriteString("]\n\n")
	_ = utf8.RuneError
	o.WriteString(bufioTests)
	return o.String()
}

const bufioTests = `/// What a test reads from: data in scripted chunks, optionally an error
/// after some chunks.
struct Chunks {
  data: Bytes
  sizes: List[Int]
  pos: Int = 0
  k: Int = 0
  fail_at: Int = -1
}

struct Fake {
  st: Mutex[Chunks]
}

fn fake(data: Bytes, sizes: List[Int], fail_at: Int = -1) -> Fake {
  Fake{st: Mutex[Chunks].new(Chunks{data: data, sizes: sizes, fail_at: fail_at})}
}

fn (f: Fake) read(max: Int) -> Bytes? ! NetErr uses net {
  f.st.with(fn(var s: Chunks) -> Bytes? ! NetErr {
    if s.k == s.fail_at {
      return Err(NetErr.Timeout)
    }
    if s.pos >= s.data.len() {
      return Ok(None)
    }
    var n = s.sizes[s.k % s.sizes.len()]
    s.k += 1
    if n > max {
      n = max
    }
    if n > s.data.len() - s.pos {
      n = s.data.len() - s.pos
    }
    let out = s.data[s.pos..s.pos + n]
    s.pos += n
    Ok(Some(out))
  })
}

fn (f: Fake) write(data: Bytes) -> () ! NetErr uses net { Ok(()) }

fn (f: Fake) close() {}

fn hex_of(b: Bytes) -> Str {
  var s = ""
  let digits = "0123456789abcdef"
  for c in b {
    let v = Int(c)
    s = s + digits[v >> 4..(v >> 4) + 1] + digits[v & 15..(v & 15) + 1]
  }
  s
}

fn run_ops(var r: bufio.Reader, ops: List[(Str, Int)]) -> List[Str] uses net {
  var out: List[Str] = []
  for (kind, n) in ops {
    match kind {
      "line" => {
        match r.read_line() {
          Ok(Some(s)) => out.push("s:" + hex_of(s.bytes()))
          Ok(None) => out.push("none")
          Err(e) => out.push("err " + e.display())
        }
      }
      "until" => {
        match r.read_until(UInt8(n)) {
          Ok(Some(b)) => out.push("b:" + hex_of(b))
          Ok(None) => out.push("none")
          Err(e) => out.push("err " + e.display())
        }
      }
      "exact" => {
        match r.read_exact(n) {
          Ok(b) => out.push("b:" + hex_of(b))
          Err(_) => out.push("eof")
        }
      }
      "peek" => {
        match r.peek(n) {
          Ok(b) => out.push("b:" + hex_of(b))
          Err(e) => out.push("err " + e.display())
        }
      }
      _ => {
        match r.read(n) {
          Ok(Some(b)) => out.push("b:" + hex_of(b))
          Ok(None) => out.push("none")
          Err(e) => out.push("err " + e.display())
        }
      }
    }
  }
  out
}

test "a reader answers as Go's bufio.Reader does on every chunking" {
  var bad: List[(Str, List[Str], List[Str])] = []
  for (h, sizes, ops, want) in CASES {
    let data = Bytes.from_hex(h) ?? b""
    var r = bufio.reader(fake(data, sizes))
    let got = run_ops(r, ops)
    if got != want {
      bad.push((h, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 5 { bad.len() } else { 5 })]) == []
}

test "a record longer than the limit is TooLong, and the current chunk stays buffered" {
  var r = bufio.reader(fake(b"abcdefghij\nrest\n", [3]))
  match r.read_line(limit: 5) {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.display()) == "bufio: record too long"
  }
  expect(r.read_line(limit: 100).ok()) == Some(Some("defghij"))
  expect(r.read_line().ok()) == Some(Some("rest"))
  expect(r.read_line().ok()) == Some(None)
  // exactly the limit with its delimiter is fine
  var r2 = bufio.reader(fake(b"abcd\nxy", [2]))
  expect(r2.read_until('\n', limit: 5).ok()) == Some(Some(b"abcd\n"))
  expect(r2.read_until('\n', limit: 3).ok()) == Some(Some(b"xy"))
}

test "read_exact fails with UnexpectedEof when the stream ends first" {
  var r = bufio.reader(fake(b"abc", [2]))
  match r.read_exact(5) {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.display()) == "bufio: unexpected EOF"
  }
}

test "a failure of the stream is Net, with the stream's text" {
  var r = bufio.reader(fake(b"abcdef\n", [2], fail_at: 1))
  match r.read_line() {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.display()) == "i/o timeout"
  }
  // the reader goes on once the stream does (the fake fails once at chunk 1)
  var r2 = bufio.reader(fake(b"xyz", [3], fail_at: 0))
  match r2.read(10) {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.display()) == "i/o timeout"
  }
}

test "buffered counts the bytes read ahead, and size is at least 16" {
  var r = bufio.reader(fake(b"hello world\nmore", [20]), size: 1)
  expect(r.buffered()) == 0
  expect(r.read_line().ok()) == Some(Some("hello world"))
  expect(r.buffered()) == 4
  expect(r.read_exact(4).ok()) == Some(b"more")
  expect(r.buffered()) == 0
}

test "read, peek and read_exact of nothing, and a long line over many tiny chunks" {
  var r = bufio.reader(fake(b"abc", [1]))
  expect(r.read(0).ok()) == Some(Some(b""))
  expect(r.peek(0).ok()) == Some(b"")
  expect(r.read_exact(0).ok()) == Some(b"")
  expect(r.peek(2).ok()) == Some(b"ab")
  expect(r.read(5).ok()) == Some(Some(b"ab"))
  var big = BytesBuilder.new()
  for i in 0..20000 {
    big.write_byte(UInt8(97 + i % 26))
  }
  big.write_byte('\n')
  let long = big.freeze()
  var r2 = bufio.reader(fake(long, [7, 1, 13]))
  expect(r2.read_until('\n').ok()) == Some(Some(long))
  expect(r2.read_until('\n').ok()) == Some(None)
}
`
