package rt

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// Direct JSON decoding (std.md 5). Json_decode first checks the text with
// json.Valid, then decodes it straight into the target with JsonR: no
// token stream, no parsed tree of maps and interfaces. Generated
// `derive Json` structs read their fields with XoJsonRead
// (internal/gen/json.go). The fast path only reports success; on any
// failure Json_decode runs the tree decoder (jsonParse and JsonDec) from
// the start, so error messages, paths, and the duplicate key rule (the
// last value wins) stay exactly those of the tree decoder.

// JsonR reads JSON text that json.Valid accepted.
type JsonR struct {
	b []byte
	i int
}

type jsonReader interface {
	XoJsonRead(tk *Task, r *JsonR) bool
}

func (r *JsonR) ws() {
	for r.i < len(r.b) {
		switch r.b[r.i] {
		case ' ', '\t', '\n', '\r':
			r.i++
		default:
			return
		}
	}
}

func (r *JsonR) peek() byte {
	r.ws()
	if r.i < len(r.b) {
		return r.b[r.i]
	}
	return 0
}

// ObjStart consumes '{' and reports whether it was there.
func (r *JsonR) ObjStart() bool {
	if r.peek() != '{' {
		return false
	}
	r.i++
	return true
}

// Key reads the next key of an object, consuming the ',' before it and
// the ':' after it. more is false at the closing '}', which it consumes.
// ok is false for a key with escapes (the tree decoder handles those).
func (r *JsonR) Key() (key []byte, more, ok bool) {
	c := r.peek()
	if c == '}' {
		r.i++
		return nil, false, true
	}
	if c == ',' {
		r.i++
		r.ws()
	}
	s, ok := r.rawStr()
	if !ok {
		return nil, false, false
	}
	r.ws()
	r.i++ // ':'
	return s, true, true
}

// rawStr reads a string with no escapes and returns its bytes.
func (r *JsonR) rawStr() ([]byte, bool) {
	start := r.i + 1
	for j := start; j < len(r.b); j++ {
		switch r.b[j] {
		case '"':
			r.i = j + 1
			return r.b[start:j], true
		case '\\':
			return nil, false
		}
	}
	return nil, false
}

// Null consumes a null and reports whether it was there.
func (r *JsonR) Null() bool {
	if r.peek() == 'n' {
		r.i += 4
		return true
	}
	return false
}

// Str reads a string value.
func (r *JsonR) Str() (string, bool) {
	if r.peek() != '"' {
		return "", false
	}
	start := r.i
	if s, ok := r.rawStr(); ok {
		if utf8.Valid(s) {
			return string(s), true
		}
		r.i = start
	}
	// escapes or invalid UTF-8: decode exactly as encoding/json does
	r.Skip()
	var out string
	if json.Unmarshal(r.b[start:r.i], &out) != nil {
		return "", false
	}
	return out, true
}

// num reads a number literal.
func (r *JsonR) num() ([]byte, bool) {
	c := r.peek()
	if c != '-' && (c < '0' || c > '9') {
		return nil, false
	}
	start := r.i
	for r.i < len(r.b) {
		c := r.b[r.i]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			r.i++
			continue
		}
		break
	}
	return r.b[start:r.i], true
}

// Skip skips one value.
func (r *JsonR) Skip() {
	switch r.peek() {
	case '"':
		for j := r.i + 1; j < len(r.b); j++ {
			if r.b[j] == '\\' {
				j++
				continue
			}
			if r.b[j] == '"' {
				r.i = j + 1
				return
			}
		}
		r.i = len(r.b)
	case '{', '[':
		depth := 0
		for r.i < len(r.b) {
			switch r.b[r.i] {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					r.i++
					return
				}
			case '"':
				r.Skip()
				continue
			}
			r.i++
		}
	case 't', 'n':
		r.i += 4
	case 'f':
		r.i += 5
	default:
		r.num()
	}
}

// JsonRead decodes one value into dst. It reports false where the tree
// decoder may report an error.
func JsonRead[T any](tk *Task, r *JsonR, dst *T) bool {
	switch d := any(dst).(type) {
	case *string:
		s, ok := r.Str()
		*d = s
		return ok
	case *int64:
		lit, ok := r.num()
		if !ok {
			return false
		}
		n, err := strconv.ParseInt(string(lit), 10, 64)
		*d = n
		return err == nil
	case *float64:
		lit, ok := r.num()
		if !ok {
			return false
		}
		f, err := strconv.ParseFloat(string(lit), 64)
		*d = f
		return err == nil
	case *bool:
		switch r.peek() {
		case 't':
			r.i += 4
			*d = true
			return true
		case 'f':
			r.i += 5
			*d = false
			return true
		}
		return false
	case jsonReader:
		return d.XoJsonRead(tk, r)
	}
	// anything else: parse just this value into a tree and decode it
	r.ws()
	start := r.i
	r.Skip()
	x, err := jsonParse(r.b[start:r.i])
	if err != nil {
		return false
	}
	return JsonDec(tk, dst, x, "$") == nil
}

// jsonDecodeFast is the fast path of Json_decode; se is the scanner's error
// when the text is not valid JSON, which Json_decode answers (jsonErrOf)
// without the value reader or the parse tree.
func jsonDecodeFast[T any](tk *Task, data []byte, dst *T) (ok bool, se *jsSyntaxError) {
	if se := jsCheck(data); se != nil {
		return false, se
	}
	r := JsonR{b: data}
	if !JsonRead(tk, &r, dst) {
		return false, nil
	}
	r.ws()
	return r.i == len(data), nil
}

// XoJsonRead reads an array.
func (l *List[T]) XoJsonRead(tk *Task, r *JsonR) bool {
	if r.peek() != '[' {
		return false
	}
	r.i++
	out := []T{}
	for {
		c := r.peek()
		if c == ']' {
			r.i++
			break
		}
		if c == ',' {
			r.i++
		}
		var v T
		if !JsonRead(tk, r, &v) {
			return false
		}
		out = append(out, v)
	}
	*l = ListOf(out...)
	return true
}

// XoJsonRead reads null as None.
func (o *Option[T]) XoJsonRead(tk *Task, r *JsonR) bool {
	if r.Null() {
		*o = Option[T]{}
		return true
	}
	var v T
	if !JsonRead(tk, r, &v) {
		return false
	}
	*o = Some(v)
	return true
}

// XoJsonRead reads a Patch field: null is Null, a value is Value(v).
func (p *Json_Patch[T]) XoJsonRead(tk *Task, r *JsonR) bool {
	if r.Null() {
		*p = Json_Patch[T]{Tag: 2}
		return true
	}
	var v T
	if !JsonRead(tk, r, &v) {
		return false
	}
	*p = Json_Patch[T]{Tag: 3, P: Json_Patch_Value[T]{F_v: v}}
	return true
}

// Int writes an integer value.
func (w *JsonW) Int(n int64) { w.b = strconv.AppendInt(w.b, n, 10) }

// Bool writes a boolean value.
func (w *JsonW) Bool(b bool) { w.b = strconv.AppendBool(w.b, b) }
