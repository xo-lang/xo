package rt

// Header fields with repeated names (std.md 4.3, decision 0288): a
// Request's or Response's headers Map is the view; XFields holds every
// field ([name, value, ...]) when a name repeats. std/native/http.xo
// values_of and friends are the native backends'.

import (
	"net/textproto"
	"strings"
)

// fieldView is the Map's value of a name's fields: joined with ", ",
// except Set-Cookie's first (cookies are never joined).
func fieldView(k string, vs []string) string {
	if k == "Set-Cookie" {
		return vs[0]
	}
	return strings.Join(vs, ", ")
}

func appendFields(fs []string, k string, vs []string) []string {
	for _, v := range vs {
		fs = append(fs, textOf(k), textOf(v))
	}
	return fs
}

// valuesOf is a name's values: its fields while the Map still shows them,
// else the Map's one value (a direct change of headers wins), else none.
func valuesOf(h Map[string, string], fs []string, name string) []string {
	k := textproto.CanonicalMIMEHeaderKey(name)
	mv := headerGet(h, k)
	if !mv.Some {
		return nil
	}
	var vs []string
	for i := 0; i+1 < len(fs); i += 2 {
		if fs[i] == k {
			vs = append(vs, fs[i+1])
		}
	}
	if len(vs) > 0 && fieldView(k, vs) == mv.V {
		return vs
	}
	return []string{mv.V}
}

func withoutField(fs []string, k string) []string {
	if len(fs) == 0 {
		return fs
	}
	out := make([]string, 0, len(fs))
	for i := 0; i+1 < len(fs); i += 2 {
		if fs[i] != k {
			out = append(out, fs[i], fs[i+1])
		}
	}
	return out
}

// withValues sets name k's values to vs in a copy of h and fs.
func withValues(tk *Task, h Map[string, string], fs []string, k string, vs []string) (Map[string, string], []string) {
	Map_put(tk, &h, k, fieldView(k, vs))
	out := withoutField(fs, k)
	if len(vs) > 1 {
		for _, v := range vs {
			out = append(out, k, v)
		}
	}
	return h, out
}

// M_header_values is `r.header_values(name)`.
func (r Http_Request) M_header_values(_ *Task, name string) List[string] {
	return ListOf(valuesOf(r.F_headers, r.XFields, name)...)
}

// M_add_header is `r.add_header(name, value)`.
func (r Http_Request) M_add_header(tk *Task, name, value string) Http_Request {
	k := textproto.CanonicalMIMEHeaderKey(name)
	r.F_headers, r.XFields = withValues(tk, r.F_headers, r.XFields, k, append(valuesOf(r.F_headers, r.XFields, k), value))
	return r
}

// M_header_values is `r.header_values(name)`.
func (r Http_Response) M_header_values(_ *Task, name string) List[string] {
	return ListOf(valuesOf(r.F_headers, r.XFields, name)...)
}

// M_add_header is `r.add_header(name, value)`.
func (r Http_Response) M_add_header(tk *Task, name, value string) Http_Response {
	k := textproto.CanonicalMIMEHeaderKey(name)
	r.F_headers, r.XFields = withValues(tk, r.F_headers, r.XFields, k, append(valuesOf(r.F_headers, r.XFields, k), value))
	return r
}

// M_with_cookie is `r.with_cookie(c)`: Go's SetCookie.
func (r Http_Response) M_with_cookie(tk *Task, c Http_Cookie) Http_Response {
	if v := goCookie(c).String(); v != "" {
		return r.M_add_header(tk, "Set-Cookie", v)
	}
	return r
}

// M_cookies is `r.cookies()`: Go's readSetCookies.
func (r Http_Response) M_cookies(*Task) List[Http_Cookie] {
	vs := valuesOf(r.F_headers, r.XFields, "Set-Cookie")
	if len(vs) > 3000 {
		return ListOf[Http_Cookie]()
	}
	var out []Http_Cookie
	for _, v := range vs {
		if g, err := httpParseSetCookie(v); err == nil {
			out = append(out, xoCookie(g))
		}
	}
	return ListOf(out...)
}
