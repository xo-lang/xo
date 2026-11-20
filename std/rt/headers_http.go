//go:build !xo_nohttp

package rt

import (
	"net/textproto"
)

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
