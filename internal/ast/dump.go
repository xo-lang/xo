package ast

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/xo-lang/xo/internal/token"
)

// ToJSON renders a node as indented JSON. Every node object has a "kind"
// (the Go type name) and a "span" ("line:col-line:col"); nil fields, false
// flags, and empty lists are omitted.
func ToJSON(n any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(dump(reflect.ValueOf(n))); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Object is a JSON object that keeps its keys in insertion order, so "kind"
// and "span" come first and fields follow declaration order.
type Object struct {
	Keys   []string
	Values []any
}

func (o *Object) set(k string, v any) {
	o.Keys = append(o.Keys, k)
	o.Values = append(o.Values, v)
}

// MarshalJSON writes the keys in order.
func (o *Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.Keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		buf.Write(kb)
		buf.WriteByte(':')
		var vb bytes.Buffer
		enc := json.NewEncoder(&vb)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(o.Values[i]); err != nil {
			return nil, err
		}
		buf.Write(bytes.TrimRight(vb.Bytes(), "\n"))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Dump returns the generic JSON value (maps, slices, scalars) for n.
func Dump(n any) any { return dump(reflect.ValueOf(n)) }

var (
	spanType = reflect.TypeOf(token.Span{})
	kindType = reflect.TypeOf(token.Kind(0))
	nodeType = reflect.TypeOf((*Node)(nil)).Elem()
)

func dump(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Invalid:
		return nil
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return dump(v.Elem())
	case reflect.Slice:
		if v.Len() == 0 {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = dump(v.Index(i))
		}
		return out
	case reflect.Struct:
		if v.Type() == spanType {
			s := v.Interface().(token.Span)
			return fmt.Sprintf("%d:%d-%d:%d", s.Start.Line, s.Start.Col, s.End.Line, s.End.Col)
		}
		obj := &Object{}
		if reflect.PointerTo(v.Type()).Implements(nodeType) || v.Type().Name() == "StringPart" {
			obj.set("kind", v.Type().Name())
		}
		addFields(obj, v)
		return obj
	case reflect.Bool:
		if !v.Bool() {
			return nil
		}
		return true
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int32, reflect.Int64:
		if v.Type() == kindType {
			return token.Kind(v.Int()).String()
		}
		if v.Type() == reflect.TypeOf(rune(0)) {
			return string(rune(v.Int()))
		}
		if v.Type() == reflect.TypeOf(StringKind(0)) {
			return [...]string{"plain", "triple", "raw"}[v.Int()]
		}
		return v.Int()
	}
	return fmt.Sprint(v.Interface())
}

func addFields(obj *Object, v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && f.Type == reflect.TypeOf(Base{}) {
			obj.set("span", dump(fv.Field(0)))
			continue
		}
		if !f.IsExported() {
			continue
		}
		d := dump(fv)
		if d == nil {
			continue
		}
		if s, ok := d.(string); ok && s == "" {
			continue
		}
		obj.set(jsonName(f.Name), d)
	}
}

// jsonName converts a Go field name to snake_case.
func jsonName(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				out = append(out, '_')
			}
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
