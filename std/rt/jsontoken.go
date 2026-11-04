package rt

// json.TokenReader and json.JsonWriter on the Go backend (decision 0495):
// the same state machine and writer as std/native/json.xo's, which follow
// encoding/json's Decoder.Token (stream.go) and its string and float
// forms, so both backends give the same tokens and errors.

import (
	"strconv"
	"strings"
)

const (
	tkTop = iota
	tkArrStart
	tkArrValue
	tkArrComma
	tkObjStart
	tkObjKey
	tkObjColon
	tkObjValue
	tkObjComma
)

type Json_TokenReader struct {
	buf   []byte
	pos   int
	base  int
	max   int64
	state int
	stack []int
	eof   bool
	err   string // an error after tokens a write returned: the next call's
}

func (Json_TokenReader) XoDebug(b *strings.Builder) { b.WriteString("<json token reader>") }
func (x Json_TokenReader) M_debug(*Task) string     { return Debug(x) }

// Json_token_reader is `json.token_reader(max_token)`.
func Json_token_reader(_ *Task, max int64) Json_TokenReader { return Json_TokenReader{max: max} }

// Json_tokens is `json.tokens(data)`.
func Json_tokens(_ *Task, data Bytes) Json_TokenReader {
	b := []byte(data)
	return Json_TokenReader{buf: b, max: int64(len(b)), eof: true}
}

func tkValueAllowed(st int) bool {
	return st == tkTop || st == tkArrStart || st == tkArrValue || st == tkObjValue
}

// tkAfterValue is Go's tokenValueEnd.
func tkAfterValue(st int) int {
	if st == tkArrStart || st == tkArrValue {
		return tkArrComma
	}
	if st == tkObjValue {
		return tkObjComma
	}
	return st
}

func tkErr(msg string) Json_DecodeErr { return Json_DecodeErr{F_path: "$", F_msg: msg} }

// tkBadChar is Go's tokenError.
func tkBadChar(st int, c byte) Json_DecodeErr {
	ctx := " looking for beginning of value"
	switch st {
	case tkArrComma:
		ctx = " after array element"
	case tkObjStart:
		ctx = "" // Go's tokenError has no case for it
	case tkObjKey:
		ctx = " looking for beginning of object key string"
	case tkObjColon:
		ctx = " after object key"
	case tkObjComma:
		ctx = " after object key:value pair"
	}
	return tkErr("invalid JSON: invalid character " + jsonQuoteChar(c) + ctx)
}

var tkEOF = tkErr("invalid JSON: unexpected EOF")

func tkEOFMapped(e Json_DecodeErr) Json_DecodeErr {
	if e.F_msg == "invalid JSON: EOF" || strings.HasSuffix(e.F_msg, "unexpected end of JSON input") {
		return tkEOF
	}
	return e
}

// tkStringEnd is std/native/json.xo's string_end.
func tkStringEnd(b []byte, i int) int {
	n := len(b)
	for k := i + 1; k < n; {
		x := b[k]
		if x == '"' {
			return k + 1
		}
		if x == '\\' {
			k += 2
			continue
		}
		if x < 32 {
			return k + 1
		}
		k++
	}
	return -1
}

// tkStringText is std/native/json.xo's string_text.
func tkStringText(tk *Task, b []byte, lo, hi int) (string, *Json_DecodeErr) {
	plain := hi-lo >= 2 && b[hi-1] == '"'
	for k := lo + 1; plain && k < hi-1; k++ {
		if x := b[k]; x < 32 || x == '\\' || x >= 128 {
			plain = false
		}
	}
	if plain {
		return string(b[lo+1 : hi-1]), nil
	}
	r := Json_parse_value(tk, string(b[lo:hi]))
	if !r.Ok {
		return "", &r.E
	}
	if r.V.Tag == 5 {
		return r.V.P.(Json_Value_Str).F_s, nil
	}
	return "", nil
}

func (r *Json_TokenReader) skipSpace() {
	for r.pos < len(r.buf) && jsonSpace(r.buf[r.pos]) {
		r.pos++
	}
}

func tkTooBig(max int64) Json_DecodeErr {
	return tkErr("invalid JSON: token exceeds max_token (" + strconv.FormatInt(max, 10) + " bytes)")
}

func (r *Json_TokenReader) pop() int {
	n := len(r.stack)
	if n == 0 {
		return tkTop
	}
	st := r.stack[n-1]
	r.stack = r.stack[:n-1]
	return st
}

// pull is std/native/json.xo's pull: the next token, ok false when the
// input is used up or ends in the middle of a token.
func (r *Json_TokenReader) pull(tk *Task) (Json_Token, bool, *Json_DecodeErr) {
	b := r.buf
	n := len(b)
	bad := func(st int, c byte) (Json_Token, bool, *Json_DecodeErr) {
		e := tkBadChar(st, c)
		return Json_Token{}, false, &e
	}
	for {
		r.skipSpace()
		i := r.pos
		if i >= n {
			if r.eof && len(r.stack) > 0 {
				e := tkEOF
				return Json_Token{}, false, &e
			}
			return Json_Token{}, false, nil
		}
		c := b[i]
		st := r.state
		switch c {
		case '[', '{':
			if !tkValueAllowed(st) {
				return bad(st, c)
			}
			r.pos = i + 1
			r.stack = append(r.stack, st)
			if c == '[' {
				r.state = tkArrStart
				return Json_Token{Tag: 3}, true, nil
			}
			r.state = tkObjStart
			return Json_Token{Tag: 1}, true, nil
		case ']':
			if st != tkArrStart && st != tkArrComma {
				return bad(st, c)
			}
			r.pos = i + 1
			r.state = tkAfterValue(r.pop())
			return Json_Token{Tag: 4}, true, nil
		case '}':
			if st != tkObjStart && st != tkObjComma {
				return bad(st, c)
			}
			r.pos = i + 1
			r.state = tkAfterValue(r.pop())
			return Json_Token{Tag: 2}, true, nil
		case ',':
			if st == tkArrComma {
				r.pos = i + 1
				r.state = tkArrValue
				continue
			}
			if st == tkObjComma {
				r.pos = i + 1
				r.state = tkObjKey
				continue
			}
			return bad(st, c)
		case ':':
			if st != tkObjColon {
				return bad(st, c)
			}
			r.pos = i + 1
			r.state = tkObjValue
			continue
		case '"':
			key := st == tkObjStart || st == tkObjKey
			if !key && !tkValueAllowed(st) {
				return bad(st, c)
			}
			end := tkStringEnd(b, i)
			if end < 0 {
				if r.eof {
					e := tkEOF
					return Json_Token{}, false, &e
				}
				if int64(n-i) > r.max {
					e := tkTooBig(r.max)
					return Json_Token{}, false, &e
				}
				return Json_Token{}, false, nil
			}
			s, e := tkStringText(tk, b, i, end)
			if e != nil {
				return Json_Token{}, false, e
			}
			r.pos = end
			if key {
				r.state = tkObjColon
				return Json_Token{Tag: 5, P: Json_Token_Key{F_s: s}}, true, nil
			}
			r.state = tkAfterValue(st)
			return Json_Token{Tag: 6, P: Json_Token_Str{F_s: s}}, true, nil
		}
		num := c == '-' || jsonDigit(c)
		lit := c == 't' || c == 'f' || c == 'n'
		if (!num && !lit) || !tkValueAllowed(st) {
			return bad(st, c)
		}
		var end int
		if num {
			end = jsonNumberEnd(b, i)
		} else {
			end = jsonLiteralEnd(b, i)
		}
		if end < 0 {
			if !r.eof {
				if int64(n-i) > r.max {
					e := tkTooBig(r.max)
					return Json_Token{}, false, &e
				}
				return Json_Token{}, false, nil
			}
			end = n
		}
		v := Json_parse_value(tk, string(b[i:end]))
		if !v.Ok {
			e := tkEOFMapped(v.E)
			return Json_Token{}, false, &e
		}
		r.pos = end
		r.state = tkAfterValue(st)
		if num {
			return Json_Token{Tag: 7, P: Json_Token_Number{F_text: string(b[i:end])}}, true, nil
		}
		if v.V.Tag == 2 {
			return Json_Token{Tag: 8, P: Json_Token_Bool{F_b: v.V.P.(Json_Value_Bool).F_b}}, true, nil
		}
		return Json_Token{Tag: 9}, true, nil
	}
}

func (r *Json_TokenReader) drain(tk *Task) Result[List[Json_Token], Json_DecodeErr] {
	if r.err != "" {
		return Err[List[Json_Token]](tkErr(r.err))
	}
	var out []Json_Token
	for {
		t, ok, e := r.pull(tk)
		if e != nil {
			if len(out) == 0 {
				return Err[List[Json_Token]](*e)
			}
			r.err = e.F_msg
			break
		}
		if !ok {
			break
		}
		out = append(out, t)
	}
	return Ok[List[Json_Token], Json_DecodeErr](ListFromSlice(out))
}

// M_write is `r.write(data)`.
func (r *Json_TokenReader) M_write(tk *Task, data Bytes) Result[List[Json_Token], Json_DecodeErr] {
	if r.pos > 0 {
		r.base += r.pos
		r.buf = append([]byte(nil), r.buf[r.pos:]...)
		r.pos = 0
	}
	r.buf = append(r.buf, data...)
	return r.drain(tk)
}

// M_finish is `r.finish()`.
func (r *Json_TokenReader) M_finish(tk *Task) Result[List[Json_Token], Json_DecodeErr] {
	r.eof = true
	return r.drain(tk)
}

// M_next is `r.next()`.
func (r *Json_TokenReader) M_next(tk *Task) Result[Option[Json_Token], Json_DecodeErr] {
	if r.err != "" {
		return Err[Option[Json_Token]](tkErr(r.err))
	}
	t, ok, e := r.pull(tk)
	if e != nil {
		return Err[Option[Json_Token]](*e)
	}
	if !ok {
		return Ok[Option[Json_Token], Json_DecodeErr](Option[Json_Token]{})
	}
	return Ok[Option[Json_Token], Json_DecodeErr](Some(t))
}

// M_more is `r.more()`.
func (r *Json_TokenReader) M_more(*Task) bool {
	r.skipSpace()
	return r.pos < len(r.buf) && r.buf[r.pos] != ']' && r.buf[r.pos] != '}'
}

// M_offset is `r.offset()`.
func (r *Json_TokenReader) M_offset(*Task) int64 { return int64(r.base + r.pos) }

// toValue is std/native/json.xo's to_value.
func (r *Json_TokenReader) toValue() {
	r.skipSpace()
	if r.pos < len(r.buf) {
		c := r.buf[r.pos]
		if c == ':' && r.state == tkObjColon {
			r.pos++
			r.state = tkObjValue
		} else if c == ',' && r.state == tkArrComma {
			r.pos++
			r.state = tkArrValue
		} else if c == ',' && r.state == tkObjComma {
			r.pos++
			r.state = tkObjKey
		}
		r.skipSpace()
	}
}

// M_skip is `r.skip()`.
func (r *Json_TokenReader) M_skip(tk *Task) Result[Unit, Json_DecodeErr] {
	r.toValue()
	b := r.buf
	i := r.pos
	if i < len(b) && (b[i] == '{' || b[i] == '[') && tkValueAllowed(r.state) {
		end := jsonValueEnd(b, i)
		if end < 0 {
			return Err[Unit](tkEOF)
		}
		if v := Json_parse_value(tk, string(b[i:end])); !v.Ok {
			return Err[Unit](v.E)
		}
		r.pos = end
		r.state = tkAfterValue(r.state)
		return Ok[Unit, Json_DecodeErr](Unit{})
	}
	t, ok, e := r.pull(tk)
	if e != nil {
		return Err[Unit](*e)
	}
	if !ok {
		return Err[Unit](tkEOF)
	}
	switch t.Tag {
	case 5:
		return Err[Unit](tkErr("invalid JSON: no value to skip, an object key is next"))
	case 2:
		return Err[Unit](tkErr("invalid JSON: no value to skip, an object ends"))
	case 4:
		return Err[Unit](tkErr("invalid JSON: no value to skip, an array ends"))
	}
	return Ok[Unit, Json_DecodeErr](Unit{})
}

// M_value is `r.value()`.
func (r *Json_TokenReader) M_value(tk *Task) Result[Json_Value, Json_DecodeErr] {
	r.toValue()
	start := r.pos
	if s := r.M_skip(tk); !s.Ok {
		return Err[Json_Value](s.E)
	}
	return Json_parse_value(tk, string(r.buf[start:r.pos]))
}

// ---------------------------------------------------------------------------
// The writer

type Json_JsonWriter struct {
	out   []byte
	stack []int // per open array (1) or object (0): bit 1 once it holds an element, bit 2 while an object's value is due
	html  bool
	top   int
}

func (Json_JsonWriter) XoDebug(b *strings.Builder) { b.WriteString("<json writer>") }
func (x Json_JsonWriter) M_debug(*Task) string     { return Debug(x) }

// Json_json_writer is `json.json_writer(escape_html)`.
func Json_json_writer(_ *Task, escapeHTML bool) Json_JsonWriter {
	return Json_JsonWriter{html: escapeHTML}
}

func (w *Json_JsonWriter) beginValue() {
	n := len(w.stack)
	if n == 0 {
		if w.top > 0 {
			w.out = append(w.out, '\n')
		}
		w.top++
		return
	}
	t := w.stack[n-1]
	if t&1 == 1 {
		if t&2 != 0 {
			w.out = append(w.out, ',')
		}
		w.stack[n-1] = t | 2
		return
	}
	if t&4 == 0 {
		FaultHere("json: a value is written where an object key is due")
	}
	w.stack[n-1] = t - 4
}

func (w *Json_JsonWriter) M_object_start(*Task) {
	w.beginValue()
	w.out = append(w.out, '{')
	w.stack = append(w.stack, 0)
}

func (w *Json_JsonWriter) M_array_start(*Task) {
	w.beginValue()
	w.out = append(w.out, '[')
	w.stack = append(w.stack, 1)
}

func (w *Json_JsonWriter) M_object_end(*Task) {
	n := len(w.stack)
	if n == 0 || w.stack[n-1]&1 == 1 || w.stack[n-1]&4 != 0 {
		FaultHere("json: object_end with no open object, or a key without its value")
	}
	w.out = append(w.out, '}')
	w.stack = w.stack[:n-1]
}

func (w *Json_JsonWriter) M_array_end(*Task) {
	n := len(w.stack)
	if n == 0 || w.stack[n-1]&1 == 0 {
		FaultHere("json: array_end with no open array")
	}
	w.out = append(w.out, ']')
	w.stack = w.stack[:n-1]
}

// writeString is Go's encodeState.string.
func (w *Json_JsonWriter) writeString(tk *Task, s string) {
	q := Json_encode_str(tk, s)
	if w.html && strings.ContainsAny(q, "<>&") {
		q = htmlEscaper.Replace(q)
	}
	w.out = append(w.out, q...)
}

func (w *Json_JsonWriter) M_key(tk *Task, name string) {
	n := len(w.stack)
	if n == 0 || w.stack[n-1]&1 == 1 || w.stack[n-1]&4 != 0 {
		FaultHere("json: a key is written outside an object, or after a key")
	}
	t := w.stack[n-1]
	if t&2 != 0 {
		w.out = append(w.out, ',')
	}
	w.writeString(tk, name)
	w.out = append(w.out, ':')
	w.stack[n-1] = t | 6
}

func (w *Json_JsonWriter) M_str(tk *Task, s string) {
	w.beginValue()
	w.writeString(tk, s)
}

func (w *Json_JsonWriter) M_int(_ *Task, n int64) {
	w.beginValue()
	w.out = strconv.AppendInt(w.out, n, 10)
}

func (w *Json_JsonWriter) M_float(tk *Task, f float64) Result[Unit, Json_DecodeErr] {
	if f != f {
		return Err[Unit](tkErr("json: unsupported value: NaN"))
	}
	if f > 1.7976931348623157e308 {
		return Err[Unit](tkErr("json: unsupported value: +Inf"))
	}
	if f < -1.7976931348623157e308 {
		return Err[Unit](tkErr("json: unsupported value: -Inf"))
	}
	w.beginValue()
	w.out = append(w.out, Json_encode_str(tk, f)...)
	return Ok[Unit, Json_DecodeErr](Unit{})
}

func (w *Json_JsonWriter) M_number(tk *Task, text string) Result[Unit, Json_DecodeErr] {
	n := len(text)
	if n == 0 || !(text[0] == '-' || jsonDigit(text[0])) || jsonSpace(text[n-1]) || !Json_parse_value(tk, text).Ok {
		return Err[Unit](tkErr("json: invalid number literal \"" + text + "\""))
	}
	w.beginValue()
	w.out = append(w.out, text...)
	return Ok[Unit, Json_DecodeErr](Unit{})
}

func (w *Json_JsonWriter) M_bool(_ *Task, b bool) {
	w.beginValue()
	if b {
		w.out = append(w.out, "true"...)
	} else {
		w.out = append(w.out, "false"...)
	}
}

func (w *Json_JsonWriter) M_null(*Task) {
	w.beginValue()
	w.out = append(w.out, "null"...)
}

func (w *Json_JsonWriter) M_raw(tk *Task, text string) Result[Unit, Json_DecodeErr] {
	c := Json_compact(tk, text)
	if !c.Ok {
		return Err[Unit](c.E)
	}
	s := c.V
	if w.html {
		s = htmlEscaper.Replace(s)
	}
	w.beginValue()
	w.out = append(w.out, s...)
	return Ok[Unit, Json_DecodeErr](Unit{})
}

func (w *Json_JsonWriter) M_value(tk *Task, v Json_Value) Result[Unit, Json_DecodeErr] {
	switch v.Tag {
	case 1:
		w.M_null(tk)
	case 2:
		w.M_bool(tk, v.P.(Json_Value_Bool).F_b)
	case 3:
		w.M_int(tk, v.P.(Json_Value_Int).F_n)
	case 4:
		if r := w.M_float(tk, v.P.(Json_Value_Num).F_n); !r.Ok {
			return r
		}
	case 5:
		w.M_str(tk, v.P.(Json_Value_Str).F_s)
	case 8:
		w.M_str(tk, v.P.(Json_Value_Trusted).F_s)
	case 6:
		w.M_array_start(tk)
		for _, x := range v.P.(Json_Value_List).F_items.s {
			if r := w.M_value(tk, x); !r.Ok {
				return r
			}
		}
		w.M_array_end(tk)
	case 7:
		w.M_object_start(tk)
		for _, f := range v.P.(Json_Value_Object).F_fields.s {
			w.M_key(tk, f.F0)
			if r := w.M_value(tk, f.F1); !r.Ok {
				return r
			}
		}
		w.M_object_end(tk)
	}
	return Ok[Unit, Json_DecodeErr](Unit{})
}

func (w *Json_JsonWriter) M_finish(*Task) Bytes {
	if len(w.stack) > 0 {
		FaultHere("json: finish with an array or object still open")
	}
	return Bytes(w.out)
}

// htmlEscaper writes <, > and & as encoding/json's escapeHTML does.
var htmlEscaper = strings.NewReplacer("<", `\u003c`, ">", `\u003e`, "&", `\u0026`)
