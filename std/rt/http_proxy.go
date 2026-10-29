package rt

import (
	"net/textproto"
	"strings"
)

// A reverse proxy (decision 0404 stage D): httputil.ReverseProxy's
// Rewrite with SetURL and SetXForwarded, on Client.open and a streamed
// request body, so the Go backend gives the native one's behavior.

// hopHeaders are httputil's hop-by-hop fields, removed both ways.
var hopHeaders = []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func withoutHop(tk *Task, h Map[string, string]) Map[string, string] {
	out := h
	if c := Map_get(tk, h, "Connection"); c.Some {
		for _, f := range strings.Split(c.V, ",") {
			if n := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(f)); n != "" {
				Map_remove(tk, &out, n)
			}
		}
	}
	for _, n := range hopHeaders {
		Map_remove(tk, &out, n)
	}
	return out
}

func joiningSlash(a, b string) string {
	as, bs := strings.HasSuffix(a, "/"), strings.HasPrefix(b, "/")
	switch {
	case as && bs:
		return a + b[1:]
	case !as && !bs:
		return a + "/" + b
	}
	return a + b
}

// Http_ReverseProxy is `http.ReverseProxy`.
type Http_ReverseProxy struct {
	c       Http_Client
	target  Http_Url
	rewrite func(*Task, Http_Request, Http_Request) Result[Http_Request, Never]
}

// XoDebug implements Debug.
func (Http_ReverseProxy) XoDebug(b *strings.Builder) { b.WriteString("<reverse proxy>") }

// Http_reverse_proxy is `http.reverse_proxy(client, target)`.
func Http_reverse_proxy(tk *Task, client Http_Client, target Http_Url) Http_ReverseProxy {
	return Http_ReverseProxy{c: client.M_with_redirects(tk, 0), target: target}
}

// M_with_rewrite is `p.with_rewrite(f)`.
func (p Http_ReverseProxy) M_with_rewrite(_ *Task, f func(*Task, Http_Request, Http_Request) Result[Http_Request, Never]) Http_ReverseProxy {
	p.rewrite = f
	return p
}

// M_serve is `p.serve(req, body)`.
func (p Http_ReverseProxy) M_serve(tk *Task, req Http_Request, body Http_BodyReader) Http_Response {
	c, target := p.c, p.target
	u := target
	u.F_path = joiningSlash(target.F_path, req.F_url.F_path)
	q := target.F_query
	for k, v := range Map_All(req.F_url.F_query) {
		Map_put(tk, &q, k, v)
	}
	u.F_query = q
	h := withoutHop(tk, req.F_headers)
	// SetXForwarded after the incoming X-Forwarded-* are dropped
	// (Rewrite's rule)
	for _, n := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		Map_remove(tk, &h, n)
	}
	Map_put(tk, &h, "X-Forwarded-For", req.F_remote_ip)
	if host := Map_get(tk, req.F_headers, "Host"); host.Some {
		Map_put(tk, &h, "X-Forwarded-Host", host.V)
	}
	proto := req.F_url.F_scheme
	if proto == "" {
		proto = "http"
	}
	Map_put(tk, &h, "X-Forwarded-Proto", proto)
	Map_remove(tk, &h, "Host")
	out := req
	out.F_url = u
	out.F_headers = h
	out.XFields = nil
	out.F_body = ""
	if p.rewrite != nil {
		out = p.rewrite(tk, req, out).V
	}
	out = out.M_with_body_reader(tk, body)
	r := c.M_open(tk, out)
	if !r.Ok {
		logRecord(tk, levelError, "http: proxy error", []logField{{k: "error", v: Display(tk, r.E)}}, "")
		if r.E.Tag == 3 {
			return Http_empty(tk, 504)
		}
		return Http_empty(tk, 502)
	}
	st := r.V
	head := st.M_head(tk)
	rh := withoutHop(tk, head.F_headers)
	ct := Map_get(tk, rh, "Content-Type")
	flush := !Map_get(tk, rh, "Content-Length").Some || (ct.Some && strings.HasPrefix(ct.V, "text/event-stream"))
	resp := Http_stream(tk, head.F_status, func(tk *Task, w Http_BodyWriter) Result[Unit, IoErr] {
		defer st.M_close(tk)
		for {
			b := st.M_read(tk, 32768)
			if !b.Ok {
				return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: Display(tk, b.E)}})
			}
			if !b.V.Some {
				return Ok[Unit, IoErr](Unit{})
			}
			if e := w.M_write(tk, b.V.V); !e.Ok {
				return e
			}
			if flush {
				if e := w.M_flush(tk); !e.Ok {
					return e
				}
			}
		}
	})
	resp.F_headers = rh
	return resp
}
