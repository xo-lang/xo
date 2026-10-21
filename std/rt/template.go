package rt

import (
	"strings"
	"text/template"
)

// std/template on the Go backend (decision 0299): Go's text/template
// itself over the json.Value's any form (nil, bool, int64, float64,
// string, []any, map[string]any); std/native/template.xo ports it.

// jsonValueAny is v as encoding/json would decode it, numbers kept as
// int64 or float64.
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
func templateSet(t Template_Template) (*template.Template, error) {
	var root *template.Template
	for _, s := range t.F_sources.s {
		var x *template.Template
		if root == nil {
			root = template.New(s.F0).Delims(t.F_left, t.F_right)
			x = root
		} else {
			x = root.New(s.F0)
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

func Template_parse(_ *Task, name string, src string, left string, right string) Result[Template_Template, Template_TemplateErr] {
	t := Template_Template{F_name: name, F_sources: ListOf(Tuple2[string, string]{F0: name, F1: src}), F_left: left, F_right: right}
	if _, err := templateSet(t); err != nil {
		return templateErr[Template_Template](err)
	}
	return Ok[Template_Template, Template_TemplateErr](t)
}

func (t Template_Template) M_add(_ *Task, name string, src string) Result[Template_Template, Template_TemplateErr] {
	srcs := append(append([]Tuple2[string, string]{}, t.F_sources.s...), Tuple2[string, string]{F0: name, F1: src})
	n := t
	n.F_sources = ListFromSlice(srcs)
	if _, err := templateSet(n); err != nil {
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

func (t Template_Template) M_render_named(_ *Task, name string, data Json_Value) Result[string, Template_TemplateErr] {
	set, err := templateSet(t)
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
