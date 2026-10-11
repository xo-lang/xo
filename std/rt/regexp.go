package rt

import (
	"regexp"
	"strconv"
	"strings"
)

// std/regexp (std.md 17, decision 0164): Go's regexp (RE2 syntax, linear
// time). Native builds report it as XO1201 (planned).

// Regexp_Regex is `regexp.Regex`.
type Regexp_Regex struct{ re *regexp.Regexp }

func (Regexp_Regex) XoDebug(b *strings.Builder) { b.WriteString("<regex>") }
func (r Regexp_Regex) M_debug(*Task) string     { return Debug(r) }

// M_display is `invalid regexp "(a": missing closing ): ...`.
func (e Regexp_RegexErr) M_display(*Task) string {
	return "invalid regexp " + strconv.Quote(e.F_pattern) + ": " + e.F_msg
}

// regexErrMsg is Go's message without its "error parsing regexp: " prefix.
func regexErrMsg(err error) string {
	return strings.TrimPrefix(err.Error(), "error parsing regexp: ")
}

// Regexp_compile is `regexp.compile(pattern)`.
func Regexp_compile(_ *Task, pattern string) Result[Regexp_Regex, Regexp_RegexErr] {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Err[Regexp_Regex](Regexp_RegexErr{F_pattern: pattern, F_msg: regexErrMsg(err)})
	}
	return Ok[Regexp_Regex, Regexp_RegexErr](Regexp_Regex{re})
}

// Regexp_must is `regexp.must(pattern)`: faults on a bad pattern.
func Regexp_must(tk *Task, pattern string) Regexp_Regex {
	r := Regexp_compile(tk, pattern)
	if !r.Ok {
		FaultHere("%s", r.E.M_display(tk))
	}
	return r.V
}

// Regexp_quote is `regexp.quote(text)`.
func Regexp_quote(_ *Task, text string) string { return regexp.QuoteMeta(text) }

func (r Regexp_Regex) M_pattern(*Task) string            { return r.re.String() }
func (r Regexp_Regex) M_is_match(_ *Task, s string) bool { return r.re.MatchString(s) }

func (r Regexp_Regex) M_find(_ *Task, s string) Option[string] {
	loc := r.re.FindStringIndex(s)
	if loc == nil {
		return None[string]()
	}
	return Some(s[loc[0]:loc[1]])
}

func (r Regexp_Regex) M_find_index(_ *Task, s string) Option[Tuple2[int64, int64]] {
	loc := r.re.FindStringIndex(s)
	if loc == nil {
		return None[Tuple2[int64, int64]]()
	}
	return Some(Tuple2[int64, int64]{F0: int64(loc[0]), F1: int64(loc[1])})
}

func (r Regexp_Regex) M_find_all(_ *Task, s string) List[string] {
	return ListOf(r.re.FindAllString(s, -1)...)
}

func (r Regexp_Regex) M_captures(_ *Task, s string) Option[List[Option[string]]] {
	loc := r.re.FindStringSubmatchIndex(s)
	if loc == nil {
		return None[List[Option[string]]]()
	}
	out := make([]Option[string], len(loc)/2)
	for i := range out {
		if loc[2*i] >= 0 {
			out[i] = Some(s[loc[2*i]:loc[2*i+1]])
		}
	}
	return Some(ListOf(out...))
}

func (r Regexp_Regex) M_named(_ *Task, s string) Option[Map[string, string]] {
	loc := r.re.FindStringSubmatchIndex(s)
	if loc == nil {
		return None[Map[string, string]]()
	}
	var m Map[string, string]
	for i, name := range r.re.SubexpNames() {
		if name != "" && loc[2*i] >= 0 {
			Map_put(nil, &m, name, s[loc[2*i]:loc[2*i+1]])
		}
	}
	return Some(m)
}

func (r Regexp_Regex) M_replace_all(_ *Task, s, repl string) string {
	return r.re.ReplaceAllString(s, repl)
}

func (r Regexp_Regex) M_replace_all_literal(_ *Task, s, repl string) string {
	return r.re.ReplaceAllLiteralString(s, repl)
}

func (r Regexp_Regex) M_split(_ *Task, s string) List[string] {
	return ListOf(r.re.Split(s, -1)...)
}
