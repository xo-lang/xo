package rt

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// json.Value (decision 0299): built from jsonParse's tree, as Json_decode
// reads it; std/native/json.xo builds it from jsontext's tape.

// jsonValueOf is jsonValueAt at "$", Null for the error (to_value's text
// is an encoding, whose numbers are in range).
func jsonValueOf(x any) Json_Value {
	v, err := jsonValueAt(x, JsonRoot)
	if err != nil {
		return Json_Value{Tag: 1}
	}
	return v
}

// jsonValueAt is the Value of a parsed tree; a number out of Float64's
// range (strconv's range error) is "cannot unmarshal number N into Float"
// at its path, as std/native/json.xo's parse_bytes gives.
func jsonValueAt(x any, path JsonPath) (Json_Value, *Json_DecodeErr) {
	switch t := x.(type) {
	case nil:
		return Json_Value{Tag: 1}, nil
	case bool:
		return Json_Value{Tag: 2, P: Json_Value_Bool{F_b: t}}, nil
	case json.Number:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil && string(t) != "-0" {
			return Json_Value{Tag: 3, P: Json_Value_Int{F_n: n}}, nil
		}
		f, err := strconv.ParseFloat(string(t), 64)
		if err != nil && math.IsInf(f, 0) {
			return Json_Value{}, JsonErr(path, "cannot unmarshal number "+string(t)+" into Float")
		}
		return Json_Value{Tag: 4, P: Json_Value_Num{F_n: f}}, nil
	case string:
		return Json_Value{Tag: 5, P: Json_Value_Str{F_s: t}}, nil
	case []any:
		items := make([]Json_Value, len(t))
		for i, e := range t {
			v, err := jsonValueAt(e, path.Idx(i))
			if err != nil {
				return Json_Value{}, err
			}
			items[i] = v
		}
		return Json_Value{Tag: 6, P: Json_Value_List{F_items: ListFromSlice(items)}}, nil
	case *jsonObj:
		fs := make([]Tuple2[string, Json_Value], len(t.keys))
		if t.all != nil {
			// the values a duplicate key replaced are range checked too, in
			// document order, as Go's decoder reads every member; each
			// member is converted once, its key's last one kept (converting
			// the kept values again doubled the work at every level with a
			// duplicate key: 2^depth, the 0616 json review's F3)
			last := make(map[string]int, len(t.keys))
			for i, m := range t.all {
				last[m.k] = i
			}
			kept := make(map[string]Json_Value, len(t.keys))
			for i, m := range t.all {
				v, err := jsonValueAt(m.v, path.Key("."+m.k))
				if err != nil {
					return Json_Value{}, err
				}
				if last[m.k] == i {
					kept[m.k] = v
				}
			}
			for i, k := range t.keys {
				fs[i] = Tuple2[string, Json_Value]{F0: k, F1: kept[k]}
			}
			return Json_Value{Tag: 7, P: Json_Value_Object{F_fields: ListFromSlice(fs)}}, nil
		}
		for i, k := range t.keys {
			v, err := jsonValueAt(t.vals[k], path.Key("."+k))
			if err != nil {
				return Json_Value{}, err
			}
			fs[i] = Tuple2[string, Json_Value]{F0: k, F1: v}
		}
		return Json_Value{Tag: 7, P: Json_Value_Object{F_fields: ListFromSlice(fs)}}, nil
	}
	return Json_Value{Tag: 1}, nil
}

// Json_parse_value is `json.parse_value(text)`.
func Json_parse_value(_ *Task, text string) Result[Json_Value, Json_DecodeErr] {
	x, err := jsonParse([]byte(text))
	if err != nil {
		return Err[Json_Value](*err)
	}
	v, verr := jsonValueAt(x, JsonRoot)
	if verr != nil {
		return Err[Json_Value](*verr)
	}
	return Ok[Json_Value, Json_DecodeErr](v)
}

// Json_to_value is `json.to_value(v)`: v's encoding, parsed.
func Json_to_value[T any](tk *Task, v T) Json_Value {
	x, err := jsonParse([]byte(Json_encode(tk, v)))
	if err != nil {
		return Json_Value{Tag: 1}
	}
	return jsonValueOf(x)
}

func (v Json_Value) appendTo(tk *Task, b *strings.Builder) {
	switch v.Tag {
	case 1:
		b.WriteString("null")
	case 2:
		if v.P.(Json_Value_Bool).F_b {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case 3:
		b.WriteString(strconv.FormatInt(v.P.(Json_Value_Int).F_n, 10))
	case 4:
		b.WriteString(Json_encode_str(tk, v.P.(Json_Value_Num).F_n))
	case 5:
		b.WriteString(Json_encode_str(tk, v.P.(Json_Value_Str).F_s))
	case 8: // trusted template content (decision 0408): the plain string
		b.WriteString(Json_encode_str(tk, v.P.(Json_Value_Trusted).F_s))
	case 6:
		b.WriteByte('[')
		for i, x := range v.P.(Json_Value_List).F_items.s {
			if i > 0 {
				b.WriteByte(',')
			}
			x.appendTo(tk, b)
		}
		b.WriteByte(']')
	case 7:
		b.WriteByte('{')
		for i, f := range v.P.(Json_Value_Object).F_fields.s {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(Json_encode_str(tk, f.F0))
			b.WriteByte(':')
			f.F1.appendTo(tk, b)
		}
		b.WriteByte('}')
	}
}

func (v Json_Value) M_encode(tk *Task) string {
	var b strings.Builder
	v.appendTo(tk, &b)
	return b.String()
}

func (v Json_Value) M_get(_ *Task, key string) Option[Json_Value] {
	if v.Tag != 7 {
		return Option[Json_Value]{}
	}
	for _, f := range v.P.(Json_Value_Object).F_fields.s {
		if f.F0 == key {
			return Some(f.F1)
		}
	}
	return Option[Json_Value]{}
}

// XoJsonEnc writes v as it is: a json.Value field encodes to its own text
// (std.md 5; Go's any and RawMessage).
func (v Json_Value) XoJsonEnc(tk *Task, w *JsonW) {
	var b strings.Builder
	v.appendTo(tk, &b)
	w.b = append(w.b, b.String()...)
}

// XoJsonDec reads any JSON value into v.
func (v *Json_Value) XoJsonDec(tk *Task, x any, path JsonPath) *Json_DecodeErr {
	// a number out of Float's range fails, as in Go and std/native
	jv, err := jsonValueAt(x, path)
	if err != nil {
		return err
	}
	*v = jv
	return nil
}

// Json_encode_indent is `json.encode_indent(v, prefix, indent)`: Go's
// MarshalIndent (Marshal then Indent).
func Json_encode_indent[T any](tk *Task, v T, prefix string, indent string) Bytes {
	var b bytes.Buffer
	if err := json.Indent(&b, []byte(Json_encode(tk, v)), prefix, indent); err != nil {
		return Json_encode(tk, v)
	}
	return Bytes(b.Bytes())
}

func Json_encode_indent_str[T any](tk *Task, v T, prefix string, indent string) string {
	return string(Json_encode_indent(tk, v, prefix, indent))
}

// Json_indent is `json.indent(text, prefix, indent)`: Go's Indent; a text
// that is not JSON is parse_value's error.
func Json_indent(tk *Task, text string, prefix string, indent string) Result[string, Json_DecodeErr] {
	// the syntax only: Go's Compact and Indent take a number out of
	// Float's range
	if _, err := jsonParse([]byte(text)); err != nil {
		return Err[string](*err)
	}
	var b bytes.Buffer
	if err := json.Indent(&b, []byte(text), prefix, indent); err != nil {
		return Err[string](Json_DecodeErr{F_path: "$", F_msg: "invalid JSON: " + err.Error()})
	}
	return Ok[string, Json_DecodeErr](b.String())
}

// Json_html_escape is `json.html_escape(text)`: Go's HTMLEscape.
func Json_html_escape(tk *Task, text string) string {
	var b bytes.Buffer
	json.HTMLEscape(&b, []byte(text))
	return b.String()
}

// Json_compact is `json.compact(text)`: Go's Compact.
func Json_compact(tk *Task, text string) Result[string, Json_DecodeErr] {
	// the syntax only: Go's Compact and Indent take a number out of
	// Float's range
	if _, err := jsonParse([]byte(text)); err != nil {
		return Err[string](*err)
	}
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(text)); err != nil {
		return Err[string](Json_DecodeErr{F_path: "$", F_msg: "invalid JSON: " + err.Error()})
	}
	return Ok[string, Json_DecodeErr](b.String())
}
