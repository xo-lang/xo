package rt

import (
	"encoding/json"
	"strconv"
	"strings"
)

// json.Value (decision 0299): built from jsonParse's tree, as Json_decode
// reads it; std/native/json.xo builds it from jsontext's tape.

func jsonValueOf(x any) Json_Value {
	switch t := x.(type) {
	case nil:
		return Json_Value{Tag: 1}
	case bool:
		return Json_Value{Tag: 2, P: Json_Value_Bool{F_b: t}}
	case json.Number:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return Json_Value{Tag: 3, P: Json_Value_Int{F_n: n}}
		}
		f, _ := strconv.ParseFloat(string(t), 64)
		return Json_Value{Tag: 4, P: Json_Value_Num{F_n: f}}
	case string:
		return Json_Value{Tag: 5, P: Json_Value_Str{F_s: t}}
	case []any:
		items := make([]Json_Value, len(t))
		for i, e := range t {
			items[i] = jsonValueOf(e)
		}
		return Json_Value{Tag: 6, P: Json_Value_List{F_items: ListFromSlice(items)}}
	case *jsonObj:
		fs := make([]Tuple2[string, Json_Value], len(t.keys))
		for i, k := range t.keys {
			fs[i] = Tuple2[string, Json_Value]{F0: k, F1: jsonValueOf(t.vals[k])}
		}
		return Json_Value{Tag: 7, P: Json_Value_Object{F_fields: ListFromSlice(fs)}}
	}
	return Json_Value{Tag: 1}
}

// Json_parse_value is `json.parse_value(text)`.
func Json_parse_value(_ *Task, text string) Result[Json_Value, Json_DecodeErr] {
	x, err := jsonParse([]byte(text))
	if err != nil {
		return Err[Json_Value](*err)
	}
	return Ok[Json_Value, Json_DecodeErr](jsonValueOf(x))
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
