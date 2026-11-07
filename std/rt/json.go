package rt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// std/json (std.md 5). Values of builtin types are encoded by JsonEnc and
// decoded by JsonDec; types with `derive Json` get generated XoJsonEnc and
// XoJsonDec methods (internal/gen/json.go) that call back into these.

// JsonW builds JSON text.
type JsonW struct {
	b     []byte
	count []int // values written in each open object or array
}

type jsonEncoder interface {
	XoJsonEnc(tk *Task, w *JsonW)
}

type jsonDecoder interface {
	XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr
}

func (w *JsonW) sep() {
	if n := len(w.count); n > 0 {
		if w.count[n-1] > 0 {
			w.b = append(w.b, ',')
		}
		w.count[n-1]++
	}
}

// ObjStart opens an object.
func (w *JsonW) ObjStart() {
	w.b = append(w.b, '{')
	w.count = append(w.count, 0)
}

// ObjEnd closes an object.
func (w *JsonW) ObjEnd() {
	w.count = w.count[:len(w.count)-1]
	w.b = append(w.b, '}')
}

// ArrStart opens an array.
func (w *JsonW) ArrStart() {
	w.b = append(w.b, '[')
	w.count = append(w.count, 0)
}

// ArrEnd closes an array.
func (w *JsonW) ArrEnd() {
	w.count = w.count[:len(w.count)-1]
	w.b = append(w.b, ']')
}

// Elem starts an array element.
func (w *JsonW) Elem() { w.sep() }

// Field writes an object key; the value follows.
func (w *JsonW) Field(k string) {
	w.sep()
	w.str(k)
	w.b = append(w.b, ':')
}

// Inline writes the members of v's object into the open object, after
// those already in it: an internally tagged enum's wrapped struct beside
// its tag (decision 0478).
func Inline[T any](tk *Task, w *JsonW, v T) {
	var t JsonW
	JsonEnc(tk, &t, v)
	s := t.b
	if len(s) <= 2 || s[0] != '{' {
		return
	}
	w.sep()
	w.b = append(w.b, s[1:len(s)-1]...)
}

// Str writes a string value.
func (w *JsonW) Str(s string) { w.str(s) }

// str appends s as a JSON string, byte for byte what encoding/json writes
// with SetEscapeHTML(false) (json_test.go checks it), without the
// encoder and buffer that encoding/json allocates per call.
func (w *JsonW) str(s string) { w.b = appendJSONString(w.b, s) }

const jsonHex = "0123456789abcdef"

func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', jsonHex[b>>4], jsonHex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, `\ufffd`...)
			i += size
			start = i
			continue
		}
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', jsonHex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// Raw writes literal JSON.
func (w *JsonW) Raw(s string) { w.b = append(w.b, s...) }

// JsonErr makes a decode error at a path (std.md 5).
func JsonErr(path, msg string) *Json_DecodeErr {
	return &Json_DecodeErr{F_path: path, F_msg: msg}
}

// M_display renders a decode error as "at $.user.email: expected string".
func (e Json_DecodeErr) M_display(*Task) string {
	return "at " + e.F_path + ": " + e.F_msg
}

// JsonEnc writes v.
//
// Str, Int, and Bool are written without an allocation: the type switch
// is on a nil *T and the value is read through unsafe.Pointer(&v), so v
// does not escape. (A switch on any(&v) moved v to the heap on every
// call, whatever its type.) Any other value is copied to the heap once to
// reach its encoder through an interface; internal/gen calls XoJsonEnc
// directly where the type is known, and List encodes its elements in
// place (JsonEncP), so this copy is made only for a top level value or a
// type parameter.
func JsonEnc[T any](tk *Task, w *JsonW, v T) {
	switch any((*T)(nil)).(type) {
	case *string:
		w.str(*(*string)(unsafe.Pointer(&v)))
		return
	case *int64:
		w.Int(*(*int64)(unsafe.Pointer(&v)))
		return
	case *bool:
		w.Bool(*(*bool)(unsafe.Pointer(&v)))
		return
	}
	p := new(T)
	*p = v
	JsonEncP(tk, w, p)
}

// JsonEncQ writes v for a field with the `string` wire option (decision
// 0555): the value's literal inside quotes, a string literal quoted again
// (Go quotes a string field twice), null as it is.
func JsonEncQ[T any](tk *Task, w *JsonW, v T) {
	var t JsonW
	JsonEnc(tk, &t, v)
	lit := string(t.b)
	switch {
	case lit == "null":
		w.Raw(lit)
	case len(lit) > 0 && lit[0] == '"':
		w.str(lit)
	default:
		w.Raw(`"` + lit + `"`)
	}
}

// JsonEncP writes *p. p is not kept; a p into memory that is on the heap
// already (a list's array) costs no allocation.
func JsonEncP[T any](tk *Task, w *JsonW, p *T) {
	switch x := any(p).(type) {
	case *string:
		w.str(*x)
		return
	case *int64:
		w.Int(*x)
		return
	case *bool:
		w.Bool(*x)
		return
	case jsonEncoder:
		x.XoJsonEnc(tk, w)
		return
	}
	jsonEncAny(tk, w, any(*p))
}

func jsonEncAny(tk *Task, w *JsonW, v any) {
	switch x := v.(type) {
	case int64:
		w.Raw(strconv.FormatInt(x, 10))
	case int8, int16, int32, uint8, uint16, uint32, uint64:
		w.Raw(fmt.Sprint(x))
	case float64:
		w.Raw(jsonFloat(x))
	case float32:
		w.Raw(jsonFloat32(x))
	case bool:
		w.Raw(strconv.FormatBool(x))
	case string:
		w.str(x)
	case Rune:
		w.str(string(rune(x)))
	case Bytes:
		w.str(base64.StdEncoding.EncodeToString([]byte(x)))
	case Duration:
		w.str(x.String())
	case Time:
		w.str(x.String())
	case Unit:
		w.Raw("null")
	case jsonEncoder:
		x.XoJsonEnc(tk, w)
	case Dyn:
		jsonEncAny(tk, w, x.V)
	case nil:
		w.Raw("null")
	default:
		FaultHere("a value of type %T cannot be encoded as JSON", v)
	}
}

func jsonFloat(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "null"
	}
	data, _ := json.Marshal(f)
	return string(data)
}

// jsonFloat32 is encoding/json's float32 text: the shortest digits that read back as the float32 (1.1, not
// 1.100000023841858).
func jsonFloat32(f float32) string {
	if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
		return "null"
	}
	data, _ := json.Marshal(f)
	return string(data)
}

// JsonDec decodes the parsed JSON value x into dst.
func JsonDec[T any](tk *Task, dst *T, x any, path string) *Json_DecodeErr {
	if dd, ok := x.(*jsonDup); ok {
		// the values a duplicate key replaced decode first, their results
		// dropped, as Go's decoder reads every member
		for _, p := range dd.prev {
			var s T
			if err := JsonDec(tk, &s, p, path); err != nil {
				return err
			}
		}
		x = dd.last
	}
	// the commonest kinds here, which Go resolves per type; every other
	// kind in jsonDecAny, compiled once rather than once per decoded type
	// (each struct is a type shape of its own: armnetwork's Go backend
	// build compiled 3,521 copies of the whole switch, 9.7 MB)
	switch d := any(dst).(type) {
	case jsonDecoder:
		return d.XoJsonDec(tk, x, path)
	case *string:
		s, ok := x.(string)
		if !ok {
			return JsonErr(path, "expected string")
		}
		*d = s
		return nil
	case *int64:
		n, err := jsonInt(x, path, 64, true)
		if err != nil {
			return err
		}
		*d = n
		return nil
	case *bool:
		b, ok := x.(bool)
		if !ok {
			return JsonErr(path, "expected boolean")
		}
		*d = b
		return nil
	}
	return jsonDecAny(tk, dst, x, path)
}

// JsonDecQ decodes x into dst for a field with the `string` wire option
// (decision 0555): x must be a string (or null, which dst's own decoder
// judges), its text parsed as JSON of its own with no space around it, and
// dst decoded from that. A text that is not JSON, or that dst does not
// take, is Go's `invalid use of ,string struct tag` error, except a
// number's own error (an overflow). tn is the type's name in the message.
func JsonDecQ[T any](tk *Task, dst *T, x any, path string, tn string) *Json_DecodeErr {
	if dd, ok := x.(*jsonDup); ok {
		// the values a duplicate key replaced decode first (JsonDec), as
		// quoted values
		for _, p := range dd.prev {
			var s T
			if err := JsonDecQ(tk, &s, p, path, tn); err != nil {
				return err
			}
		}
		x = dd.last
	}
	if x == nil {
		return JsonDec(tk, dst, x, path)
	}
	s, ok := x.(string)
	if !ok {
		return JsonErr(path, "invalid use of ,string struct tag, trying to unmarshal unquoted value into "+tn)
	}
	bad := JsonErr(path, "invalid use of ,string struct tag, trying to unmarshal \""+s+"\" into "+tn)
	if s == "" || strings.ContainsRune(" \t\n\r", rune(s[0])) || strings.ContainsRune(" \t\n\r", rune(s[len(s)-1])) {
		return bad
	}
	y, perr := jsonParse([]byte(s))
	if perr != nil {
		return bad
	}
	if err := JsonDec(tk, dst, y, path); err != nil {
		if _, isNum := y.(json.Number); !isNum {
			return bad
		}
		return err
	}
	return nil
}

// jsonDecAny is JsonDec's body for the other kinds, not generic.
func jsonDecAny(tk *Task, dst any, x any, path string) *Json_DecodeErr {
	switch d := dst.(type) {
	case *int64:
		n, err := jsonInt(x, path, 64, true)
		if err != nil {
			return err
		}
		*d = n
	case *int8:
		n, err := jsonInt(x, path, 8, true)
		if err != nil {
			return err
		}
		*d = int8(n)
	case *int16:
		n, err := jsonInt(x, path, 16, true)
		if err != nil {
			return err
		}
		*d = int16(n)
	case *int32:
		n, err := jsonInt(x, path, 32, true)
		if err != nil {
			return err
		}
		*d = int32(n)
	case *uint8:
		n, err := jsonUint(x, path, 8)
		if err != nil {
			return err
		}
		*d = uint8(n)
	case *uint16:
		n, err := jsonUint(x, path, 16)
		if err != nil {
			return err
		}
		*d = uint16(n)
	case *uint32:
		n, err := jsonUint(x, path, 32)
		if err != nil {
			return err
		}
		*d = uint32(n)
	case *uint64:
		n, err := jsonUint(x, path, 64)
		if err != nil {
			return err
		}
		*d = n
	case *float64:
		num, ok := x.(json.Number)
		if !ok {
			return JsonErr(path, "expected number")
		}
		f, err := num.Float64()
		if err != nil {
			// out of range: Go's words with Xo's type (decode's native
			// llrt_json_range_msg)
			return JsonErr(path, "cannot unmarshal number "+string(num)+" into Float")
		}
		*d = f
	case *float32:
		num, ok := x.(json.Number)
		if !ok {
			return JsonErr(path, "expected number")
		}
		f, err := strconv.ParseFloat(string(num), 32)
		if err != nil {
			return JsonErr(path, "cannot unmarshal number "+string(num)+" into Float32")
		}
		*d = float32(f)
	case *bool:
		b, ok := x.(bool)
		if !ok {
			return JsonErr(path, "expected boolean")
		}
		*d = b
	case *string:
		s, ok := x.(string)
		if !ok {
			return JsonErr(path, "expected string")
		}
		*d = s
	case *Rune:
		s, ok := x.(string)
		rs := []rune(s)
		if !ok || len(rs) != 1 {
			return JsonErr(path, "expected a one character string")
		}
		*d = Rune(rs[0])
	case *Bytes:
		s, ok := x.(string)
		if !ok {
			return JsonErr(path, "expected base64 string")
		}
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return JsonErr(path, "expected base64 string")
		}
		*d = Bytes(data)
	case *Duration:
		s, ok := x.(string)
		if !ok {
			return JsonErr(path, "expected duration string")
		}
		dd, ok2 := parseDuration(s)
		if !ok2 {
			return JsonErr(path, "expected duration string such as \"1.5s\"")
		}
		*d = dd
	case *Time:
		s, ok := x.(string)
		if !ok {
			return JsonErr(path, "expected RFC 3339 time string")
		}
		t, ok := parseRFC3339(s)
		if !ok || !inTimeRange(t) {
			return JsonErr(path, "expected RFC 3339 time string")
		}
		*d = Time(t.UnixNano())
	case *Unit:
		if x != nil {
			return JsonErr(path, "expected null")
		}
	case jsonDecoder:
		return d.XoJsonDec(tk, x, path)
	default:
		return JsonErr(path, fmt.Sprintf("cannot decode into %s", reflect.TypeOf(dst).Elem()))
	}
	return nil
}

func jsonInt(x any, path string, bits int, signed bool) (int64, *Json_DecodeErr) {
	num, ok := x.(json.Number)
	if !ok {
		return 0, JsonErr(path, "expected integer")
	}
	n, err := strconv.ParseInt(string(num), 10, bits)
	if err != nil {
		return 0, JsonErr(path, "expected integer")
	}
	return n, nil
}

func jsonUint(x any, path string, bits int) (uint64, *Json_DecodeErr) {
	num, ok := x.(json.Number)
	if !ok {
		return 0, JsonErr(path, "expected integer")
	}
	n, err := strconv.ParseUint(string(num), 10, bits)
	if err != nil {
		return 0, JsonErr(path, "expected non negative integer")
	}
	return n, nil
}

// jsonObj is a parsed JSON object that keeps its key order.
type jsonObj struct {
	keys []string
	vals map[string]any
	// reps are the non-null values a later member of the same key
	// replaced, in document order: Go's decoder reads every member, so a
	// typed decode reads them too (std/native's jsontext.find_dups and
	// replaced; a null into a value is a no-op in Go)
	reps []jsonRep
	// all is every member as written, kept from an object's first
	// duplicate key on (nil without one): a Value's range check names
	// the first number out of range in document order
	all []jsonRep
}

// jsonRep is a member whose value a later member of its key replaced.
type jsonRep struct {
	k string
	v any
}

// jsonDup is a struct field's value with the values it replaced
// (JsonObject), which JsonDec decodes first.
type jsonDup struct {
	prev []any
	last any
}

// put sets member k to v, keeping a replaced non-null value in reps.
func (o *jsonObj) put(k string, v any) {
	if old, dup := o.vals[k]; !dup {
		o.keys = append(o.keys, k)
	} else {
		if old != nil {
			o.reps = append(o.reps, jsonRep{k, old})
		}
		if o.all == nil {
			// no key repeated before this one: the members so far are
			// the keys with their values
			o.all = make([]jsonRep, len(o.keys))
			for i, kk := range o.keys {
				o.all[i] = jsonRep{kk, o.vals[kk]}
			}
		}
	}
	if o.all != nil {
		o.all = append(o.all, jsonRep{k, v})
	}
	o.vals[k] = v
}

// jsonParse parses JSON text into *jsonObj, []any, json.Number, string,
// bool, and nil.
func jsonParse(data []byte) (any, *Json_DecodeErr) {
	if x, ok := jsonParseFast(data); ok {
		return x, nil
	}
	if err := jsonSyntaxErr(data); err != nil {
		return nil, err
	}
	return jsonParseSlow(data)
}

// jsonQuoteChar is encoding/json's quoteChar: the byte as a rune in single
// quotes.
func jsonQuoteChar(c byte) string {
	if c == '\'' {
		return `'\''`
	}
	if c == '"' {
		return `'"'`
	}
	s := strconv.Quote(string(rune(c)))
	return "'" + s[1:len(s)-1] + "'"
}

// jsonEOFErr is the token decoder's error for a text that ends early:
// io.EOF where it ends between tokens (white space, a delimiter, a
// closed string, a whole number or literal), io.ErrUnexpectedEOF inside one.
func jsonEOFErr(data []byte) *Json_DecodeErr {
	inStr := false
	for i := 0; i < len(data); i++ {
		switch {
		case inStr && data[i] == '\\':
			i++
		case data[i] == '"':
			inStr = !inStr
		}
	}
	d := bytes.TrimRight(data, " \t\r\n")
	if inStr {
		return JsonErr("$", "invalid JSON: unexpected EOF")
	}
	if len(d) == 0 {
		return JsonErr("$", "invalid JSON: EOF")
	}
	switch d[len(d)-1] {
	case '{', '[', ',', ':', '}', ']', '"':
		return JsonErr("$", "invalid JSON: EOF")
	}
	i := len(d)
	for i > 0 && (d[i-1] == '+' || d[i-1] == '-' || d[i-1] == '.' || d[i-1] >= '0' && d[i-1] <= '9' || d[i-1] >= 'a' && d[i-1] <= 'z' || d[i-1] >= 'A' && d[i-1] <= 'Z') {
		i--
	}
	if json.Valid(d[i:]) {
		return JsonErr("$", "invalid JSON: EOF")
	}
	return JsonErr("$", "invalid JSON: unexpected EOF")
}

// jsonSyntaxErr is jsonParseSlow's error for text the scanner of
// encoding/json rejects, found by that scan alone (Go's Unmarshal checks
// the syntax before it decodes) without the token decoder building the
// value up to the error; nil for text that is valid, or whose error text
// the token decoder words differently (a bad object key), which jsonParseSlow answers.
func jsonSyntaxErr(data []byte) *Json_DecodeErr {
	se := jsCheck(data)
	if se == nil {
		return nil
	}
	return jsonErrOf(se, data)
}

// jsonErrOf is jsonSyntaxErr for the error se of jsCheck(data).
func jsonErrOf(se *jsSyntaxError, data []byte) *Json_DecodeErr {
	msg := se.Error()
	if strings.HasSuffix(msg, "looking for beginning of object key string") {
		return nil
	}
	if msg == "unexpected end of JSON input" {
		return jsonEOFErr(data)
	}
	// the scan of a text that ends inside a literal or a number feeds a
	// space: the byte the error names must be the one at its offset
	if i := int(se.Offset) - 1; strings.HasPrefix(msg, "invalid character ") && (i < 0 || i >= len(data) || !strings.HasPrefix(msg[len("invalid character "):], jsonQuoteChar(data[i]))) {
		return jsonEOFErr(data)
	}
	return JsonErr("$", "invalid JSON: "+msg)
}

// jsonParseSlow is jsonParse by the token decoder: any text, and the
// error text of a bad one.
func jsonParseSlow(data []byte) (any, *Json_DecodeErr) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	x, err := jsonValue(dec)
	if err != nil {
		return nil, JsonErr("$", "invalid JSON: "+strings.TrimPrefix(err.Error(), "json: "))
	}
	// one value and white space (RFC 8259; Unmarshal's checkValid)
	for i := int(dec.InputOffset()); i < len(data); i++ {
		if c := data[i]; c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return nil, JsonErr("$", "invalid JSON: invalid character "+jsonQuoteChar(c)+" after top-level value")
		}
	}
	return x, nil
}

func jsonValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := &jsonObj{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := jsonValue(dec)
				if err != nil {
					return nil, err
				}
				obj.put(k, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := jsonValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)
	}
	return tok, nil
}

// Json_encode is `json.encode(v)`. The text is built in a pooled buffer
// and copied out once, at its final size: growing a fresh buffer cost
// several allocations per value (about ten for a list of 64 records).
func Json_encode[T any](tk *Task, v T) Bytes {
	w := getJsonW()
	JsonEnc(tk, w, v)
	out := Bytes(w.b)
	putJsonW(w)
	return out
}

var jsonWPool = sync.Pool{New: func() any { return new(JsonW) }}

// jsonWKeep is the largest buffer returned to the pool.
const jsonWKeep = 64 << 10

func getJsonW() *JsonW { return jsonWPool.Get().(*JsonW) }

func putJsonW(w *JsonW) {
	if cap(w.b) > jsonWKeep {
		return
	}
	w.b, w.count = w.b[:0], w.count[:0]
	jsonWPool.Put(w)
}

// Json_encode_str is `json.encode_str(v)`.
func Json_encode_str[T any](tk *Task, v T) string { return string(Json_encode(tk, v)) }

// Json_decode is `json.decode[T](data)`.
func Json_decode[T any](tk *Task, data Bytes) Result[T, Json_DecodeErr] {
	var fast T
	b := []byte(data)
	ok, se := jsonDecodeFast(tk, b, &fast)
	if ok {
		return Ok[T, Json_DecodeErr](fast)
	}
	if se != nil {
		if err := jsonErrOf(se, b); err != nil {
			return Err[T](*err)
		}
	}
	x, err := jsonParse(b)
	if err != nil {
		return Err[T](*err)
	}
	var v T
	if e := JsonDec(tk, &v, x, "$"); e != nil {
		return Err[T](*e)
	}
	return Ok[T, Json_DecodeErr](v)
}

// Json_decode_str is `json.decode_str[T](s)`.
func Json_decode_str[T any](tk *Task, s string) Result[T, Json_DecodeErr] {
	return Json_decode[T](tk, Bytes(s))
}

// ---------------------------------------------------------------------------
// Builtin generic types

// XoJsonEnc encodes a list as an array.
func (l List[T]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ArrStart()
	for i := range l.s {
		w.Elem()
		JsonEncP(tk, w, &l.s[i])
	}
	w.ArrEnd()
}

// XoJsonDec decodes an array.
func (l *List[T]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	arr, ok := x.([]any)
	if !ok {
		return JsonErr(path, "expected array")
	}
	out := make([]T, len(arr))
	for i, e := range arr {
		if err := JsonDec(tk, &out[i], e, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	*l = ListOf(out...)
	return nil
}

// XoJsonEnc encodes a set as an array.
func (s Set[T]) XoJsonEnc(tk *Task, w *JsonW) { Set_to_list(tk, s).XoJsonEnc(tk, w) }

// XoJsonDec decodes an array into a set.
func (s *Set[T]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	var l List[T]
	if err := l.XoJsonDec(tk, x, path); err != nil {
		return err
	}
	*s = List_to_set(tk, l)
	return nil
}

// XoJsonEnc encodes a Map[Str, V] as an object with its keys sorted by their bytes, as encoding/json does
// (decision 0543).
func (m Map[K, V]) XoJsonEnc(tk *Task, w *JsonW) {
	type entry struct {
		k string
		v V
	}
	es := make([]entry, 0, m.n)
	for k, v := range Map_All(m) {
		ks, ok := any(k).(string)
		if !ok {
			FaultHere("only Map[Str, V] encodes as JSON")
		}
		es = append(es, entry{ks, v})
	}
	slices.SortStableFunc(es, func(a, b entry) int { return strings.Compare(a.k, b.k) })
	w.ObjStart()
	for _, e := range es {
		w.Field(e.k)
		JsonEnc(tk, w, e.v)
	}
	w.ObjEnd()
}

// XoJsonDec decodes an object into a Map[Str, V]; keys keep the order of
// the text.
func (m *Map[K, V]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	obj, ok := x.(*jsonObj)
	if !ok {
		return JsonErr(path, "expected object")
	}
	// the values a duplicate key replaced decode first, their results
	// dropped, as Go's decoder reads every member
	for _, r := range obj.reps {
		var v V
		if err := JsonDec(tk, &v, r.v, path+"."+r.k); err != nil {
			return err
		}
	}
	var out Map[K, V]
	for _, k := range obj.keys {
		kk, ok := any(k).(K)
		if !ok {
			return JsonErr(path, "only Map[Str, V] decodes from JSON")
		}
		var v V
		if err := JsonDec(tk, &v, obj.vals[k], path+"."+k); err != nil {
			return err
		}
		Map_put(tk, &out, kk, v)
	}
	*m = out
	return nil
}

// XoJsonEnc encodes None as null.
func (o Option[T]) XoJsonEnc(tk *Task, w *JsonW) {
	if !o.Some {
		w.Raw("null")
		return
	}
	JsonEnc(tk, w, o.V)
}

// XoJsonDec decodes null as None.
func (o *Option[T]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	if x == nil {
		*o = Option[T]{}
		return nil
	}
	var v T
	if err := JsonDec(tk, &v, x, path); err != nil {
		return err
	}
	*o = Some(v)
	return nil
}

// XoJsonEnc encodes a Patch field value: Null as null, Value(v) as v.
// Absent fields are left out by the enclosing struct.
func (p Json_Patch[T]) XoJsonEnc(tk *Task, w *JsonW) {
	switch p.Tag {
	case 3:
		JsonEnc(tk, w, p.P.(Json_Patch_Value[T]).F_v)
	default:
		w.Raw("null")
	}
}

// XoJsonDec decodes a Patch field: null is Null, a value is Value(v). A
// missing key is set to Absent by the enclosing struct.
func (p *Json_Patch[T]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	if x == nil {
		*p = Json_Patch[T]{Tag: 2}
		return nil
	}
	var v T
	if err := JsonDec(tk, &v, x, path); err != nil {
		return err
	}
	*p = Json_Patch[T]{Tag: 3, P: Json_Patch_Value[T]{F_v: v}}
	return nil
}

// Json_Patch_PatchAbsent reports whether a Patch value is Absent.
func Json_Patch_PatchAbsent[T any](p Json_Patch[T]) bool { return p.Tag <= 1 }

// JsonObject returns the fields of a parsed object for generated decoders.
func JsonObject(x any, path string) (map[string]any, *Json_DecodeErr) {
	obj, ok := x.(*jsonObj)
	if !ok {
		return nil, JsonErr(path, "expected object")
	}
	if len(obj.reps) == 0 {
		return obj.vals, nil
	}
	// a duplicate key's field reads its replaced values first (JsonDec)
	m := make(map[string]any, len(obj.vals))
	for k, v := range obj.vals {
		m[k] = v
	}
	for _, r := range obj.reps {
		d, ok := m[r.k].(*jsonDup)
		if !ok {
			d = &jsonDup{last: obj.vals[r.k]}
			m[r.k] = d
		}
		d.prev = append(d.prev, r.v)
	}
	return m, nil
}

// JsonObjectFold reads the object x for a type with `derive Json(match:
// fold)` (decision 0550), keys its fields' keys in declaration order: each
// member, in document order, selects the field whose key equals it, else
// the first field whose key folds equal to it (jsonFoldName), as
// encoding/json does. The result holds each field's members in document
// order, the last one selecting it last, with the earlier null ones left
// out (a null into a value is a no-op in Go), as std/native's
// jsontext.find_all_fold.
func JsonObjectFold(x any, path string, keys []string) ([][]JsonFoldMember, *Json_DecodeErr) {
	obj, ok := x.(*jsonObj)
	if !ok {
		return nil, JsonErr(path, "expected object")
	}
	members := obj.all
	if members == nil {
		members = make([]jsonRep, len(obj.keys))
		for i, k := range obj.keys {
			members[i] = jsonRep{k, obj.vals[k]}
		}
	}
	exact := make(map[string]int, len(keys))
	folded := make(map[string]int, len(keys))
	for i, k := range keys {
		exact[k] = i
		fk := string(jsonFoldName([]byte(k)))
		if _, dup := folded[fk]; !dup {
			folded[fk] = i
		}
	}
	out := make([][]JsonFoldMember, len(keys))
	for pos, mb := range members {
		i, ok := exact[mb.k]
		if !ok {
			i, ok = folded[string(jsonFoldName([]byte(mb.k)))]
			if !ok {
				continue
			}
		}
		if n := len(out[i]); n > 0 && out[i][n-1].V == nil {
			out[i] = out[i][:n-1]
		}
		out[i] = append(out[i], JsonFoldMember{pos, mb.v})
	}
	return out, nil
}

// JsonFoldMember is a member JsonObjectFold selected: its position among
// the object's members and its value.
type JsonFoldMember struct {
	Pos int
	V   any
}

// JsonFoldErr is the error a folding type's decoder returns (decision
// 0550), in encoding/json's order: Go keeps the first type error in
// document order and goes on, while an UnmarshalJSON error stops the
// decode and wins. Every member decodes; Note keeps a hard error (a via
// type's) before any type error, else the earliest position.
type JsonFoldErr struct {
	pos  int
	hard bool
	err  *Json_DecodeErr
}

// JsonFoldMissing is the position of a missing field's error: after
// every member's (Go reports none).
const JsonFoldMissing = 1 << 62

// Note keeps err (nil: none) of the member at pos when it comes first.
func (f *JsonFoldErr) Note(pos int, hard bool, err *Json_DecodeErr) {
	if err == nil {
		return
	}
	earlier := f.err == nil || pos < f.pos
	if (hard && (!f.hard || earlier)) || (!hard && !f.hard && earlier) {
		f.pos, f.hard, f.err = pos, hard, err
	}
}

// Err is the error kept, or nil.
func (f *JsonFoldErr) Err() *Json_DecodeErr { return f.err }

// jsonFoldName is encoding/json's foldName (fold.go, Copyright 2013 The
// Go Authors, BSD license): ASCII letters upper cased, any other rune
// through unicode.ToUpper(unicode.ToLower(r)), an invalid byte as U+FFFD.
func jsonFoldName(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); {
		if c := in[i]; c < utf8.RuneSelf {
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			out = append(out, c)
			i++
			continue
		}
		r, n := utf8.DecodeRune(in[i:])
		out = utf8.AppendRune(out, unicode.ToUpper(unicode.ToLower(r)))
		i += n
	}
	return out
}

// JsonEnumInt reads the integer value of a valued enum (decision 0269).
func JsonEnumInt(x any, path string) (int64, *Json_DecodeErr) { return jsonInt(x, path, 64, true) }

// JsonEnumUnknown is the error of a value no variant of enum name has.
func JsonEnumUnknown(path, name, value string, quoted bool) *Json_DecodeErr {
	if quoted {
		value = "`" + value + "`"
	}
	return JsonErr(path, "unknown value "+value+" of enum "+name)
}

// JsonEnumUnknownInt is JsonEnumUnknown for an Int valued enum.
func JsonEnumUnknownInt(path, name string, n int64) *Json_DecodeErr {
	return JsonEnumUnknown(path, name, strconv.FormatInt(n, 10), false)
}

// JsonEnumStr reads the string value of a valued enum (decision 0269).
func JsonEnumStr(x any, path string) (string, *Json_DecodeErr) {
	s, ok := x.(string)
	if !ok {
		return "", JsonErr(path, "expected string")
	}
	return s, nil
}

// JsonVariant splits an enum's JSON form into the variant name and its
// payload (nil for a unit variant written as a string).
func JsonVariant(x any, path string) (string, any, bool, *Json_DecodeErr) {
	switch y := x.(type) {
	case string:
		return y, nil, false, nil
	case *jsonObj:
		if len(y.keys) != 1 {
			return "", nil, false, JsonErr(path, "expected an object with exactly one variant key")
		}
		return y.keys[0], y.vals[y.keys[0]], true, nil
	}
	return "", nil, false, JsonErr(path, "expected a variant name or an object with one variant key")
}

// Tuples (decision 0088): a tuple is a JSON array with one element per
// position, decoded only from an array of exactly that length.

// jsonTupleDec decodes the array x into the tuple positions, one decoder
// each.
func jsonTupleDec(x any, path string, fs ...func(any, string) *Json_DecodeErr) *Json_DecodeErr {
	arr, ok := x.([]any)
	if !ok {
		return JsonErr(path, "expected array")
	}
	if len(arr) != len(fs) {
		return JsonErr(path, fmt.Sprintf("expected array of %d elements, got %d", len(fs), len(arr)))
	}
	for i, f := range fs {
		if err := f(arr[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

// XoJsonEnc encodes a tuple as an array.
func (t Tuple2[A, B]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ArrStart()
	w.Elem()
	JsonEncP(tk, w, &t.F0)
	w.Elem()
	JsonEncP(tk, w, &t.F1)
	w.ArrEnd()
}

// XoJsonDec decodes an array of 2 elements.
func (t *Tuple2[A, B]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	return jsonTupleDec(x, path,
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F0, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F1, e, p) },
	)
}

// XoJsonEnc encodes a tuple as an array.
func (t Tuple3[A, B, C]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ArrStart()
	w.Elem()
	JsonEncP(tk, w, &t.F0)
	w.Elem()
	JsonEncP(tk, w, &t.F1)
	w.Elem()
	JsonEncP(tk, w, &t.F2)
	w.ArrEnd()
}

// XoJsonDec decodes an array of 3 elements.
func (t *Tuple3[A, B, C]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	return jsonTupleDec(x, path,
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F0, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F1, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F2, e, p) },
	)
}

// XoJsonEnc encodes a tuple as an array.
func (t Tuple4[A, B, C, D]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ArrStart()
	w.Elem()
	JsonEncP(tk, w, &t.F0)
	w.Elem()
	JsonEncP(tk, w, &t.F1)
	w.Elem()
	JsonEncP(tk, w, &t.F2)
	w.Elem()
	JsonEncP(tk, w, &t.F3)
	w.ArrEnd()
}

// XoJsonDec decodes an array of 4 elements.
func (t *Tuple4[A, B, C, D]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	return jsonTupleDec(x, path,
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F0, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F1, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F2, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F3, e, p) },
	)
}

// XoJsonEnc encodes a tuple as an array.
func (t Tuple5[A, B, C, D, E]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ArrStart()
	w.Elem()
	JsonEncP(tk, w, &t.F0)
	w.Elem()
	JsonEncP(tk, w, &t.F1)
	w.Elem()
	JsonEncP(tk, w, &t.F2)
	w.Elem()
	JsonEncP(tk, w, &t.F3)
	w.Elem()
	JsonEncP(tk, w, &t.F4)
	w.ArrEnd()
}

// XoJsonDec decodes an array of 5 elements.
func (t *Tuple5[A, B, C, D, E]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	return jsonTupleDec(x, path,
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F0, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F1, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F2, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F3, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F4, e, p) },
	)
}

// XoJsonEnc encodes a tuple as an array.
func (t Tuple6[A, B, C, D, E, F]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ArrStart()
	w.Elem()
	JsonEncP(tk, w, &t.F0)
	w.Elem()
	JsonEncP(tk, w, &t.F1)
	w.Elem()
	JsonEncP(tk, w, &t.F2)
	w.Elem()
	JsonEncP(tk, w, &t.F3)
	w.Elem()
	JsonEncP(tk, w, &t.F4)
	w.Elem()
	JsonEncP(tk, w, &t.F5)
	w.ArrEnd()
}

// XoJsonDec decodes an array of 6 elements.
func (t *Tuple6[A, B, C, D, E, F]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	return jsonTupleDec(x, path,
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F0, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F1, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F2, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F3, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F4, e, p) },
		func(e any, p string) *Json_DecodeErr { return JsonDec(tk, &t.F5, e, p) },
	)
}
