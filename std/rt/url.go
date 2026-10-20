package rt

import (
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// std/url (std.md 20, decision 0279): Go's net/url escaping and Values.
// One difference from Go: a decoded byte sequence that is not UTF-8
// keeps each invalid byte as its %XX escape (decision 0259), as the
// native backends' std/native/url.xo does.

// M_display is Go's text, `invalid URL escape "%zz"`.
func (e Url_EscapeErr) M_display(*Task) string { return e.F_msg }

func urlText(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteByte('%')
			b.WriteByte(hex[s[i]>>4])
			b.WriteByte(hex[s[i]&15])
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

func urlUnescaped(s string, err error) Result[string, Url_EscapeErr] {
	if err != nil {
		return Err[string](Url_EscapeErr{F_msg: err.Error()})
	}
	return Ok[string, Url_EscapeErr](urlText(s))
}

func Url_query_escape(_ *Task, s string) string { return url.QueryEscape(s) }
func Url_path_escape(_ *Task, s string) string  { return url.PathEscape(s) }

func Url_query_unescape(_ *Task, s string) Result[string, Url_EscapeErr] {
	return urlUnescaped(url.QueryUnescape(s))
}

func Url_path_unescape(_ *Task, s string) Result[string, Url_EscapeErr] {
	return urlUnescaped(url.PathUnescape(s))
}

// Url_Values is `url.Values`: keys in first appearance order. A value:
// every change copies the slices it writes, so copies never share a
// write.
type Url_Values struct {
	ks []string
	vs [][]string
}

func (v Url_Values) XoEq(x any) bool {
	o, ok := x.(Url_Values)
	return ok && slices.Equal(v.ks, o.ks) && slices.EqualFunc(v.vs, o.vs, slices.Equal)
}

func (v Url_Values) XoHash() uint64 {
	h := uint64(0x5a17)
	for i, k := range v.ks {
		h = MixHash(h, Hash(k))
		for _, x := range v.vs[i] {
			h = MixHash(h, Hash(x))
		}
	}
	return h
}

func (v Url_Values) XoDebug(b *strings.Builder) {
	b.WriteString("Values{")
	for i, k := range v.ks {
		if i > 0 {
			b.WriteString(", ")
		}
		DebugTo(b, k)
		b.WriteString(": ")
		DebugTo(b, ListOf(v.vs[i]...))
	}
	b.WriteString("}")
}

func (v Url_Values) M_debug(*Task) string { return Debug(v) }

func Url_values(*Task) Url_Values { return Url_Values{} }

func (v Url_Values) M_get(_ *Task, key string) Option[string] {
	if i := slices.Index(v.ks, key); i >= 0 && len(v.vs[i]) > 0 {
		return Some(v.vs[i][0])
	}
	return None[string]()
}

func (v Url_Values) M_all(_ *Task, key string) List[string] {
	if i := slices.Index(v.ks, key); i >= 0 {
		return ListOf(v.vs[i]...)
	}
	return ListOf[string]()
}

func (v Url_Values) M_has(_ *Task, key string) bool { return slices.Contains(v.ks, key) }

func (v Url_Values) M_keys(*Task) List[string] { return ListOf(v.ks...) }

func (v Url_Values) M_encode(*Task) string {
	m := url.Values{}
	for i, k := range v.ks {
		m[k] = v.vs[i]
	}
	return m.Encode()
}

func (v *Url_Values) M_add(_ *Task, key, value string) {
	ks, vs := slices.Clone(v.ks), slices.Clone(v.vs)
	if i := slices.Index(ks, key); i >= 0 {
		vs[i] = append(slices.Clone(vs[i]), value)
	} else {
		ks = append(ks, key)
		vs = append(vs, []string{value})
	}
	v.ks, v.vs = ks, vs
}

func (v *Url_Values) M_set(_ *Task, key, value string) {
	ks, vs := slices.Clone(v.ks), slices.Clone(v.vs)
	if i := slices.Index(ks, key); i >= 0 {
		vs[i] = []string{value}
	} else {
		ks = append(ks, key)
		vs = append(vs, []string{value})
	}
	v.ks, v.vs = ks, vs
}

func (v *Url_Values) M_remove(_ *Task, key string) {
	if i := slices.Index(v.ks, key); i >= 0 {
		v.ks = slices.Delete(slices.Clone(v.ks), i, i+1)
		v.vs = slices.Delete(slices.Clone(v.vs), i, i+1)
	}
}

// Url_parse_query is `url.parse_query(s)`: url.ParseQuery's pairs in
// order of first appearance; Go's first error, without the pairs parsed
// around it.
func Url_parse_query(_ *Task, s string) Result[Url_Values, Url_EscapeErr] {
	if _, err := url.ParseQuery(s); err != nil {
		return Err[Url_Values](Url_EscapeErr{F_msg: err.Error()})
	}
	v := Url_Values{}
	at := map[string]int{}
	for _, pair := range strings.Split(s, "&") {
		if pair == "" {
			continue
		}
		k, x, _ := strings.Cut(pair, "=")
		k, _ = url.QueryUnescape(k)
		x, _ = url.QueryUnescape(x)
		k, x = urlText(k), urlText(x)
		if i, ok := at[k]; ok {
			v.vs[i] = append(v.vs[i], x)
		} else {
			at[k] = len(v.ks)
			v.ks = append(v.ks, k)
			v.vs = append(v.vs, []string{x})
		}
	}
	return Ok[Url_Values, Url_EscapeErr](v)
}
