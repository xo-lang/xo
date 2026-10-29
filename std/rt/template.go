package rt

import (
	"errors"
	"strings"
	"text/template"
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
	case 8: // trusted template content (decision 0408): a string to text/template
		return v.P.(Json_Value_Trusted).F_s
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
	fm := template.FuncMap{}
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

func (t Template_Template) M_add(tk *Task, name string, src string) Result[Template_Template, Template_TemplateErr] {
	srcs := append(append([]Tuple2[string, string]{}, t.F_sources.s...), Tuple2[string, string]{F0: name, F1: src})
	n := t
	n.F_sources = ListFromSlice(srcs)
	if _, err := templateSet(tk, n); err != nil {
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
	set, err := templateSet(tk, t)
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
