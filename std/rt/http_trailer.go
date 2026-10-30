package rt

import (
	"io"
	"net/textproto"
	"slices"
	"strings"
)

// Trailers (decision 0433): Go's TrailerPrefix on a streamed response,
// Request.Trailer and Response.Trailer. This file names no net/http
// (http.Header is a map[string][]string), so http_nonet.go builds.

// badTrailer is Go's list of fields a trailer may not carry
// (golang.org/x/net/http/httpguts badTrailer): set_trailer and
// with_trailers drop them, as net/http ignores them.
var badTrailer = map[string]bool{
	"Authorization":       true,
	"Cache-Control":       true,
	"Connection":          true,
	"Content-Encoding":    true,
	"Content-Length":      true,
	"Content-Range":       true,
	"Content-Type":        true,
	"Expect":              true,
	"Host":                true,
	"Keep-Alive":          true,
	"Max-Forwards":        true,
	"Pragma":              true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Proxy-Connection":    true,
	"Range":               true,
	"Realm":               true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Www-Authenticate":    true,
}

// trailerKey is name made canonical, false for a name a trailer may not
// carry.
func trailerKey(name string) (string, bool) {
	k := textproto.CanonicalMIMEHeaderKey(name)
	return k, k != "" && !badTrailer[k]
}

// trailerMap is h (an http.Header's map) as a Map, each name's first
// value, the names sorted so the order does not follow Go's map order.
func trailerMap(tk *Task, h map[string][]string) Map[string, string] {
	keys := make([]string, 0, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	m := mapCap[string, string](len(keys))
	for _, k := range keys {
		Map_put(tk, &m, k, h[k][0])
	}
	return m
}

// M_set_trailer is `w.set_trailer(name, value)`.
func (w Http_BodyWriter) M_set_trailer(_ *Task, name, value string) {
	k, ok := trailerKey(name)
	if !ok {
		return
	}
	w.w.mu.Lock()
	defer w.w.mu.Unlock()
	if w.w.tr != nil {
		w.w.tr(k, value)
	}
}

// M_with_trailer_names is `resp.with_trailer_names(names)`: the
// `Trailer` header, names Go forbids left out.
func (r Http_Response) M_with_trailer_names(tk *Task, names List[string]) Http_Response {
	var ks []string
	for n := range List_All(names) {
		if k, ok := trailerKey(n); ok && !slices.Contains(ks, k) {
			ks = append(ks, k)
		}
	}
	if len(ks) == 0 {
		return r
	}
	return r.M_with_header(tk, "Trailer", strings.Join(ks, ", "))
}

// M_trailers is `b.trailers()`: empty until the body has ended.
func (b Http_BodyReader) M_trailers(tk *Task) Map[string, string] {
	r := b.r
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.eof || r.trailer == nil {
		return mapCap[string, string](0)
	}
	return trailerMap(tk, r.trailer())
}

// M_trailers is `s.trailers()`: empty until the body has ended.
func (s Http_ClientStream) M_trailers(tk *Task) Map[string, string] {
	x := s.Xs
	if x == nil {
		return mapCap[string, string](0)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.done || x.trailer == nil {
		return mapCap[string, string](0)
	}
	return trailerMap(tk, x.trailer())
}

// M_with_trailers is `r.with_trailers(f)`: the body is sent from a
// trailerSrc, which asks f at its end.
func (r Http_Request) M_with_trailers(_ *Task, names List[string], f func(*Task) Result[Map[string, string], Never]) Http_Request {
	src := r.XSrc
	if src == nil {
		src = strings.NewReader(string(r.F_body))
		r.F_body = ""
	}
	var ks []string
	for n := range List_All(names) {
		if k, ok := trailerKey(n); ok && !slices.Contains(ks, k) {
			ks = append(ks, k)
		}
	}
	r.XSrc = &trailerSrc{src: src, names: ks, f: f}
	return r
}

// trailerSrc is a request body followed by trailers: at the body's end
// it asks f (on the transport's goroutine, while the request's task
// waits in the exchange) and sets the request's Trailer, which net/http
// writes after the body (HTTP/1.1 chunked, HTTP/2 a HEADERS frame).
type trailerSrc struct {
	src   io.Reader
	names []string // declared in the request's head
	f     func(*Task) Result[Map[string, string], Never]
	tk   *Task
	hdr  map[string][]string // the request's Trailer, set by Client.do
	done bool
}

func (s *trailerSrc) Read(p []byte) (int, error) {
	n, err := s.src.Read(p)
	if err == io.EOF && !s.done {
		s.done = true
		if s.hdr != nil {
			m := s.f(s.tk).V
			for k, v := range Map_All(m) {
				if ck, ok := trailerKey(k); ok && slices.Contains(s.names, ck) {
					s.hdr[ck] = []string{v}
				}
			}
		}
	}
	return n, err
}
