package rt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
func JsonEnc[T any](tk *Task, w *JsonW, v T) {
	jsonEncAny(tk, w, any(v))
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
		w.Raw(jsonFloat(float64(x)))
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

// JsonDec decodes the parsed JSON value x into dst.
func JsonDec[T any](tk *Task, dst *T, x any, path string) *Json_DecodeErr {
	switch d := any(dst).(type) {
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
			return JsonErr(path, "expected number")
		}
		*d = f
	case *float32:
		num, ok := x.(json.Number)
		if !ok {
			return JsonErr(path, "expected number")
		}
		f, err := strconv.ParseFloat(string(num), 32)
		if err != nil {
			return JsonErr(path, "expected number")
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
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
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
}

// jsonParse parses JSON text into *jsonObj, []any, json.Number, string,
// bool, and nil.
func jsonParse(data []byte) (any, *Json_DecodeErr) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	x, err := jsonValue(dec)
	if err != nil {
		return nil, JsonErr("$", "invalid JSON: "+strings.TrimPrefix(err.Error(), "json: "))
	}
	if _, err := dec.Token(); err == nil {
		return nil, JsonErr("$", "invalid JSON: trailing data")
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
				if _, dup := obj.vals[k]; !dup {
					obj.keys = append(obj.keys, k)
				}
				obj.vals[k] = v
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

// Json_encode is `json.encode(v)`.
func Json_encode[T any](tk *Task, v T) Bytes {
	var w JsonW
	JsonEnc(tk, &w, v)
	return Bytes(w.b)
}

// Json_encode_str is `json.encode_str(v)`.
func Json_encode_str[T any](tk *Task, v T) string { return string(Json_encode(tk, v)) }

// Json_decode is `json.decode[T](data)`.
func Json_decode[T any](tk *Task, data Bytes) Result[T, Json_DecodeErr] {
	x, err := jsonParse([]byte(data))
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
	for _, x := range l.s {
		w.Elem()
		JsonEnc(tk, w, x)
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
func (s Set[T]) XoJsonEnc(tk *Task, w *JsonW) { s.M_to_list(tk).XoJsonEnc(tk, w) }

// XoJsonDec decodes an array into a set.
func (s *Set[T]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	var l List[T]
	if err := l.XoJsonDec(tk, x, path); err != nil {
		return err
	}
	*s = l.M_to_set(tk)
	return nil
}

// XoJsonEnc encodes a Map[Str, V] as an object, in insertion order.
func (m Map[K, V]) XoJsonEnc(tk *Task, w *JsonW) {
	w.ObjStart()
	for k, v := range m.All() {
		ks, ok := any(k).(string)
		if !ok {
			FaultHere("only Map[Str, V] encodes as JSON")
		}
		w.Field(ks)
		JsonEnc(tk, w, v)
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
		out.M_put(tk, kk, v)
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

// PatchAbsent reports whether a Patch value is Absent.
func (p Json_Patch[T]) PatchAbsent() bool { return p.Tag <= 1 }

// JsonObject returns the fields of a parsed object for generated decoders.
func JsonObject(x any, path string) (map[string]any, *Json_DecodeErr) {
	obj, ok := x.(*jsonObj)
	if !ok {
		return nil, JsonErr(path, "expected object")
	}
	return obj.vals, nil
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
