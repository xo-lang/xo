// A fuzz harness for std/bufio against Go's bufio.Reader (decision 0616's
// fuzz kind). Each case is data (0 to 60 bytes from a small alphabet of
// delimiters, \r, invalid UTF-8 and é, or a tenth of the time up to 9,000
// bytes, past the 4,096 byte buffer), the chunk sizes a stream returns it
// in (1 to 9 bytes, or up to 4,096 for the long data), and 4 to 40
// operations: read_line, read_until a delimiter, read_exact, peek and read
// of 0 to 14 bytes (up to 5,000 for the long data). std/bufio's Reader over
// a stream returning those chunks must give what Go's bufio.Reader
// (ReadBytes, Peek, Read) and io.ReadFull give over a reader returning the
// same chunks (scripts/stdcorpus/bufio.go's model, which the bufio corpus
// uses), whose buffer is larger than any chunk: std/bufio buffers whole
// chunks and gathers spans (its doc), where Go's fixed buffer cuts a chunk
// to the space it has left, so the size of one read differs past 4,096
// buffered bytes by design.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/bufiofuzz/bufiofuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestBufioFuzzCorpusCurrent checks it is what this
// generator writes (XO_BUFIOFUZZ_WRITE=1 rewrites it). TestBufioFuzz runs a
// fresh seed (XO_BUFIOFUZZ_SEED, default 2; XO_BUFIOFUZZ_N cases, default
// 2000) on XO_BUFIOFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

type bfChunks struct {
	data   []byte
	sizes  []int
	k, off int
}

func (c *bfChunks) Read(p []byte) (int, error) {
	if c.off >= len(c.data) {
		return 0, io.EOF
	}
	n := c.sizes[c.k%len(c.sizes)]
	c.k++
	n = min(n, len(p), len(c.data)-c.off)
	copy(p, c.data[c.off:c.off+n])
	c.off += n
	return n, nil
}

// bfLossy is b with each invalid UTF-8 byte as U+FFFD, as Bytes.to_str_lossy.
func bfLossy(b []byte) string {
	var o strings.Builder
	for _, r := range string(b) {
		o.WriteRune(r)
	}
	return o.String()
}

type bfCase struct {
	data           []byte
	sizes, ops, rs string
}

func bfCases(r *rand.Rand, n int) []bfCase {
	alphabet := []byte("ab,\n\r x\xff\xc3\xa9")
	cs := make([]bfCase, 0, n)
	for len(cs) < n {
		long := r.Intn(10) == 0
		size := r.Intn(61)
		if long {
			size = r.Intn(9001)
		}
		data := make([]byte, size)
		for k := range data {
			data[k] = alphabet[r.Intn(len(alphabet))]
		}
		if size%13 == 7 {
			// the hostile dictionary (task #352): UTF-8 cut inside a
			// rune at the front, chosen by the size so the cases after
			// it stay as they were
			cuts := hostile.CutUTF8("é日😀")
			copy(data, cuts[size%len(cuts)])
		}
		sizes := make([]int, 1+r.Intn(4))
		for k := range sizes {
			sizes[k] = 1 + r.Intn(9)
			if long && r.Intn(2) == 0 {
				sizes[k] = 1 + r.Intn(4096)
			}
		}
		if r.Intn(7) == 0 {
			sizes = []int{1}
		}
		maxN := 14
		if long {
			maxN = 5000
		}
		type op struct {
			kind string
			n    int
		}
		var ops []op
		for k := 4 + r.Intn(37); k > 0; k-- {
			switch r.Intn(5) {
			case 0:
				ops = append(ops, op{"line", 0})
			case 1:
				ops = append(ops, op{"until", []int{',', '\n', 'x', ' ', 0xff}[r.Intn(5)]})
			case 2:
				ops = append(ops, op{"exact", r.Intn(maxN + 1)})
			case 3:
				ops = append(ops, op{"peek", r.Intn(min(maxN, 4096) + 1)})
			default:
				ops = append(ops, op{"read", r.Intn(maxN + 1)})
			}
		}
		// std/bufio keeps each chunk as read (up to its size, 4096) and
		// gathers what a peek or record spans, where Go's buffer is one
		// fixed array: a Go buffer larger than any read keeps the two
		// alike (a chunk is never cut by the space left in Go's buffer)
		br := bufio.NewReaderSize(&bfChunks{data: data, sizes: sizes}, 1<<20)
		var res []string
		for _, p := range ops {
			switch p.kind {
			case "line", "until":
				d := byte('\n')
				if p.kind == "until" {
					d = byte(p.n)
				}
				b, err := br.ReadBytes(d)
				if err != nil && len(b) == 0 {
					res = append(res, "none")
					break
				}
				if p.kind == "until" {
					res = append(res, "b:"+hex.EncodeToString(b))
					break
				}
				m := len(b)
				if m > 0 && b[m-1] == '\n' {
					m--
					if m > 0 && b[m-1] == '\r' {
						m--
					}
				}
				res = append(res, "s:"+hex.EncodeToString([]byte(bfLossy(b[:m]))))
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
		var ss, os []string
		for _, s := range sizes {
			ss = append(ss, fmt.Sprint(s))
		}
		for _, p := range ops {
			os = append(os, fmt.Sprintf("%s:%d", p.kind, p.n))
		}
		cs = append(cs, bfCase{data, strings.Join(ss, " "), strings.Join(os, " "), strings.Join(res, "|")})
	}
	return cs
}

const bfCheckXo = `use std/bufio

/// What a test reads from: data in scripted chunks, optionally an error
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

fn run_ops(r0: bufio.Reader, ops: List[(Str, Int)]) -> List[Str] uses net {
  var r = r0
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

fn ints(s: Str) -> List[Int] { s.split(" ").map(fn(w: Str) -> Int { Int.parse(w) ?? fault("int ${w}") }) }

fn ops_of(s: Str) -> List[(Str, Int)] {
  s.split(" ").map(fn(w: Str) -> (Str, Int) {
    let (k, n) = w.cut(":") ?? fault("op ${w}")
    (k, Int.parse(n) ?? fault("n"))
  })
}

test "std/bufio's Reader answers as Go's bufio.Reader on every chunking" {
  var bad: List[Str] = []
  var k = 0
  for (h, sizes, ops, want) in CASES {
    let data = Bytes.from_hex(h) ?? b""
    let ol = ops_of(ops)
    let got = run_ops(bufio.reader(fake(data, ints(sizes))), ol)
    let ws = want.split("|")
    if got.join("|") != want {
      // the first operation that differs, and the operations before it
      var i = 0
      while i < got.len() && i < ws.len() && got[i] == ws[i] {
        i += 1
      }
      let g = if i < got.len() { got[i] } else { "(none)" }
      let w = if i < ws.len() { ws[i] } else { "(none)" }
      bad.push("case ${k} (${data.len()} bytes, chunks ${sizes}): op ${i} ${ol[i].0}:${ol[i].1} after ${ol[0..i].debug()}: got ${g.len() / 2 - 1} bytes ${g[0..g.len().min(40)]} want ${w.len() / 2 - 1} bytes ${w[0..w.len().min(40)]}")
    }
    k += 1
  }
  expect(bad[0..(if bad.len() < 3 { bad.len() } else { 3 })]) == []
}
`

func bfXoFile(seed int64, cases []bfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/bufiofuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// bufio; do not edit: XO_BUFIOFUZZ_WRITE=1 go test ./internal/stdtest -run TestBufioFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(bfCheckXo)
	b.WriteString("\n/// (data in hex, chunk sizes, operations as kind:n, Go's results joined by |)\nconst CASES: List[(Str, Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%q, %q, %q, %q),\n", hex.EncodeToString(c.data), c.sizes, c.ops, c.rs)
	}
	b.WriteString("]\n")
	return b.String()
}

// bfCoverage fails when the cases stop reaching the end of the data,
// short exact reads and data past the buffer.
func bfCoverage(t *testing.T, cases []bfCase) {
	t.Helper()
	nones, eofs, long := 0, 0, 0
	for _, c := range cases {
		nones += strings.Count(c.rs, "none")
		eofs += strings.Count(c.rs, "eof")
		if len(c.data) > 4096 {
			long++
		}
	}
	n := len(cases)
	if nones < n/2 || eofs < n/10 || long < n/50 {
		t.Fatalf("coverage fell: %d none, %d eof, %d long data in %d cases", nones, eofs, long, n)
	}
}

const bfCorpus = "testdata/corpus/bufiofuzz/bufiofuzz_test.xo"

// TestBufioFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
// Its cases stop at 150 to keep the file small (a tenth hold kilobytes).
func TestBufioFuzzCorpusCurrent(t *testing.T) {
	cases := bfCases(rand.New(rand.NewSource(1)), 150)
	bfCoverage(t, cases)
	bxCorpusCurrent(t, bfCorpus, "XO_BUFIOFUZZ_WRITE", bfXoFile(1, cases))
}

// TestBufioFuzz runs a fresh seed's cases.
func TestBufioFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the bufio fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_BUFIOFUZZ_SEED", 2))
	n := envInt(t, "XO_BUFIOFUZZ_N", 2000)
	cases := bfCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "bufiofuzz_test.xo", bfXoFile(seed, cases), envBackends("XO_BUFIOFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
