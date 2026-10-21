package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"text/template"
)

// tmplData is the data every template runs on (as JSON).
const tmplData = `{"s":"hello","e":"","i":17,"neg":-3,"z":0,"f":3.25,"big":1e21,"small":0.0001,"t":true,"fa":false,"n":null,` +
	`"list":[1,2,3],"strs":["a","b","c"],"empty":[],"mixed":[1,"x",null,true,2.5],` +
	`"m":{"one":1,"two":2,"three":3},"em":{},"nested":{"a":{"b":{"c":"deep"}},"arr":[{"k":"v1"},{"k":"v2"}]},` +
	`"html":"<b>\"x\" & 'y'</b>","uni":"héllo wörld","num_str":"42","url":"a b&c=d/é"}`

// tmplAny is std/rt's jsonValueAny of JSON text: numbers as int64 when
// they parse as one, else float64.
func tmplAny(x any) any {
	switch t := x.(type) {
	case json.Number:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return n
		}
		f, _ := strconv.ParseFloat(string(t), 64)
		return f
	case []any:
		for i := range t {
			t[i] = tmplAny(t[i])
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = tmplAny(t[k])
		}
		return t
	}
	return x
}

// tmplRun is std/rt's parse and render: the output, or err TEXT.
func tmplRun(src string, data any, missingErr bool) string {
	t, err := template.New("t").Parse(src)
	if err != nil {
		return "err " + textOf(err.Error())
	}
	if missingErr {
		t.Option("missingkey=error")
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "err " + textOf(err.Error())
	}
	return textOf(b.String())
}

var tmplFields = []string{".s", ".e", ".i", ".neg", ".z", ".f", ".big", ".small", ".t", ".fa", ".n", ".list", ".strs", ".empty", ".mixed", ".m", ".em", ".nested.a.b.c", ".nested.arr", ".html", ".uni", ".num_str", ".url", ".missing", ".m.one", ".m.four", ".s.x", ".", "$", "$.i", ".nested.a"}

// tmplArg is an operand: a field, a literal, or a variable.
func tmplArg(r *rand.Rand, vars []string) string {
	switch r.Intn(6) {
	case 0:
		return []string{"1", "0", "-2", "3.5", "1e3", "0x1F", "'a'", `"x"`, "`raw`", "true", "false", "nil", "2i"}[r.Intn(13)]
	case 1:
		if len(vars) > 0 {
			return vars[r.Intn(len(vars))]
		}
	}
	return tmplFields[r.Intn(len(tmplFields))]
}

// tmplPipe is a pipeline: an operand, a function call, or a chain.
func tmplPipe(r *rand.Rand, vars []string, depth int) string {
	switch r.Intn(9) {
	case 0, 1, 2:
		return tmplArg(r, vars)
	case 3:
		f := []string{"len", "not", "html", "js", "urlquery", "print", "println"}[r.Intn(7)]
		return f + " " + tmplArg(r, vars)
	case 4:
		f := []string{"eq", "ne", "lt", "le", "gt", "ge", "and", "or", "index", "print"}[r.Intn(10)]
		s := f + " " + tmplArg(r, vars) + " " + tmplArg(r, vars)
		if r.Intn(3) == 0 {
			s += " " + tmplArg(r, vars)
		}
		return s
	case 5:
		return "printf " + []string{`"%v"`, `"%d"`, `"%s-%q"`, `"%5.2f"`, `"%x"`, `"%T"`, `"%v %v"`, `"%%"`}[r.Intn(8)] + " " + tmplArg(r, vars) + " " + tmplArg(r, vars)
	case 6:
		return "slice " + tmplArg(r, vars) + " " + []string{"1", "0", "2", "5"}[r.Intn(4)]
	case 7:
		if depth > 0 {
			return "(" + tmplPipe(r, vars, depth-1) + ")"
		}
	}
	return tmplArg(r, vars) + " | " + []string{"len", "print", "html", "printf \"%v\"", "not", "urlquery"}[r.Intn(6)]
}

func tmplOpen(r *rand.Rand) string {
	if r.Intn(6) == 0 {
		return "{{- "
	}
	return "{{"
}

func tmplClose(r *rand.Rand) string {
	if r.Intn(6) == 0 {
		return " -}}"
	}
	return "}}"
}

// tmplBody writes a random template body.
func tmplBody(r *rand.Rand, b *strings.Builder, vars []string, depth int, inRange bool) {
	for i, n := 0, 1+r.Intn(4); i < n; i++ {
		switch k := r.Intn(12); {
		case k < 3:
			b.WriteString([]string{"x", " ", "\n", "a b", "<", "é", "  \n  "}[r.Intn(7)])
		case k < 6:
			b.WriteString(tmplOpen(r) + tmplPipe(r, vars, 2) + tmplClose(r))
		case k == 6:
			v := "$v" + strconv.Itoa(len(vars))
			b.WriteString("{{" + v + " := " + tmplPipe(r, vars, 1) + "}}")
			vars = append(vars, v)
		case k == 7 && depth > 0:
			kw := []string{"if", "with"}[r.Intn(2)]
			b.WriteString("{{" + kw + " " + tmplPipe(r, vars, 1) + "}}")
			tmplBody(r, b, vars, depth-1, inRange)
			if r.Intn(2) == 0 {
				if r.Intn(2) == 0 {
					b.WriteString("{{else " + kw + " " + tmplPipe(r, vars, 1) + "}}")
				} else {
					b.WriteString("{{else}}")
				}
				tmplBody(r, b, vars, depth-1, inRange)
			}
			b.WriteString("{{end}}")
		case k == 8 && depth > 0:
			rv := vars
			switch r.Intn(3) {
			case 0:
				b.WriteString("{{range " + tmplArg(r, vars) + "}}")
			case 1:
				b.WriteString("{{range $k, $e := " + tmplArg(r, vars) + "}}")
				rv = append(append([]string{}, vars...), "$k", "$e")
			default:
				b.WriteString("{{range " + []string{"3", "0", "-1", ".i"}[r.Intn(4)] + "}}")
			}
			tmplBody(r, b, rv, depth-1, true)
			if r.Intn(3) == 0 {
				b.WriteString("{{else}}")
				tmplBody(r, b, vars, depth-1, inRange)
			}
			b.WriteString("{{end}}")
		case k == 9 && inRange:
			b.WriteString([]string{"{{break}}", "{{continue}}"}[r.Intn(2)])
		case k == 10:
			b.WriteString([]string{"{{/* c */}}", "{{- /* c */ -}}", "{{.s}", "{{end}}", "{{", "{{else}}", "{{range}}", "{{template \"u\"}}", "{{block \"b\" .}}B{{end}}", "{{define \"d\"}}D{{end}}{{template \"d\" .}}", "{{$}}", "{{.Method 1}}", "{{nil}}", "{{len 3}}", "{{index .list 5}}", "{{index .m \"two\"}}", "{{and}}", "{{call .s}}"}[r.Intn(18)])
		default:
			b.WriteString("{{" + tmplPipe(r, vars, 2) + "}}")
		}
	}
}

func templateCorpus() string {
	r := rand.New(rand.NewSource(8))
	dec := json.NewDecoder(bytes.NewReader([]byte(tmplData)))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		panic(err)
	}
	data := tmplAny(raw)
	var o strings.Builder
	o.WriteString(header("text/template on seeded random templates (pipelines, builtin functions, if, with, range with break and continue, variables, trim markers, define, block, template, bad syntax) over DATA as std/rt/template.go converts it"))
	o.WriteString("use std/json\nuse std/template\n\nconst DATA: Str = " + xq(tmplData) + "\n\n/// (template, missingkey=error, Go's output or err TEXT)\nconst CASES: List[(Str, Bool, Str)] = [\n")
	for i := 0; i < 3000; i++ {
		var b strings.Builder
		tmplBody(r, &b, nil, 3, false)
		src := textOf(b.String())
		missing := r.Intn(8) == 0
		fmt.Fprintf(&o, "  (%s, %v, %s),\n", xq(src), missing, xq(tmplRun(src, data, missing)))
	}
	o.WriteString("]\n")
	o.WriteString(templateTests)
	return o.String()
}

const templateTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "parse and render give Go's output and errors" {
  let data = json.parse_value(DATA).ok() ?? json.Value.Null
  var bad: List[(Str, Str, Str)] = []
  for (src, missing, want) in CASES {
    let got = match template.parse("t", src) {
      Ok(t) => match t.missing_key(missing).render(data) {
        Ok(s) => s
        Err(e) => "err ${e.msg}"
      }
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((src, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
