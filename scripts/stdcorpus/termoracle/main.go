// termoracle writes the std/term line editor corpus: sessions run through
// golang.org/x/term v0.48.0's Terminal over a scripted MockTerminal, with
// every line read and every byte written back (decision 0684).
//
// A session is its settings, its input and its operations; the Xo test
// runs the same operations on term.Terminal over a func_port and compares
// the results and the bytes written.
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"
)

// mock is terminal_test.go's MockTerminal: reads give at most perRead
// bytes (all that fit when 0), io.EOF when the input is used up.
type mock struct {
	mu       sync.Mutex
	toSend   []byte
	perRead  int
	received []byte
}

func (c *mock) Read(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(data)
	if n == 0 {
		return 0, nil
	}
	if n > len(c.toSend) {
		n = len(c.toSend)
	}
	if n == 0 {
		return 0, io.EOF
	}
	if c.perRead > 0 && n > c.perRead {
		n = c.perRead
	}
	copy(data, c.toSend[:n])
	c.toSend = c.toSend[n:]
	return n, nil
}

func (c *mock) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.received = append(c.received, data...)
	return len(data), nil
}

// complete is the sessions' autocomplete: Tab appends "!" at the cursor
// and moves past it; any other key is left to the editor.
func complete(line string, pos int, key rune) (string, int, bool) {
	if key != '\t' {
		return "", 0, false
	}
	return line[:pos] + "!" + line[pos:], pos + 1, true
}

type session struct {
	width, height, perRead int
	prompt                 string
	auto                   bool
	in                     []byte
	ops                    []string // R, P<hex prompt>, S<w>x<h>, W<hex>, B0, B1
}

func run(s session) string {
	c := &mock{toSend: s.in, perRead: s.perRead}
	t := term.NewTerminal(c, s.prompt)
	if s.auto {
		t.AutoCompleteCallback = complete
	}
	var res []string
	if s.width != 80 || s.height != 24 {
		if err := t.SetSize(s.width, s.height); err != nil {
			panic(err)
		}
	}
	for _, op := range s.ops {
		switch op[0] {
		case 'R':
			line, err := t.ReadLine()
			res = append(res, outcome(line, err))
		case 'P':
			p, _ := hex.DecodeString(op[1:])
			line, err := t.ReadPassword(string(p))
			res = append(res, outcome(line, err))
		case 'S':
			var w, h int
			fmt.Sscanf(op[1:], "%dx%d", &w, &h)
			if err := t.SetSize(w, h); err != nil {
				panic(err)
			}
			res = append(res, "s")
		case 'W':
			b, _ := hex.DecodeString(op[1:])
			if _, err := t.Write(b); err != nil {
				panic(err)
			}
			res = append(res, "w")
		case 'B':
			t.SetBracketedPasteMode(op[1] == '1')
			res = append(res, "b")
		}
	}
	return strings.Join(res, "|") + ";" + hex.EncodeToString(c.received)
}

// outcome is ok:<hex line>:<0 or 1 for pasted>, or eof.
func outcome(line string, err error) string {
	switch {
	case err == nil:
		return "ok:" + hex.EncodeToString([]byte(line)) + ":0"
	case errors.Is(err, term.ErrPasteIndicator):
		return "ok:" + hex.EncodeToString([]byte(line)) + ":1"
	case err == io.EOF:
		if line != "" {
			panic("a line with EOF")
		}
		return "eof"
	}
	panic(err)
}

func (s session) spec() string {
	return fmt.Sprintf("%d %d %d %s %t %s %s", s.width, s.height, s.perRead, hex.EncodeToString([]byte(s.prompt)), s.auto, hex.EncodeToString(s.in), strings.Join(s.ops, ","))
}

// keyPress is terminal_test.go's keyPressTests: the input, and the lines
// thrown away before the one checked.
var keyPress = []struct {
	in    string
	throw int
}{
	{"", 0}, {"\r", 0}, {"foo\r", 0}, {"a\x1b[Cb\r", 0}, {"a\x1b[Db\r", 0}, {"a\006b\r", 0}, {"a\002b\r", 0},
	{"a\177b\r", 0}, {"\x1b[A\r", 0}, {"\x1b[B\r", 0}, {"\016\r", 0}, {"\014\r", 0}, {"line\x1b[A\x1b[B\r", 0},
	{"line1\rline2\x1b[A\r", 1}, {"line1\rline2\rline3\x1b[A\x1b[Axxx\r", 2}, {"a b \001\013\r", 0},
	{"a b \001\005\013\r", 0}, {"\027\r", 0}, {"a\027\r", 0}, {"a \027\r", 0}, {"a b\027\r", 0}, {"a b \027\r", 0},
	{"one two thr\x1b[D\027\r", 0}, {"\013\r", 0}, {"a\013\r", 0}, {"ab\x1b[D\013\r", 0}, {"Ξεσκεπάζω\r", 0},
	{"£\r\x1b[A\177\r", 1}, {"£\r££\x1b[A\x1b[B\177\r", 1}, {"a\004\r", 0}, {"ab\x1b[D\004\r", 0},
	{"abcd\x1b[D\x1b[D\025\r", 0}, {"abc\x1b[200~de\177f\x1b[201~\177\r", 0}, {"abc\x1b[200~d\refg\x1b[201~h\r", 1},
	{"abc\x1b[200~d\nefg\x1b[201~h\r", 1}, {"\x1b[200~a\r", 0}, {"\x1b[200~a\n", 0}, {"\003", 0}, {"a\003\r", 0},
	{"abc\x1b[3~\r", 0}, {"\x1b[3~\r", 0}, {"abc\x1b[D\x1b[3~\r", 0}, {"abc\x1b[H\x1b[3~\r", 0},
	{"abc\x1b[H\x1b[3~\x1b[3~\r", 0}, {"abc\x14\r", 0}, {"abc\x14N\r", 0}, {"abc\x1b[D\x14\r", 0},
	{"abcd\x1b[D\x1b[D\x14N\r", 0}, {"abc\x1b[H\x1b[C\x14N\r", 0}, {"a\x14\r", 0}, {"a\x14N\r", 0},
	{"ab\x1b[H\x14\r", 0}, {"ab\x1b[H\x14N\r", 0}, {"\x14\r", 0}, {"abc\x14\x14\r", 0},
	{"abcd\x1b[D\x1b[D\x1b[D\x14\x14\x14\r", 0},
	// renderTests and TestCRLF
	{"abcd\x1b[H\r", 0}, {"cdef\x1b[Hab\r", 0}, {"line1\rline2\r\nline3\n", 2},
}

// keys a random session types: printable text, editing keys and escape
// sequences Go knows, one it does not, and paste markers.
var keys = []string{
	"a", "b", "c", " ", " ", "x y", "0", "~", "é", "日", "£", "😀",
	"\x01", "\x02", "\x05", "\x06", "\x08", "\x0b", "\x0c", "\x0e", "\x10", "\x14", "\x15", "\x17", "\x7f", "\x04", "\t",
	"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[H", "\x1b[F", "\x1b[3~", "\x1b[1;3C", "\x1b[1;3D", "\x1b[5~", "\x1bOA",
	"\x1b[200~", "\x1b[201~", "\r", "\n", "\r\n",
}

var prompts = []string{"> ", "", "$ ", "\x1b[32mgo\x1b[0m> ", "π: ", "a long prompt here: "}

func random(r *rand.Rand) session {
	s := session{width: 80, height: 24, perRead: []int{0, 1, 2, 3, 7}[r.Intn(5)], prompt: prompts[r.Intn(len(prompts))], auto: r.Intn(3) == 0}
	if r.Intn(2) == 0 {
		s.width = 1 + r.Intn(24)
		s.height = 1 + r.Intn(10)
	}
	var in strings.Builder
	for i, n := 0, r.Intn(60); i < n; i++ {
		k := keys[r.Intn(len(keys))]
		if k == "\x03" {
			continue
		}
		in.WriteString(k)
	}
	if r.Intn(4) > 0 {
		in.WriteString("\r")
	}
	s.in = []byte(in.String())
	for i, n := 0, 1+r.Intn(5); i < n; i++ {
		switch r.Intn(10) {
		case 0:
			s.ops = append(s.ops, fmt.Sprintf("S%dx%d", 1+r.Intn(30), 1+r.Intn(10)))
		case 1:
			s.ops = append(s.ops, "W"+hex.EncodeToString([]byte([]string{"note\n", "two\nlines\n", "no newline", "\x1b[31mred\x1b[0m\n"}[r.Intn(4)])))
		case 2:
			s.ops = append(s.ops, "P"+hex.EncodeToString([]byte("Password: ")))
		case 3:
			s.ops = append(s.ops, "B"+fmt.Sprint(r.Intn(2)))
		default:
			s.ops = append(s.ops, "R")
		}
	}
	s.ops = append(s.ops, "R")
	return s
}

// fuzzSeed is XO_TERMFUZZ_SEED (default 0, the committed corpus):
// internal/stdtest/corpusfuzz_test.go's fresh seeds (task #152). A seed
// other than 0 writes only random sessions, 2000 of them, from harsh.
func fuzzSeed() int64 {
	seed, _ := strconv.ParseInt(os.Getenv("XO_TERMFUZZ_SEED"), 10, 64)
	return seed
}

// extra are the pieces harsh adds to a random session's keys: runes of
// two to four bytes, wide ones, bytes that are not UTF-8, escape
// sequences cut short or unknown, and Ctrl-D.
var extra = []string{
	"é", "世", "😀", "\u0301", "\uff21", "\x80", "\xc3", "\xe4\xb8", "\xf0\x9f\x98", "\xff", "\xfe",
	"\x1b", "\x1b[", "\x1b[1;", "\x1b[1;3", "\x1b[200", "\x1b[20~", "\x1b[2;5C", "\x1b[Z", "\x1bO", "\x1bb", "\x1bf",
	"\x00", "\x04", "\x1b\x1b", "\x1b[201~\x1b[200~",
}

// harsh is a random session with extra pieces mixed into its input and,
// one time in four, a long run of letters that wraps at a small width.
func harsh(r *rand.Rand) session {
	s := random(r)
	b := []byte{}
	for _, c := range s.in {
		if r.Intn(4) == 0 {
			b = append(b, extra[r.Intn(len(extra))]...)
		}
		b = append(b, c)
	}
	if r.Intn(4) == 0 {
		at := r.Intn(len(b) + 1)
		run := strings.Repeat(string(rune('a'+r.Intn(26))), 20+r.Intn(300))
		b = append(b[:at:at], append([]byte(run), b[at:]...)...)
	}
	if r.Intn(4) > 0 {
		b = append(b, '\r')
	}
	s.in = b
	return s
}

func main() {
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from golang.org/x/term v0.48.0's Terminal over terminal_test.go's MockTerminal: keyPressTests, renderTests and TestCRLF at several read sizes, and seeded random sessions (keys, escape sequences, pastes, widths, SetSize, Write, ReadPassword, an autocomplete callback), each line read and every byte written (Copyright 2011 The Go Authors, BSD-style license).\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString("use std/term\n\n")
	o.WriteString("/// (session: width height bytes-per-read prompt-hex autocomplete input-hex ops; results joined by | then ; and the bytes written in hex)\nconst SESSIONS: List[(Str, Str)] = [\n")
	emit := func(s session) {
		fmt.Fprintf(&o, "  (%q, %q),\n", s.spec(), run(s))
	}
	if seed := fuzzSeed(); seed != 0 {
		r := rand.New(rand.NewSource(684 + seed))
		for i := 0; i < 2000; i++ {
			emit(harsh(r))
		}
		o.WriteString("]\n")
		o.WriteString(tests)
		fmt.Print(o.String())
		return
	}
	for _, k := range keyPress {
		for _, per := range []int{0, 1, 2, 3, 5} {
			ops := []string{}
			for i := 0; i <= k.throw; i++ {
				ops = append(ops, "R")
			}
			ops = append(ops, "R")
			emit(session{width: 80, height: 24, perRead: per, prompt: "> ", in: []byte(k.in), ops: ops})
		}
	}
	// TestPasswordNotSaved, TestReadPasswordLineEnd and
	// TestReadPasswordDisabledAutoCompleteCallback, TestTerminalSetSize
	// and TestOutputNewlines' shapes
	pw := "P" + hex.EncodeToString([]byte("Password: "))
	emit(session{width: 80, height: 24, perRead: 1, prompt: "> ", in: []byte("password\r\x1b[A\r"), ops: []string{pw, "R"}})
	for _, in := range []string{"\r\n", "\r", "\n", "x\r\n", "x\n", "\r\r", "\n\n"} {
		emit(session{width: 80, height: 24, prompt: "> ", in: []byte(in), ops: []string{pw, "R", "R"}})
	}
	emit(session{width: 80, height: 24, prompt: "> ", auto: true, in: []byte("testpw\t\r"), ops: []string{pw, "R"}})
	for _, w := range []int{0, 1, 5, 40, 120} {
		emit(session{width: 80, height: 24, prompt: "> ", in: []byte("abcdefghij"), ops: []string{"R", fmt.Sprintf("S%dx%d", w, 24), "R"}})
	}
	emit(session{width: 80, height: 24, prompt: "> ", ops: []string{"W" + hex.EncodeToString([]byte("1\n2\n3\n")), "R"}})
	// bytes that are not UTF-8 and runes cut short before ASCII: Go drops
	// each such byte (bytesToKey's RuneError of width 1) and reads on
	// (TestTermFuzz seed 2, task #152)
	for _, in := range []string{"ab\x80cd\rxy\r", "ab\xffcd\rxy\r", "ab\xfecd\r", "ab\xc3cd\rxy\r", "ab\xe4\xb8cd\rxy\r", "ab\xf0\x9f\x98cd\rxy\r", "\x80\x80\r"} {
		for _, per := range []int{0, 1, 3} {
			emit(session{width: 80, height: 24, perRead: per, prompt: "> ", in: []byte(in), ops: []string{"R", "R", "R"}})
		}
	}
	r := rand.New(rand.NewSource(684))
	for i := 0; i < 400; i++ {
		emit(random(r))
	}
	o.WriteString("]\n")
	o.WriteString(tests)
	fmt.Print(o.String())
}

const tests = `
/// A Port reading a session's input as terminal_test.go's MockTerminal
/// (at most per bytes a read, all that fit when 0, None at the end) and
/// recording every byte written.
struct Mock {
  st: Mutex[(Bytes, List[UInt8])]
  per: Int
}

fn port_of(m: Mock) -> term.Port[sync] {
  term.func_port(
    fn(max: Int) -> Bytes? ! Str uses sync {
      Ok(m.st.with(fn(var s: (Bytes, List[UInt8])) -> Bytes? {
        var n = max
        if n > s.0.len() {
          n = s.0.len()
        }
        if n == 0 {
          return None
        }
        if m.per > 0 && n > m.per {
          n = m.per
        }
        let got = s.0[0..n]
        s.0 = s.0[n..s.0.len()]
        Some(got)
      }))
    },
    fn(data: Bytes) -> () ! Str uses sync {
      m.st.with(fn(var s: (Bytes, List[UInt8])) -> () {
        for x in data {
          s.1.push(x)
        }
      })
      Ok(())
    },
  )
}

fn complete(line: Str, pos: Int, key: Int) -> (Str, Int)? {
  if key != 9 {
    return None
  }
  Some((line.bytes()[0..pos].to_str_lossy() + "!" + line.bytes()[pos..line.len()].to_str_lossy(), pos + 1))
}

fn unhex(h: Str) -> Bytes { Bytes.from_hex(h).or_fault("hex") }

fn outcome(r: Result[term.Line, term.TermErr]) -> Str {
  match r {
    Ok(l) => "ok:${l.text.bytes().to_hex()}:${if l.pasted { 1 } else { 0 }}"
    Err(term.TermErr.Eof) => "eof"
    Err(e) => "error ${e}"
  }
}

fn run(spec: Str) -> Str uses sync {
  let f = spec.split(" ")
  let m = Mock{st: Mutex.new((unhex(f[5]), [])), per: Int.parse(f[2]) ?? 0}
  let t = term.new_terminal(port_of(m), unhex(f[3]).to_str_lossy())
  if f[4] == "true" {
    t.set_autocomplete(Some(complete))
  }
  let w = Int.parse(f[0]) ?? 80
  let h = Int.parse(f[1]) ?? 24
  if w != 80 || h != 24 {
    t.set_size(w, h).or_fault("size")
  }
  var res: List[Str] = []
  let ops = if f.len() > 6 && !f[6].is_empty() { f[6].split(",") } else { [] }
  for op in ops {
    let k = op.bytes()[0]
    let arg = op.bytes()[1..op.len()].to_str_lossy()
    if k == 82 {
      res.push(outcome(t.read_line()))
    } else if k == 80 {
      res.push(outcome(t.read_password(unhex(arg).to_str_lossy())))
    } else if k == 83 {
      let wh = arg.split("x")
      t.set_size(Int.parse(wh[0]) ?? 0, Int.parse(wh[1]) ?? 0).or_fault("size")
      res.push("s")
    } else if k == 87 {
      t.write(unhex(arg)).or_fault("write")
      res.push("w")
    } else if k == 66 {
      t.set_bracketed_paste(arg == "1").or_fault("paste")
      res.push("b")
    }
  }
  let got = m.st.with_read(fn(s: (Bytes, List[UInt8])) -> Bytes { Bytes.from_list(s.1) })
  res.join("|") + ";" + got.to_hex()
}

test "std/term's line editor answers and writes what x/term's Terminal does in every session" {
  var bad = 0
  var first = ""
  for (spec, want) in SESSIONS {
    let got = run(spec)
    if got != want {
      if bad == 0 {
        first = "${spec}\n got ${got}\nwant ${want}"
      }
      bad += 1
    }
  }
  if bad > 0 {
    fault("${bad} of ${SESSIONS.len()} sessions differ, first: ${first}")
  }
}

test "conn_port: a Terminal over a connection reads the line typed at the other end and echoes it after the prompt" {
  let l = os.net.listen("127.0.0.1:0")?
  let server = os.tasks.spawn(fn() -> Bytes ! NetErr {
    let c = l.accept()?
    c.write(b"hi\r")?
    var got = BytesBuilder.new()
    while got.len() < 6 {
      match c.read(64)? {
        Some(b) => got.write(b)
        None => break
      }
    }
    c.close()
    Ok(got.freeze())
  })
  let t = term.new_terminal(term.conn_port(os.net.dial(l.addr())?), "> ")
  expect(t.read_line()?.text) == "hi"
  expect(server.await?) == b"> hi\r\n"
  l.close()
}

test "set_history with a ring of two recalls only the last two lines, and set_prompt applies from the next read" {
  let m = Mock{st: Mutex.new((b"a\rb\rc\r\x1b[A\x1b[A\x1b[A\r", [])), per: 0}
  let t = term.new_terminal(port_of(m), "> ")
  t.set_history(term.ring_history(2))
  for want in ["a", "b", "c"] {
    expect(t.read_line()?.text) == want
  }
  t.set_prompt("$ ")
  expect(t.read_line()?.text) == "b"
  let out = m.st.with_read(fn(s: (Bytes, List[UInt8])) -> Bytes { Bytes.from_list(s.1) })
  expect(out.to_str_lossy().contains("c\r\n$ c")) == true
}

test "the escape codes are VT100's, and stdio_port writes through standard output" {
  let t = term.new_terminal(term.stdio_port(os.stdio), "")
  expect(t.escape().red) == b"\x1b[31m"
  expect(term.vt100().reset) == b"\x1b[0m"
  expect(term.KEY_UNKNOWN) == 55296
  t.write(b"")?
}

`
