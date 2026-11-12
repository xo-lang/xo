package rt

// json.Decoder on the Go backend (decision 0309): the same scan as
// std/native/json.xo's take, each value parsed as parse_value parses it,
// so both backends give the same values and errors.

import (
	"reflect"
	"strconv"
	"strings"
)

type Json_Decoder struct {
	buf []byte
	max int64
	err string // an error after values a write returned: the next call's
	// scan is the state of the unfinished value buf begins with, scanned
	// through buf[scanned]: a write scans only its new bytes (it rescanned
	// and copied the whole value on every write, quadratic in a value sent
	// in small pieces: the 0616 json review's F5)
	scan    jsonVScan
	scanned int
}

// jsonVScan is jsonValueEnd's state over a value that may arrive in
// pieces (std/native/json.xo VScan): kind is its first byte (0: none).
type jsonVScan struct {
	kind       byte
	depth      int
	inStr, esc bool
	num, lit   int
}

func jsonScanStart(c byte) jsonVScan {
	switch {
	case c == '{' || c == '[':
		return jsonVScan{kind: c, depth: 1}
	case c == '"':
		return jsonVScan{kind: c, inStr: true}
	case c == '-':
		return jsonVScan{kind: c, num: 1}
	case c == '0':
		return jsonVScan{kind: c, num: 2}
	case jsonDigit(c):
		return jsonVScan{kind: c, num: 3}
	}
	return jsonVScan{kind: c, lit: 1}
}

// jsonScanValue is jsonValueEnd over b[from:] for the value s began: its
// end in b, or -1 when b ends first, s then holding where it stopped.
func jsonScanValue(s *jsonVScan, b []byte, from int) int {
	c := s.kind
	k := from
	n := len(b)
	switch {
	case c == '{' || c == '[' || c == '"':
		for ; k < n; k++ {
			x := b[k]
			switch {
			case s.esc:
				s.esc = false
			case s.inStr:
				if x == '\\' {
					s.esc = true
				} else if x == '"' {
					s.inStr = false
					if c == '"' {
						return k + 1
					}
				}
			case x == '"':
				s.inStr = true
			case x == '{' || x == '[':
				s.depth++
			case x == '}' || x == ']':
				s.depth--
				if s.depth == 0 {
					return k + 1
				}
			}
		}
		return -1
	case c == '-' || jsonDigit(c):
		for ; k < n; k++ {
			x := b[k]
			next := -1
			switch st := s.num; st {
			case 1:
				if x == '0' {
					next = 2
				} else if jsonDigit(x) {
					next = 3
				}
			case 2, 3:
				if st == 3 && jsonDigit(x) {
					next = 3
				} else if x == '.' {
					next = 4
				} else if x == 'e' || x == 'E' {
					next = 6
				} else {
					next = -2
				}
			case 4, 5:
				if jsonDigit(x) {
					next = 5
				} else if st == 5 && (x == 'e' || x == 'E') {
					next = 6
				} else if st == 5 {
					next = -2
				}
			case 6:
				if x == '+' || x == '-' {
					next = 7
				} else if jsonDigit(x) {
					next = 8
				}
			case 7:
				if jsonDigit(x) {
					next = 8
				}
			default:
				if jsonDigit(x) {
					next = 8
				} else {
					next = -2
				}
			}
			if next == -2 {
				return k
			}
			if next == -1 {
				return k + 1
			}
			s.num = next
		}
		return -1
	case c == 't' || c == 'f' || c == 'n':
		lit := "null"
		if c == 't' {
			lit = "true"
		} else if c == 'f' {
			lit = "false"
		}
		for ; s.lit < len(lit); s.lit, k = s.lit+1, k+1 {
			if k >= n {
				return -1
			}
			if b[k] != lit[s.lit] {
				return k + 1
			}
		}
		return k
	}
	return from
}

func (Json_Decoder) XoDebug(b *strings.Builder) { b.WriteString("<json decoder>") }
func (x Json_Decoder) M_debug(*Task) string     { return Debug(x) }

// Json_decoder is `json.decoder(max_value)`.
func Json_decoder(_ *Task, max int64) Json_Decoder { return Json_Decoder{max: max} }

func jsonSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func jsonDigit(c byte) bool { return c >= '0' && c <= '9' }

// jsonNumberEnd is std/native/json.xo's number_end.
func jsonNumberEnd(b []byte, i int) int {
	st := 0
	for k := i; k < len(b); k++ {
		c := b[k]
		next := -1
		switch st {
		case 0:
			if c == '-' {
				next = 1
			} else if c == '0' {
				next = 2
			} else if jsonDigit(c) {
				next = 3
			}
		case 1:
			if c == '0' {
				next = 2
			} else if jsonDigit(c) {
				next = 3
			}
		case 2, 3:
			if st == 3 && jsonDigit(c) {
				next = 3
			} else if c == '.' {
				next = 4
			} else if c == 'e' || c == 'E' {
				next = 6
			} else {
				next = -2
			}
		case 4, 5:
			if jsonDigit(c) {
				next = 5
			} else if st == 5 && (c == 'e' || c == 'E') {
				next = 6
			} else if st == 5 {
				next = -2
			}
		case 6:
			if c == '+' || c == '-' {
				next = 7
			} else if jsonDigit(c) {
				next = 8
			}
		case 7:
			if jsonDigit(c) {
				next = 8
			}
		default:
			if jsonDigit(c) {
				next = 8
			} else {
				next = -2
			}
		}
		if next == -2 {
			return k
		}
		if next == -1 {
			return k + 1
		}
		st = next
	}
	return -1
}

// jsonLiteralEnd is std/native/json.xo's literal_end.
func jsonLiteralEnd(b []byte, i int) int {
	lit := "null"
	if b[i] == 't' {
		lit = "true"
	} else if b[i] == 'f' {
		lit = "false"
	}
	for k := 0; k < len(lit); k++ {
		if i+k >= len(b) {
			return -1
		}
		if b[i+k] != lit[k] {
			return i + k + 1
		}
	}
	return i + len(lit)
}

// jsonValueEnd is std/native/json.xo's value_end.
func jsonValueEnd(b []byte, i int) int {
	n := len(b)
	c := b[i]
	switch {
	case c == '{' || c == '[':
		depth, inStr := 0, false
		for k := i; k < n; k++ {
			x := b[k]
			if inStr {
				if x == '\\' {
					k++
				} else if x == '"' {
					inStr = false
				}
			} else if x == '"' {
				inStr = true
			} else if x == '{' || x == '[' {
				depth++
			} else if x == '}' || x == ']' {
				depth--
				if depth == 0 {
					return k + 1
				}
			}
		}
		return -1
	case c == '"':
		for k := i + 1; k < n; k++ {
			if b[k] == '\\' {
				k++
			} else if b[k] == '"' {
				return k + 1
			}
		}
		return -1
	case c == '-' || jsonDigit(c):
		return jsonNumberEnd(b, i)
	case c == 't' || c == 'f' || c == 'n':
		return jsonLiteralEnd(b, i)
	}
	return i + 1
}

func jsonStreamErr(msg string) Result[List[Json_Value], Json_DecodeErr] {
	return Err[List[Json_Value]](Json_DecodeErr{F_path: "$", F_msg: msg})
}

func (d *Json_Decoder) take(tk *Task, last bool) Result[List[Json_Value], Json_DecodeErr] {
	if d.err != "" {
		return jsonStreamErr(d.err)
	}
	var out []Json_Value
	b := d.buf
	n := len(b)
	i := 0
	err := ""
	tooBig := "invalid JSON: value exceeds max_value (" + strconv.FormatInt(d.max, 10) + " bytes)"
	resume := d.scan.kind != 0
	for {
		for i < n && jsonSpace(b[i]) {
			i++
		}
		if i >= n {
			break
		}
		var end int
		if resume {
			// the value buf begins with, from where the last write's scan
			// stopped
			end = jsonScanValue(&d.scan, b, d.scanned)
			resume = false
		} else {
			d.scan = jsonScanStart(b[i])
			end = jsonScanValue(&d.scan, b, i+1)
		}
		if end < 0 && !last {
			if int64(n-i) > d.max {
				err = tooBig
				d.scan = jsonVScan{}
			} else {
				d.scanned = n - i
			}
			break
		}
		d.scan = jsonVScan{}
		if end < 0 {
			end = n
		}
		if int64(end-i) > d.max {
			err = tooBig
			break
		}
		r := Json_parse_value(tk, string(b[i:end]))
		if !r.Ok {
			d.scan = jsonVScan{}
			msg := r.E.F_msg
			if msg == "invalid JSON: EOF" || strings.HasSuffix(msg, "unexpected end of JSON input") {
				msg = "invalid JSON: unexpected EOF"
			}
			err = msg
			break
		}
		out = append(out, r.V)
		i = end
	}
	if i > 0 {
		// copied only when values were taken: a value still arriving
		// stays where it is, appended to
		d.buf = append([]byte(nil), b[i:]...)
	}
	if err == "" {
		return Ok[List[Json_Value], Json_DecodeErr](ListFromSlice(out))
	}
	if len(out) == 0 {
		return jsonStreamErr(err)
	}
	d.err = err
	return Ok[List[Json_Value], Json_DecodeErr](ListFromSlice(out))
}

// M_write is `d.write(data)`.
func (d *Json_Decoder) M_write(tk *Task, data Bytes) Result[List[Json_Value], Json_DecodeErr] {
	d.buf = append(d.buf, data...)
	return d.take(tk, false)
}

// M_finish is `d.finish()`.
func (d *Json_Decoder) M_finish(tk *Task) Result[List[Json_Value], Json_DecodeErr] {
	return d.take(tk, true)
}

// Json_from_value is `json.from_value[T](v)`.
func Json_from_value[T any](tk *Task, v Json_Value) Result[T, Json_DecodeErr] {
	return Json_decode[T](tk, Bytes(v.M_encode(tk)))
}

// omitempty (decision 0309): the generated XoJsonEnc leaves a field out
// when JsonEmpty is true, as compiler/ssa json.xo's json_empty does.

type jsonEmptier interface{ xoJsonEmpty() bool }

func (o Option[T]) xoJsonEmpty() bool { return !o.Some }
func (l List[T]) xoJsonEmpty() bool   { return len(l.s) == 0 }
func (m Map[K, V]) xoJsonEmpty() bool { return m.n == 0 }
func (s Set[T]) xoJsonEmpty() bool    { return s.m.n == 0 }
func (b Box[T]) xoJsonEmpty() bool    { return b.p == nil || JsonEmpty(*b.p) }

// JsonEmpty is whether v is empty in Go's omitempty sense: None, false,
// 0, 0.0, "", or no elements; a struct or an enum never is.
func JsonEmpty(v any) bool {
	if e, ok := v.(jsonEmptier); ok {
		return e.xoJsonEmpty()
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Bool:
		return !r.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return r.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return r.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return r.Float() == 0
	case reflect.String:
		return r.Len() == 0
	}
	return false
}
