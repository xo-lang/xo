package rt

import (
	"strings"
)

// std/bufio on the Go backend: the same algorithm as std/native/bufio.xo
// over the Stream interface (a chunk kept as read, a record that spans
// chunks gathered once). The corpus checks both against Go's bufio.Reader.

// M_display is the stream's error text, `bufio: record too long`, or
// `bufio: unexpected EOF`.
func (e Bufio_BufErr) M_display(tk *Task) string {
	switch e.Tag {
	case 2:
		return "bufio: record too long"
	case 3:
		return "bufio: unexpected EOF"
	}
	n := e.P.(Bufio_BufErr_Net).F_err
	return n.M_display(tk)
}

// Bufio_Reader is `bufio.Reader`.
type Bufio_Reader struct {
	s    Stream
	cur  string
	pos  int
	size int
	eof  bool
}

func (*Bufio_Reader) XoDebug(b *strings.Builder) { b.WriteString("<bufio.reader>") }
func (x *Bufio_Reader) M_debug(*Task) string     { return "<bufio.reader>" }

// Bufio_reader is `bufio.reader(s, size)`.
func Bufio_reader(_ *Task, s Stream, size int64) Bufio_Reader {
	if size < 16 {
		size = 16
	}
	return Bufio_Reader{s: s, size: int(size)}
}

func bufioNet(e NetErr) Bufio_BufErr { return Bufio_BufErr{Tag: 1, P: Bufio_BufErr_Net{F_err: e}} }

// next replaces the chunk with the stream's next; false at the end.
func (x *Bufio_Reader) next(tk *Task) (bool, *NetErr) {
	for !x.eof {
		r := x.s.M_read(tk, int64(x.size))
		if !r.Ok {
			e := r.E
			return false, &e
		}
		if !r.V.Some {
			x.eof = true
			break
		}
		if len(r.V.V) > 0 {
			x.cur, x.pos = string(r.V.V), 0
			return true, nil
		}
	}
	return false, nil
}

// M_read is `Reader.read(max)`.
func (x *Bufio_Reader) M_read(tk *Task, max int64) Result[Option[Bytes], NetErr] {
	if max <= 0 {
		return Ok[Option[Bytes], NetErr](Some(Bytes("")))
	}
	if x.pos >= len(x.cur) {
		more, e := x.next(tk)
		if e != nil {
			return Err[Option[Bytes]](*e)
		}
		if !more {
			return Ok[Option[Bytes], NetErr](Option[Bytes]{})
		}
	}
	n := len(x.cur) - x.pos
	if int64(n) > max {
		n = int(max)
	}
	out := x.cur[x.pos : x.pos+n]
	x.pos += n
	return Ok[Option[Bytes], NetErr](Some(Bytes(out)))
}

// M_read_until is `Reader.read_until(delim, limit)`.
func (x *Bufio_Reader) M_read_until(tk *Task, delim uint8, limit int64) Result[Option[Bytes], Bufio_BufErr] {
	var acc []byte
	have := 0
	for {
		if x.pos >= len(x.cur) {
			more, e := x.next(tk)
			if e != nil {
				return Err[Option[Bytes]](bufioNet(*e))
			}
			if !more {
				if have == 0 {
					return Ok[Option[Bytes], Bufio_BufErr](Option[Bytes]{})
				}
				return Ok[Option[Bytes], Bufio_BufErr](Some(Bytes(acc)))
			}
		}
		rest := x.cur[x.pos:]
		if j := strings.IndexByte(rest, delim); j >= 0 {
			if int64(have+j+1) > limit {
				return Err[Option[Bytes]](Bufio_BufErr{Tag: 2, P: nil})
			}
			x.pos += j + 1
			if have == 0 {
				return Ok[Option[Bytes], Bufio_BufErr](Some(Bytes(rest[:j+1])))
			}
			acc = append(acc, rest[:j+1]...)
			return Ok[Option[Bytes], Bufio_BufErr](Some(Bytes(acc)))
		}
		if int64(have+len(rest)) >= limit {
			return Err[Option[Bytes]](Bufio_BufErr{Tag: 2, P: nil})
		}
		acc = append(acc, rest...)
		have += len(rest)
		x.pos = len(x.cur)
	}
}

// M_read_line is `Reader.read_line(limit)`.
func (x *Bufio_Reader) M_read_line(tk *Task, limit int64) Result[Option[string], Bufio_BufErr] {
	r := x.M_read_until(tk, '\n', limit)
	if !r.Ok {
		return Err[Option[string]](r.E)
	}
	if !r.V.Some {
		return Ok[Option[string], Bufio_BufErr](Option[string]{})
	}
	b := string(r.V.V)
	n := len(b)
	if n > 0 && b[n-1] == '\n' {
		n--
		if n > 0 && b[n-1] == '\r' {
			n--
		}
	}
	var o strings.Builder
	for _, c := range b[:n] {
		o.WriteRune(c)
	}
	return Ok[Option[string], Bufio_BufErr](Some(o.String()))
}

// M_read_exact is `Reader.read_exact(n)`.
func (x *Bufio_Reader) M_read_exact(tk *Task, n int64) Result[Bytes, Bufio_BufErr] {
	if n <= 0 {
		return Ok[Bytes, Bufio_BufErr](Bytes(""))
	}
	if int64(len(x.cur)-x.pos) >= n {
		out := x.cur[x.pos : x.pos+int(n)]
		x.pos += int(n)
		return Ok[Bytes, Bufio_BufErr](Bytes(out))
	}
	acc := make([]byte, 0, n)
	acc = append(acc, x.cur[x.pos:]...)
	x.pos = len(x.cur)
	for int64(len(acc)) < n {
		more, e := x.next(tk)
		if e != nil {
			return Err[Bytes](bufioNet(*e))
		}
		if !more {
			return Err[Bytes](Bufio_BufErr{Tag: 3, P: nil})
		}
		take := int(n) - len(acc)
		if take > len(x.cur) {
			take = len(x.cur)
		}
		acc = append(acc, x.cur[:take]...)
		x.pos = take
	}
	return Ok[Bytes, Bufio_BufErr](Bytes(acc))
}

// M_peek is `Reader.peek(n)`.
func (x *Bufio_Reader) M_peek(tk *Task, n int64) Result[Bytes, Bufio_BufErr] {
	if n <= 0 {
		return Ok[Bytes, Bufio_BufErr](Bytes(""))
	}
	for int64(len(x.cur)-x.pos) < n {
		old := x.cur[x.pos:]
		more, e := x.next(tk)
		if e != nil {
			x.cur, x.pos = old, 0
			return Err[Bytes](bufioNet(*e))
		}
		if !more {
			x.cur, x.pos = old, 0
			break
		}
		x.cur, x.pos = old+x.cur, 0
	}
	k := len(x.cur) - x.pos
	if int64(k) > n {
		k = int(n)
	}
	return Ok[Bytes, Bufio_BufErr](Bytes(x.cur[x.pos : x.pos+k]))
}

// M_buffered is `Reader.buffered()`.
func (x *Bufio_Reader) M_buffered(*Task) int64 { return int64(len(x.cur) - x.pos) }
