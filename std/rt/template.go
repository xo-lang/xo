package rt

import (
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"reflect"
	"strings"
	"text/template"
	"unicode/utf8"
)

// std/template on the Go backend (decision 0299): Go's text/template
// itself over the json.Value's any form (nil, bool, int64, float64,
// string, []any, map[string]any); std/native/template.xo ports it.

// jsonValueAny is v as encoding/json would decode it, numbers kept as
// int64 or float64.
// Template_Parsed is std/template's opaque parsed set: the Go backend
// keeps text/template's own trees, not this.
type Template_Parsed struct{}

func jsonValueAny(v Json_Value) any {
	switch v.Tag {
	case 2:
		return v.P.(Json_Value_Bool).F_b
	case 3:
		return v.P.(Json_Value_Int).F_n
	case 4:
		return v.P.(Json_Value_Num).F_n
	case 5:
		return v.P.(Json_Value_Str).F_s
	case 8: // trusted template content (decision 0408): html/template's types
		t := v.P.(Json_Value_Trusted)
		switch t.F_kind {
		case "html":
			return htmltemplate.HTML(t.F_s)
		case "css":
			return htmltemplate.CSS(t.F_s)
		case "js":
			return htmltemplate.JS(t.F_s)
		case "jsstr":
			return htmltemplate.JSStr(t.F_s)
		case "url":
			return htmltemplate.URL(t.F_s)
		case "attr":
			return htmltemplate.HTMLAttr(t.F_s)
		case "srcset":
			return htmltemplate.Srcset(t.F_s)
		}
		return t.F_s
	case 6:
		items := v.P.(Json_Value_List).F_items.s
		out := make([]any, len(items))
		for i, x := range items {
			out[i] = jsonValueAny(x)
		}
		return out
	case 7:
		out := map[string]any{}
		for _, f := range v.P.(Json_Value_Object).F_fields.s {
			out[f.F0] = jsonValueAny(f.F1)
		}
		return out
	}
	return nil
}

// templateSet parses t's sources into one set.
// TemplateFn is std/template's TemplateFn on this backend: the arguments
// as json.Values, the result or the call's error text.
type TemplateFn = func(*Task, List[Json_Value]) Result[Json_Value, string]

// templateFuncs is t's functions as text/template's FuncMap (decision
// 0408): each takes any arguments, hands them to the Xo function as
// json.Values, and returns its result as text/template sees data.
func templateFuncs(tk *Task, t Template_Template) template.FuncMap {
	// Xo's slice in place of Go's builtin (a program's own slice still wins)
	fm := template.FuncMap{"slice": templateSlice}
	m := t.F_funcs
	Map_each(&m, func(e *mentry[string, TemplateFn], f TemplateFn) bool {
		name := e.k
		fm[name] = func(args ...any) (any, error) {
			xs := make([]Json_Value, len(args))
			for i, a := range args {
				xs[i] = jsonValueOf(a)
			}
			r := f(tk, ListFromSlice(xs))
			if !r.Ok {
				return nil, errors.New(r.E)
			}
			return jsonValueAny(r.V), nil
		}
		return true
	})
	return fm
}

// templateSlice is text/template's slice builtin, except that a string cut
// inside a character gives U+FFFD for each byte of it, as Xo's Str and
// std/native/template.xo do (decision 0259; Go keeps the bytes, and the
// Go backend's template corpus failed on it, 2026-11-16).
func templateSlice(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error) {
	for item.IsValid() && item.Kind() == reflect.Interface {
		if item.IsNil() {
			item = reflect.Value{}
			break
		}
		item = item.Elem()
	}
	if !item.IsValid() {
		return reflect.Value{}, fmt.Errorf("slice of untyped nil")
	}
	if len(indexes) > 3 {
		return reflect.Value{}, fmt.Errorf("too many slice indexes: %d", len(indexes))
	}
	var cap int
	switch item.Kind() {
	case reflect.String:
		if len(indexes) == 3 {
			return reflect.Value{}, fmt.Errorf("cannot 3-index slice a string")
		}
		cap = item.Len()
	case reflect.Array, reflect.Slice:
		cap = item.Cap()
	default:
		return reflect.Value{}, fmt.Errorf("can't slice item of type %s", item.Type())
	}
	idx := [3]int{0, item.Len()}
	for i, index := range indexes {
		for index.IsValid() && index.Kind() == reflect.Interface && !index.IsNil() {
			index = index.Elem()
		}
		var x int64
		switch index.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			x = index.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			x = int64(index.Uint())
		case reflect.Invalid, reflect.Interface:
			return reflect.Value{}, fmt.Errorf("cannot index slice/array with nil")
		default:
			return reflect.Value{}, fmt.Errorf("cannot index slice/array with type %s", index.Type())
		}
		if x < 0 || int(x) < 0 || int(x) > cap {
			return reflect.Value{}, fmt.Errorf("index out of range: %d", x)
		}
		idx[i] = int(x)
	}
	if idx[0] > idx[1] {
		return reflect.Value{}, fmt.Errorf("invalid slice index: %d > %d", idx[0], idx[1])
	}
	if item.Kind() == reflect.String {
		return reflect.ValueOf(lossyPerByte(item.String()[idx[0]:idx[1]])), nil
	}
	if len(indexes) < 3 {
		return item.Slice(idx[0], idx[1]), nil
	}
	if idx[1] > idx[2] {
		return reflect.Value{}, fmt.Errorf("invalid slice index: %d > %d", idx[1], idx[2])
	}
	return item.Slice3(idx[0], idx[1], idx[2]), nil
}

// lossyPerByte is s with U+FFFD for each byte of an invalid sequence.
func lossyPerByte(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += n
	}
	return b.String()
}

// executor is what both template packages' sets do (decision 0408).
type executor interface {
	ExecuteTemplate(wr io.Writer, name string, data any) error
}

// htmlSet is templateSet for the html mode: html/template's set.
func htmlSet(tk *Task, t Template_Template) (*htmltemplate.Template, error) {
	var root *htmltemplate.Template
	fm := htmltemplate.FuncMap(templateFuncs(tk, t))
	for _, s := range t.F_sources.s {
		var x *htmltemplate.Template
		if root == nil {
			root = htmltemplate.New(s.F0).Delims(t.F_left, t.F_right).Funcs(fm)
			x = root
		} else {
			x = root.New(s.F0).Funcs(fm)
		}
		if _, err := x.Parse(s.F1); err != nil {
			return nil, err
		}
	}
	if t.F_missing_key_error {
		root.Option("missingkey=error")
	}
	return root, nil
}

func anySet(tk *Task, t Template_Template) (executor, error) {
	if t.F_html {
		return htmlSet(tk, t)
	}
	return templateSet(tk, t)
}

func templateSet(tk *Task, t Template_Template) (*template.Template, error) {
	var root *template.Template
	fm := templateFuncs(tk, t)
	for _, s := range t.F_sources.s {
		var x *template.Template
		if root == nil {
			root = template.New(s.F0).Delims(t.F_left, t.F_right).Funcs(fm)
			x = root
		} else {
			x = root.New(s.F0).Funcs(fm)
		}
		if _, err := x.Parse(s.F1); err != nil {
			return nil, err
		}
	}
	if t.F_missing_key_error {
		root.Option("missingkey=error")
	}
	return root, nil
}

func templateErr[T any](err error) Result[T, Template_TemplateErr] {
	return Err[T](Template_TemplateErr{F_msg: err.Error()})
}

func Template_parse(tk *Task, name string, src string, left string, right string, funcs Map[string, TemplateFn]) Result[Template_Template, Template_TemplateErr] {
	t := Template_Template{F_name: name, F_sources: ListOf(Tuple2[string, string]{F0: name, F1: src}), F_left: left, F_right: right, F_funcs: funcs}
	if _, err := templateSet(tk, t); err != nil {
		return templateErr[Template_Template](err)
	}
	return Ok[Template_Template, Template_TemplateErr](t)
}

func Template_parse_html(tk *Task, name string, src string, left string, right string, funcs Map[string, TemplateFn]) Result[Template_Template, Template_TemplateErr] {
	t := Template_Template{F_name: name, F_sources: ListOf(Tuple2[string, string]{F0: name, F1: src}), F_left: left, F_right: right, F_funcs: funcs, F_html: true}
	if _, err := htmlSet(tk, t); err != nil {
		return templateErr[Template_Template](err)
	}
	return Ok[Template_Template, Template_TemplateErr](t)
}

func (t Template_Template) M_add(tk *Task, name string, src string) Result[Template_Template, Template_TemplateErr] {
	srcs := append(append([]Tuple2[string, string]{}, t.F_sources.s...), Tuple2[string, string]{F0: name, F1: src})
	n := t
	n.F_sources = ListFromSlice(srcs)
	if _, err := anySet(tk, n); err != nil {
		return templateErr[Template_Template](err)
	}
	return Ok[Template_Template, Template_TemplateErr](n)
}

func (t Template_Template) M_missing_key(_ *Task, e bool) Template_Template {
	t.F_missing_key_error = e
	return t
}

func (t Template_Template) M_render(tk *Task, data Json_Value) Result[string, Template_TemplateErr] {
	return t.M_render_named(tk, t.F_name, data)
}

func trusted(kind, s string) Json_Value {
	return Json_Value{Tag: 8, P: Json_Value_Trusted{F_kind: kind, F_s: s}}
}

func Template_html(_ *Task, s string) Json_Value   { return trusted("html", s) }
func Template_css(_ *Task, s string) Json_Value    { return trusted("css", s) }
func Template_js(_ *Task, s string) Json_Value     { return trusted("js", s) }
func Template_jsstr(_ *Task, s string) Json_Value  { return trusted("jsstr", s) }
func Template_url(_ *Task, s string) Json_Value    { return trusted("url", s) }
func Template_attr(_ *Task, s string) Json_Value   { return trusted("attr", s) }
func Template_srcset(_ *Task, s string) Json_Value { return trusted("srcset", s) }

func (t Template_Template) M_render_named(tk *Task, name string, data Json_Value) Result[string, Template_TemplateErr] {
	set, err := anySet(tk, t)
	if err != nil {
		return templateErr[string](err)
	}
	var b strings.Builder
	if err := set.ExecuteTemplate(&b, name, jsonValueAny(data)); err != nil {
		return templateErr[string](err)
	}
	// outside bytes as Str (decision 0259): a string sliced inside a rune
	return Ok[string, Template_TemplateErr](textOf(b.String()))
}
