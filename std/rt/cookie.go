package rt

// Cookies on the Go backend (std.md 4.3, decision 0288): net/http's own
// Cookie, String, ParseCookie, ParseSetCookie, Request.Cookie, Cookies,
// and AddCookie. std/native/http.xo ports them.

import (
	"net/http"
	"time"
)

func goCookie(c Http_Cookie) *http.Cookie {
	g := &http.Cookie{Name: c.F_name, Value: c.F_value, Quoted: c.F_quoted, Path: c.F_path, Domain: c.F_domain, MaxAge: int(c.F_max_age), Secure: c.F_secure, HttpOnly: c.F_http_only, Partitioned: c.F_partitioned}
	if c.F_expires.Some {
		g.Expires = time.Unix(0, int64(c.F_expires.V)).UTC()
	}
	switch c.F_same_site.Tag {
	case 2:
		g.SameSite = http.SameSiteLaxMode
	case 3:
		g.SameSite = http.SameSiteStrictMode
	case 4:
		g.SameSite = http.SameSiteNoneMode
	}
	return g
}

func xoCookie(g *http.Cookie) Http_Cookie {
	c := Http_Cookie{F_name: g.Name, F_value: g.Value, F_quoted: g.Quoted, F_path: g.Path, F_domain: g.Domain, F_max_age: int64(g.MaxAge), F_secure: g.Secure, F_http_only: g.HttpOnly, F_partitioned: g.Partitioned, F_same_site: Http_SameSite{Tag: 1}}
	if !g.Expires.IsZero() {
		// Time spans int64 nanoseconds (about 1678 to 2262, decision 0364),
		// and net/http reads any year up to 9999. A date outside Time has no
		// Expires, the same refusal std/native/http.xo's cookie_date makes
		// through time.instant; UnixNano would otherwise wrap the date to a
		// different year without a word.
		if sec := g.Expires.Unix(); sec >= minTimeSecs && sec <= maxTimeSecs {
			c.F_expires = Some(Time(sec*1_000_000_000 + int64(g.Expires.Nanosecond())))
		}
	}
	switch g.SameSite {
	case http.SameSiteLaxMode:
		c.F_same_site.Tag = 2
	case http.SameSiteStrictMode:
		c.F_same_site.Tag = 3
	case http.SameSiteNoneMode:
		c.F_same_site.Tag = 4
	}
	return c
}

// M_display is `c.display()`: Go's Cookie.String.
func (c Http_Cookie) M_display(*Task) string { return goCookie(c).String() }

// Http_parse_cookie is `http.parse_cookie(line)`.
func Http_parse_cookie(_ *Task, line string) Result[List[Http_Cookie], Http_CookieErr] {
	gs, err := http.ParseCookie(line)
	if err != nil {
		return Err[List[Http_Cookie]](Http_CookieErr{F_msg: err.Error()})
	}
	out := make([]Http_Cookie, len(gs))
	for i, g := range gs {
		out[i] = xoCookie(g)
	}
	return Ok[List[Http_Cookie], Http_CookieErr](ListOf(out...))
}

// Http_parse_set_cookie is `http.parse_set_cookie(line)`.
func Http_parse_set_cookie(_ *Task, line string) Result[Http_Cookie, Http_CookieErr] {
	g, err := http.ParseSetCookie(line)
	if err != nil {
		return Err[Http_Cookie](Http_CookieErr{F_msg: err.Error()})
	}
	return Ok[Http_Cookie, Http_CookieErr](xoCookie(g))
}

// cookieRequest is a net/http request holding r's Cookie field.
func cookieRequest(r Http_Request) *http.Request {
	h := http.Header{}
	if v := headerGet(r.F_headers, "Cookie"); v.Some {
		h.Set("Cookie", v.V)
	}
	return &http.Request{Header: h}
}

// M_cookie is `r.cookie(name)`: Go's Request.Cookie.
func (r Http_Request) M_cookie(_ *Task, name string) Option[Http_Cookie] {
	g, err := cookieRequest(r).Cookie(name)
	if err != nil {
		return None[Http_Cookie]()
	}
	return Some(xoCookie(g))
}

// M_cookies is `r.cookies()`: Go's Request.Cookies.
func (r Http_Request) M_cookies(*Task) List[Http_Cookie] {
	gs := cookieRequest(r).Cookies()
	out := make([]Http_Cookie, len(gs))
	for i, g := range gs {
		out[i] = xoCookie(g)
	}
	return ListOf(out...)
}

// M_with_cookie is `r.with_cookie(c)`: Go's Request.AddCookie.
func (r Http_Request) M_with_cookie(tk *Task, c Http_Cookie) Http_Request {
	hr := cookieRequest(r)
	hr.AddCookie(goCookie(c))
	return r.M_with_header(tk, "Cookie", hr.Header.Get("Cookie"))
}

// httpParseSetCookie is http.ParseSetCookie (here so headers.go needs no
// net/http import of its own).
func httpParseSetCookie(line string) (*http.Cookie, error) { return http.ParseSetCookie(line) }
