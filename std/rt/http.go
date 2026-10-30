package rt

import (
	"bytes"
	"crypto/x509"
	"html"
	"io"
	"net/textproto"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// std/http (std.md 4).

// ---------------------------------------------------------------------------
// Routing (std.md 4.1)

// route is one registered handler.
type route struct {
	method string // "" matches any method
	segs   []routeSeg
	h      func(tk *Task, req Http_Request) Http_Response
	// arena is set when internal/gen proved that the handler confines its
	// request (Http_Router_arenaLast), so it may get arena memory.
	arena bool
	// stream: a handle_stream route (decision 0310), called before the
	// body is read
	stream bool
}

type routeSeg struct {
	lit   string
	param string
	isInt bool
	rest  bool
}

// middleware is one registered middleware, its error already mapped to a
// response (std.md 4.2).
type middleware = func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never]

// Http_Router is `http.Router`: a value; handle, wrap, and mount take a
// `var` receiver.
type Http_Router struct {
	routes List[*route]
	mws    List[middleware]
	// the not_found and method_not_allowed hooks (std.md 4.1); nil
	// answers a miss with the default JSON
	miss  func(*Task, Http_Request) Http_Response
	allow func(*Task, Http_Request) Http_Response
}

// Http_Router_not_found is `r.not_found(h)` (std.md 4.1).
func Http_Router_not_found[E any](_ *Task, r *Http_Router, h func(*Task, Http_Request) Result[Http_Response, E]) {
	r.miss = hookOf(h)
}

// Http_Router_method_not_allowed is `r.method_not_allowed(h)` (std.md
// 4.1): the response gets Allow when it lacks it.
func Http_Router_method_not_allowed[E any](_ *Task, r *Http_Router, h func(*Task, Http_Request) Result[Http_Response, E]) {
	r.allow = hookOf(h)
}

// hookOf is h with its error as a response, as a route's handler.
func hookOf[E any](h func(*Task, Http_Request) Result[Http_Response, E]) func(*Task, Http_Request) Http_Response {
	return func(tk *Task, req Http_Request) Http_Response {
		res := h(tk, req)
		if res.Ok {
			return res.V
		}
		return errorResponse(tk, any(res.E), res.Tr)
	}
}

// withAllow is resp with the Allow header when it has none (a 405's).
func withAllow(tk *Task, resp Http_Response, allow string) Http_Response {
	if Map_get(tk, resp.F_headers, "Allow").Some {
		return resp
	}
	h := resp.F_headers
	Map_put(tk, &h, "Allow", allow)
	resp.F_headers = h
	return resp
}

// XoDebug implements Debug.
func (r Http_Router) XoDebug(b *strings.Builder) { b.WriteString("<router>") }

// Http_router is `http.router()`.
func Http_router(*Task) Http_Router { return Http_Router{} }

// parsePattern parses "[METHOD ]/path/{name}/{id:Int}/{rest...}".
func parsePattern(p string) (string, []routeSeg) {
	method := ""
	if i := strings.IndexByte(p, ' '); i > 0 {
		method, p = strings.ToUpper(p[:i]), strings.TrimSpace(p[i+1:])
	}
	var segs []routeSeg
	for _, s := range splitPath(p) {
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			inner := s[1 : len(s)-1]
			switch {
			case strings.HasSuffix(inner, "..."):
				segs = append(segs, routeSeg{param: strings.TrimSuffix(inner, "..."), rest: true})
			case strings.HasSuffix(inner, ":Int"):
				segs = append(segs, routeSeg{param: strings.TrimSuffix(inner, ":Int"), isInt: true})
			default:
				segs = append(segs, routeSeg{param: inner})
			}
			continue
		}
		segs = append(segs, routeSeg{lit: s})
	}
	return method, segs
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// appendPathParts is splitPath appending to dst: dispatch splits the
// path once into a stack array for all routes.
func appendPathParts(dst []string, p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return dst
	}
	for {
		i := strings.IndexByte(p, '/')
		if i < 0 {
			return append(dst, p)
		}
		dst = append(dst, p[:i])
		p = p[i+1:]
	}
}

// match matches a path against a route: the parameters, whether an Int
// segment failed to parse (its name), and whether it matched at all. The
// parameter table is arena memory when a is not nil.
func (r *route) match(a *Arena, path string) (Map[string, string], string, bool) {
	var buf [16]string
	return r.matchParts(a, appendPathParts(buf[:0], path), true)
}

// matchParts is match on the split path. Without bind it only decides
// whether the route matches and builds no parameter table: dispatch binds
// the one route it picks.
func (r *route) matchParts(a *Arena, parts []string, bind bool) (Map[string, string], string, bool) {
	var params Map[string, string]
	put := func(k, v string) {
		if !bind {
			return
		}
		if params.t == nil {
			if a != nil {
				params = a.strMap()
			} else {
				params = mapCap[string, string](r.params())
			}
		}
		Map_put(nil, &params, k, v)
	}
	badInt := ""
	for i, s := range r.segs {
		if s.rest {
			rest := ""
			if i < len(parts) {
				rest = strings.Join(parts[i:], "/")
			}
			put(s.param, rest)
			return params, badInt, true
		}
		if i >= len(parts) {
			return params, "", false
		}
		part, _ := url.PathUnescape(parts[i])
		if s.param == "" {
			if part != s.lit {
				return params, "", false
			}
			continue
		}
		if s.isInt {
			if _, err := strconv.ParseInt(part, 10, 64); err != nil && badInt == "" {
				badInt = s.param
			}
		}
		put(s.param, part)
	}
	if len(parts) != len(r.segs) {
		return params, "", false
	}
	return params, badInt, true
}

// params counts the parameter segments.
func (r *route) params() int {
	n := 0
	for _, s := range r.segs {
		if s.param != "" {
			n++
		}
	}
	return n
}

// literals counts literal segments, for precedence.
func (r *route) literals() int {
	n := 0
	for _, s := range r.segs {
		if s.param == "" {
			n++
		}
	}
	return n
}

// Http_Router_handle is `r.handle(pattern, h)` (std.md 4.1). A handler's
// error is mapped to a response (std.md 4.2) and a fault is isolated: 500.
func Http_Router_handle[E any](_ *Task, r *Http_Router, pattern string, h func(*Task, Http_Request) Result[Http_Response, E]) {
	method, segs := parsePattern(pattern)
	rt := &route{method: method, segs: segs}
	rt.h = func(tk *Task, req Http_Request) Http_Response {
		res := h(tk, req)
		if res.Ok {
			return res.V
		}
		return errorResponse(tk, any(res.E), res.Tr)
	}
	List_push(nil, &r.routes, rt)
}

// Http_Router_handle_stream is `r.handle_stream(pattern, h)` (decision
// 0310): h reads the request body through a BodyReader; the server calls
// it before reading the body (http_net.go), Router.call streams req.body.
func Http_Router_handle_stream[E any](_ *Task, r *Http_Router, pattern string, h func(*Task, Http_Request, Http_BodyReader) Result[Http_Response, E]) {
	method, segs := parsePattern(pattern)
	rt := &route{method: method, segs: segs, stream: true}
	rt.h = func(tk *Task, req Http_Request) Http_Response {
		res := h(tk, req, bodyReaderOf(req))
		if res.Ok {
			return res.V
		}
		return errorResponse(tk, any(res.E), res.Tr)
	}
	List_push(nil, &r.routes, rt)
}

// errorResponse maps a handler error (std.md 4.2).
func errorResponse(tk *Task, e any, tr *Trace) Http_Response {
	if se, ok := e.(Http_StatusError); ok {
		return jsonError(tk, se.M_status(tk), se.M_display(tk))
	}
	if d, ok := e.(Dyn); ok {
		if se, ok := d.V.(Http_StatusError); ok {
			return jsonError(tk, se.M_status(tk), se.M_display(tk))
		}
	}
	fields := []logField{{k: "error", v: Display(tk, e)}}
	for i, l := range tr.Lines() {
		fields = append(fields, logField{k: "frame" + strconv.Itoa(i), v: strings.TrimSpace(l)})
	}
	logRecord(tk, levelError, "handler failed", fields, "")
	return jsonError(tk, 500, "internal error")
}

func jsonError(tk *Task, status int64, msg string) Http_Response {
	m := mapCap[string, string](1)
	Map_put(tk, &m, "error", msg)
	return Http_json(tk, status, m)
}

// Http_Router_wrap is `r.wrap(m)`: middleware run in registration order,
// outermost first (std.md 4.1). An error returned by m is mapped to a
// response like a handler's (std.md 4.2); the middleware registered
// before it see that response.
func Http_Router_wrap[E any](_ *Task, r *Http_Router, m func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, E]) {
	List_push(nil, &r.mws, func(tk *Task, req Http_Request, next func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
		res := m(tk, req, next)
		if res.Ok {
			return Ok[Http_Response, Never](res.V)
		}
		return Ok[Http_Response, Never](errorResponse(tk, any(res.E), res.Tr))
	})
}

// M_mount is `r.mount(prefix, sub)` (std.md 4.1): every route of sub is
// added to r under prefix, and runs behind the middleware of sub (inside
// the middleware of r). Unmatched requests never reach the middleware of
// sub. The routes are copied: later changes to sub do not affect r.
func (r *Http_Router) M_mount(_ *Task, prefix string, sub Http_Router) {
	_, pre := parsePattern(prefix)
	mws := append([]middleware(nil), sub.mws.s...)
	for _, sr := range sub.routes.s {
		nr := &route{method: sr.method, segs: append(append([]routeSeg(nil), pre...), sr.segs...), arena: sr.arena && len(mws) == 0, stream: sr.stream}
		nr.h = sr.h
		if len(mws) > 0 {
			// the middleware sees the handler's response and may keep it
			nr.h = noArenaResp(chain(mws, sr.h))
		}
		List_push(nil, &r.routes, nr)
	}
}

// chain runs inner behind mws, outermost first.
func chain(mws []middleware, inner func(*Task, Http_Request) Http_Response) func(*Task, Http_Request) Http_Response {
	next := func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		return Ok[Http_Response, Never](inner(tk, req))
	}
	for i := len(mws) - 1; i >= 0; i-- {
		mw, in := mws[i], next
		next = func(tk *Task, req Http_Request) Result[Http_Response, Never] {
			return mw(tk, req, in)
		}
	}
	return func(tk *Task, req Http_Request) Http_Response { return next(tk, req).V }
}

// M_call is `r.call(req)`: in process dispatch. A streamed body is
// collected into the response. The response goes back to Xo code, so no
// arena response may be built while it runs (see Arena).
func (r Http_Router) M_call(tk *Task, req Http_Request) Http_Response {
	return noArenaResp(func(tk *Task, req Http_Request) Http_Response {
		return r.call(tk, req).collect(tk)
	})(tk, req)
}

// noArenaResp wraps h so that no arena response is built while it runs:
// its response goes to Xo code (in process dispatch) or through
// middleware, which may keep it past the request (see Arena; decision
// 0066, hole 3).
func noArenaResp(h func(*Task, Http_Request) Http_Response) func(*Task, Http_Request) Http_Response {
	return func(tk *Task, req Http_Request) Http_Response {
		if a := tk.arena; a != nil && a.resp {
			a.resp = false
			defer func() { a.resp = true }()
		}
		return h(tk, req)
	}
}

// call dispatches through the middleware without collecting a stream.
func (r Http_Router) call(tk *Task, req Http_Request) Http_Response {
	if len(r.mws.s) == 0 {
		return r.dispatch(tk, req)
	}
	// a fault in the middleware is isolated as a handler's (std.md 4.2)
	return isolateHandler(tk, func(tk *Task) Http_Response { return chain(r.mws.s, r.dispatch)(tk, req) })
}

// allowed is the methods of the routes that match the path parts, for a
// 405's Allow (Go 1.22's ServeMux): registration order, no repeats.
func (r Http_Router) allowed(parts []string) []string {
	var out []string
	for _, rt := range r.routes.s {
		if _, _, ok := rt.matchParts(nil, parts, false); !ok || rt.method == "" {
			continue
		}
		seen := false
		for _, m := range out {
			if m == rt.method {
				seen = true
			}
		}
		if !seen {
			out = append(out, rt.method)
		}
	}
	return out
}

// find is the route of method and the path parts, the first Int
// segment of it that does not parse, and whether a route matched the
// path with another method.
func (r Http_Router) find(method string, parts []string) (best *route, bestBad string, methodMismatch bool) {
	method = strings.ToUpper(method)
	for _, rt := range r.routes.s {
		_, bad, ok := rt.matchParts(nil, parts, false)
		if !ok {
			continue
		}
		if rt.method != "" && rt.method != method {
			methodMismatch = true
			continue
		}
		if best == nil || rt.literals() > best.literals() {
			best, bestBad = rt, bad
		}
	}
	return best, bestBad, methodMismatch
}

// streams reports whether a request's route is a handle_stream route
// (decision 0310): the server then leaves its body unread.
func (r Http_Router) streams(method, path string) bool {
	var buf [16]string
	best, bad, _ := r.find(method, appendPathParts(buf[:0], path))
	return best != nil && best.stream && bad == ""
}

// dispatch finds the route and runs it isolated.
func (r Http_Router) dispatch(tk *Task, req Http_Request) Http_Response {
	var buf [16]string
	parts := appendPathParts(buf[:0], req.F_path)
	best, bestBad, methodMismatch := r.find(req.F_method, parts)
	if best == nil {
		if methodMismatch {
			var resp Http_Response
			if r.allow == nil {
				resp = jsonError(tk, 405, "method not allowed")
			} else {
				h := r.allow
				resp = isolateHandler(tk, func(tk *Task) Http_Response { return h(tk, req) })
			}
			return withAllow(tk, resp, strings.Join(r.allowed(parts), ", "))
		}
		if r.miss == nil {
			return jsonError(tk, 404, "not found")
		}
		h := r.miss
		return isolateHandler(tk, func(tk *Task) Http_Response { return h(tk, req) })
	}
	if bestBad != "" {
		return jsonError(tk, 400, "path segment `"+bestBad+"` must be an Int")
	}
	req.XParams, _, _ = best.matchParts(tk.arena, parts, true)
	if tk.arena != nil && !best.arena {
		// the handler may keep the request: no arena memory for it
		req = req.heapCopy()
	}
	return isolateHandler(tk, func(tk *Task) Http_Response { return best.h(tk, req) })
}

// isolateHandler turns a fault in a handler into a 500 (std.md 4.2).
func isolateHandler(tk *Task, f func(*Task) Http_Response) (resp Http_Response) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if IsUnwind(r) {
			panic(r)
		}
		if _, ok := r.(*exitUnwind); ok {
			panic(r)
		}
		fault := AsFault(r)
		logRecord(tk, levelError, "handler fault", []logField{{k: "fault", v: fault.Error()}}, "")
		isolatedCrash(tk, fault)
		resp = jsonError(tk, 500, "internal error")
	}()
	return f(tk)
}

// ---------------------------------------------------------------------------
// Requests and responses (std.md 4.3)

// M_param is `req.param(name)`: faults if the pattern has no such segment.
func (r Http_Request) M_param(_ *Task, name string) string {
	v := Map_get(nil, r.XParams, name)
	if !v.Some {
		FaultHere("request has no path parameter `%s`", name)
	}
	return v.V
}

// M_param_int is `req.param_int(name)`.
func (r Http_Request) M_param_int(tk *Task, name string) int64 {
	s := r.M_param(tk, name)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		FaultHere("path parameter `%s` is not an Int: %q (declare it as {%s:Int})", name, s, name)
	}
	return n
}

// M_query is `req.query(name)`.
func (r Http_Request) M_query(_ *Task, name string) Option[string] {
	return Map_get(nil, r.F_url.F_query, name)
}

// M_header is `req.header(name)`: the canonical name's value, then, when the
// lookup name is a token, the case-insensitive scan of the stored names
// (std.md 4.3). A name with a byte outside a token matches only exactly.
func (r Http_Request) M_header(_ *Task, name string) Option[string] {
	return headerGet(r.F_headers, name)
}

func headerGet(h Map[string, string], name string) Option[string] {
	if v := Map_get(nil, h, textproto.CanonicalMIMEHeaderKey(name)); v.Some {
		return v
	}
	if !isTokenName(name) {
		return Option[string]{}
	}
	for k, v := range Map_All(h) {
		if strings.EqualFold(k, name) {
			return Some(v)
		}
	}
	return Option[string]{}
}

// isTokenName is httpguts.ValidHeaderFieldName: one or more token bytes.
func isTokenName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '!' || c >= '#' && c <= '\'' || c == '*' || c == '+' || c == '-' ||
			c == '.' || c == '^' || c == '_' || c == '`' || c == '|' || c == '~'
		if !ok {
			return false
		}
	}
	return true
}

// M_with_header is `req.with_header(name, value)`: a copy of the request
// with the header set (canonical key).
func (r Http_Request) M_with_header(tk *Task, name, value string) Http_Request {
	k := textproto.CanonicalMIMEHeaderKey(name)
	r.F_headers.XoMark() // a copy (decision 0335)
	Map_put(tk, &r.F_headers, k, value)
	r.XFields = withoutField(r.XFields, k)
	return r
}

// Http_Key is `http.Key[T]` (std.md 4.3): a typed request value key. Two
// keys with the same name and type are the same key.
type Http_Key[T any] struct{ name string }

// XoDebug implements Debug.
func (k Http_Key[T]) XoDebug(b *strings.Builder) { b.WriteString("<key " + k.name + ">") }

// XoEq implements Eq.
func (k Http_Key[T]) XoEq(x any) bool { o, ok := x.(Http_Key[T]); return ok && o.name == k.name }

// XoHash implements Hash.
func (k Http_Key[T]) XoHash() uint64 { return Hash(k.name) }

// M_name is `k.name()`.
func (k Http_Key[T]) M_name(*Task) string { return k.name }

// Http_key is `http.key[T](name)`.
func Http_key[T any](_ *Task, name string) Http_Key[T] { return Http_Key[T]{name: name} }

// reqValues is the persistent list of a request's typed values: with_value
// prepends, so copies of a request never see each other's later values.
type reqValues struct {
	name string
	t    reflect.Type
	v    any
	next *reqValues
}

// Http_Request_with_value is `req.with_value(k, v)`: a copy of the request
// carrying v under k, replacing an earlier value of k. Values are not part
// of the request's equality, debug text, or anything sent on the network.
func Http_Request_with_value[T any](_ *Task, r Http_Request, k Http_Key[T], v T) Http_Request {
	r.XValues = &reqValues{name: k.name, t: reflect.TypeFor[T](), v: v, next: r.XValues}
	return r
}

// Http_Request_value is `req.value(k)`: the latest value set under k, or
// None.
func Http_Request_value[T any](_ *Task, r Http_Request, k Http_Key[T]) Option[T] {
	t := reflect.TypeFor[T]()
	for n := r.XValues; n != nil; n = n.next {
		if n.name == k.name && n.t == t {
			return Some(n.v.(T))
		}
	}
	return Option[T]{}
}

// Http_Request_json is `req.json[T]()`.
func Http_Request_json[T any](tk *Task, r Http_Request) Result[T, Json_DecodeErr] {
	return Json_decode[T](tk, r.F_body)
}

// pathKeepInvalid is a decoded URL path as a Str can hold it: every byte that
// is not part of valid UTF-8 stays its %XX escape (the native new_request's
// url_parts decodes the same way).
func pathKeepInvalid(p string) string {
	if utf8.ValidString(p) {
		return p
	}
	var b strings.Builder
	for i := 0; i < len(p); {
		r, n := utf8.DecodeRuneInString(p[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(p[i])+256, 16)[1:]))
		} else {
			b.WriteString(p[i : i+n])
		}
		i += n
	}
	return b.String()
}

// Http_new_request is `http.new_request(method, url, headers, body)`:
// remote_ip is "127.0.0.1".
func Http_new_request(tk *Task, method, rawURL string, headers Map[string, string], body Bytes) Http_Request {
	u := parseURLLoose(rawURL)
	var h Map[string, string]
	for k, v := range Map_All(headers) {
		Map_put(tk, &h, textproto.CanonicalMIMEHeaderKey(k), v)
	}
	raw := ""
	path := u.F_path
	if pu, err := url.Parse(rawURL); err == nil {
		raw = pu.RawQuery
		// the request's path is Go's URL.Path (its escaped form when the decoded
		// bytes are not UTF-8, which a Str cannot hold), "" for a URL with none
		// or an opaque one (url.path of the loose parse says "/" for the
		// first), as the native new_request's
		path = pathKeepInvalid(pu.Path)
	}
	return Http_Request{F_method: strings.ToUpper(method), F_path: path, F_url: u, F_headers: h, F_body: body, F_remote_ip: "127.0.0.1", XRawQuery: raw}
}

// M_query_all is `req.query_all(name)` (decision 0279): Go's
// URL.Query()[name]; a request built as a struct literal has no raw
// query, so its url.query value.
func (r Http_Request) M_query_all(_ *Task, name string) List[string] {
	if r.XRawQuery == "" {
		if v := Map_get(nil, r.F_url.F_query, name); v.Some {
			return ListOf(v.V)
		}
		return ListOf[string]()
	}
	q, _ := url.ParseQuery(r.XRawQuery)
	var out []string
	for k, vs := range q {
		if urlText(k) == name {
			for _, v := range vs {
				out = append(out, urlText(v))
			}
		}
	}
	return ListOf(out...)
}

// M_form is `req.form()` (decision 0279): the body of an
// application/x-www-form-urlencoded POST, PUT, or PATCH (Go's PostForm);
// the media type is std/mime's parse_media_type of the Content-Type.
func (r Http_Request) M_form(tk *Task) Result[Url_Values, Url_EscapeErr] {
	if r.F_method != "POST" && r.F_method != "PUT" && r.F_method != "PATCH" {
		return Ok[Url_Values, Url_EscapeErr](Url_Values{})
	}
	// Go's parsePostForm, through std/mime as the native code does
	ct := headerGet(r.F_headers, "Content-Type").V
	if ct == "" {
		ct = "application/octet-stream"
	}
	m := Mime_parse_media_type(tk, ct)
	if !m.Ok {
		return Err[Url_Values](Url_EscapeErr{F_msg: m.E.F_msg})
	}
	if m.V.F_kind != "application/x-www-form-urlencoded" {
		return Ok[Url_Values, Url_EscapeErr](Url_Values{})
	}
	return Url_parse_query(tk, urlText(string(r.F_body)))
}

// M_multipart_form is `req.multipart_form()` (decision 0282): net/http's
// multipartReader checks, then std/multipart's parse_form.
func (r Http_Request) M_multipart_form(tk *Task) Result[Multipart_Form, Multipart_MultipartErr] {
	notMultipart := Err[Multipart_Form](Multipart_MultipartErr{F_msg: "request Content-Type isn't multipart/form-data"})
	ct := headerGet(r.F_headers, "Content-Type").V
	if ct == "" {
		return notMultipart
	}
	m := Mime_parse_media_type(tk, ct)
	if !m.Ok || m.V.F_kind != "multipart/form-data" {
		return notMultipart
	}
	b := Map_get(tk, m.V.F_params, "boundary")
	if !b.Some {
		return Err[Multipart_Form](Multipart_MultipartErr{F_msg: "no multipart boundary param in Content-Type"})
	}
	return Multipart_parse_form(tk, r.F_body, b.V, 1000)
}

// parseURLLoose parses an absolute or a path only URL.
func parseURLLoose(raw string) Http_Url { return parseURLArena(nil, raw) }

// parseURLArena is parseURLLoose with the query table in arena a (when not
// nil).
func parseURLArena(a *Arena, raw string) Http_Url {
	pu, err := url.Parse(raw)
	if err != nil {
		return Http_Url{F_path: raw}
	}
	return urlOf(a, pu)
}

// urlOf is the Http_Url of a parsed URL, with the query table in arena a
// (when not nil). The server passes the request's own URL: printing it
// and parsing the text again cost two allocations per request.
func urlOf(a *Arena, pu *url.URL) Http_Url {
	// outside bytes: an invalid byte stays a percent escape (decision 0259)
	u := Http_Url{F_scheme: pu.Scheme, F_host: urlTextOf(pu.Hostname()), F_path: urlTextOf(pu.Path)}
	if u.F_path == "" {
		u.F_path = "/"
	}
	if p := pu.Port(); p != "" {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			u.F_port = Some(n)
		}
	}
	if pu.RawQuery == "" {
		return u
	}
	q := pu.Query()
	keys := make([]string, 0, len(q))
	val := make(map[string]string, len(q))
	for k := range q {
		// the escaped key (decision 0259) is the one sorted, as the native
		// backend sorts it
		ek := urlTextOf(k)
		keys = append(keys, ek)
		val[ek] = urlTextOf(q.Get(k))
	}
	sortStrings(keys)
	if a != nil {
		u.F_query = a.strMap()
	}
	for _, k := range keys {
		Map_put(nil, &u.F_query, k, val[k])
	}
	return u
}

// Http_parse_url is `http.parse_url(s)`: absolute http or https URLs with
// a host.
func Http_parse_url(_ *Task, s string) Result[Http_Url, Http_UrlErr] {
	pu, err := url.Parse(s)
	if err != nil {
		return Err[Http_Url](Http_UrlErr{F_msg: err.Error()})
	}
	if pu.Scheme != "http" && pu.Scheme != "https" {
		return Err[Http_Url](Http_UrlErr{F_msg: "scheme must be http or https: " + s})
	}
	if pu.Host == "" {
		return Err[Http_Url](Http_UrlErr{F_msg: "missing host: " + s})
	}
	return Ok[Http_Url, Http_UrlErr](parseURLLoose(s))
}

// Http_json is `http.json(status, v)`.
func Http_json[T any](tk *Task, status int64, v T) Http_Response {
	h := mapCap[string, string](1)
	Map_put(tk, &h, "Content-Type", "application/json")
	return Http_Response{F_status: status, F_headers: h, F_body: Json_encode(tk, v)}
}

// Http_text is `http.text(status, s)`.
func Http_text(tk *Task, status int64, s string) Http_Response {
	h := mapCap[string, string](1)
	Map_put(tk, &h, "Content-Type", "text/plain; charset=utf-8")
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(s)}
}

// Http_empty is `http.empty(status)`.
func Http_empty(_ *Task, status int64) Http_Response { return Http_Response{F_status: status} }

// M_host is `req.host()`: Go's Request.Host.
func (r Http_Request) M_host(_ *Task) string { return r.XHost }

// M_tls is `req.tls()`: whether the request came over TLS.
func (r Http_Request) M_tls(_ *Task) bool { return r.XTLS }

// tlsState is what a server connection's handshake negotiated, for
// Request.tls_state (Go's tls.ConnectionState); nil for a plain
// connection or a struct literal.
type tlsState struct {
	version, cipher, serverName string
	resumed                     bool
	peer                        []*x509.Certificate
}

// M_tls_state is `r.tls_state()`: Go's r.TLS.
func (r Http_Request) M_tls_state(tk *Task) Option[Http_TlsState] {
	s := r.XTLSState
	if s == nil {
		return None[Http_TlsState]()
	}
	var peer []X509_Certificate
	for _, c := range s.peer {
		peer = append(peer, x509CertOf(tk, c))
	}
	return Some(Http_TlsState{F_version: s.version, F_cipher_suite: s.cipher, F_server_name: s.serverName, F_resumed: s.resumed, F_peer_certificates: ListFromSlice(peer)})
}

// M_raw_query is `req.raw_query()`.
func (r Http_Request) M_raw_query(_ *Task) string { return r.XRawQuery }

// M_escaped_path is `req.escaped_path()`: Go's URL.EscapedPath.
func (r Http_Request) M_escaped_path(_ *Task) string {
	if r.XRawPath != "" {
		return r.XRawPath
	}
	return (&url.URL{Path: r.F_path}).EscapedPath()
}

// Http_redirect is `http.redirect(req, url, status)`: net/http's Redirect
// without net/http (std/rt builds with xo_nohttp too): a url with no scheme
// and no host is resolved against the request's escaped path.
func Http_redirect(tk *Task, req Http_Request, target string, status int64) Http_Response {
	if u, err := url.Parse(target); err == nil && u.Scheme == "" && u.Host == "" {
		oldpath := req.M_escaped_path(tk)
		if oldpath == "" {
			oldpath = "/"
		}
		if target == "" || target[0] != '/' {
			olddir, _ := path.Split(oldpath)
			target = olddir + target
		}
		var query string
		if i := strings.Index(target, "?"); i != -1 {
			target, query = target[:i], target[i:]
		}
		trailing := strings.HasSuffix(target, "/")
		target = path.Clean(target)
		if trailing && !strings.HasSuffix(target, "/") {
			target += "/"
		}
		target += query
	}
	h := mapCap[string, string](2)
	Map_put(tk, &h, "Location", hexEscapeNonASCII(target))
	var body Bytes
	if req.F_method == "GET" || req.F_method == "HEAD" {
		Map_put(tk, &h, "Content-Type", "text/html; charset=utf-8")
	}
	if req.F_method == "GET" {
		body = Bytes("<a href=\"" + htmlEscapeText(target) + "\">" + redirectStatusText(status) + "</a>.\n")
	}
	return Http_Response{F_status: status, F_headers: h, F_body: body}
}

// hexEscapeNonASCII is net/http's: every byte from 0x80 as %XX.
func hexEscapeNonASCII(s string) string {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			n += 3
		} else {
			n++
		}
	}
	if n == len(s) {
		return s
	}
	b := make([]byte, 0, n)
	const hexd = "0123456789abcdef"
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			b = append(b, '%', hexd[s[i]>>4], hexd[s[i]&0xF])
		} else {
			b = append(b, s[i])
		}
	}
	return string(b)
}

var htmlEscapeTextReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")

func htmlEscapeText(s string) string { return htmlEscapeTextReplacer.Replace(s) }

// redirectStatusText is net/http's StatusText for the codes a redirect
// uses (the body names the status).
func redirectStatusText(code int64) string {
	switch code {
	case 300:
		return "Multiple Choices"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 303:
		return "See Other"
	case 304:
		return "Not Modified"
	case 307:
		return "Temporary Redirect"
	case 308:
		return "Permanent Redirect"
	case 200:
		return "OK"
	case 201:
		return "Created"
	}
	return ""
}

// Http_error_page is `http.error_page(status, msg)`: Go's Error.
func Http_error_page(tk *Task, status int64, msg string) Http_Response {
	h := mapCap[string, string](2)
	Map_put(tk, &h, "Content-Type", "text/plain; charset=utf-8")
	Map_put(tk, &h, "X-Content-Type-Options", "nosniff")
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(msg + "\n")}
}

// Http_not_found is `http.not_found()`.
func Http_not_found(tk *Task) Http_Response { return Http_error_page(tk, 404, "404 page not found") }

// Http_error_html is `http.error_html(status, title, detail)` (std.md
// 4.3): an HTML page, the title in <title> and <h1>, the detail in <p>.
func Http_error_html(tk *Task, status int64, title, detail string) Http_Response {
	h := mapCap[string, string](2)
	Map_put(tk, &h, "Content-Type", "text/html; charset=utf-8")
	Map_put(tk, &h, "X-Content-Type-Options", "nosniff")
	t := html.EscapeString(title)
	body := "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>" + t + "</title></head><body><h1>" + t + "</h1><p>" + html.EscapeString(detail) + "</p></body></html>\n"
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(body)}
}

// Http_stream is `http.stream(status, f)`: the body is produced by f while
// the response is sent. Dispatched in process, the body is collected.
func Http_stream(_ *Task, status int64, f func(*Task, Http_BodyWriter) Result[Unit, IoErr]) Http_Response {
	return Http_Response{F_status: status, XStream: f}
}

// M_with_header is `resp.with_header(name, value)`.
func (r Http_Response) M_with_header(tk *Task, name, value string) Http_Response {
	k := textproto.CanonicalMIMEHeaderKey(name)
	r.F_headers.XoMark() // a copy (decision 0335)
	Map_put(tk, &r.F_headers, k, value)
	r.XFields = withoutField(r.XFields, k)
	return r
}

// M_header is `resp.header(name)`, case insensitive.
func (r Http_Response) M_header(_ *Task, name string) Option[string] {
	return headerGet(r.F_headers, name)
}

// M_text is `resp.text()`: the body as text, None if it is not valid
// UTF-8. A streamed body is collected first.
func (r Http_Response) M_text(tk *Task) Option[string] {
	return Bytes_to_str(tk, r.collect(tk).F_body)
}

// Http_Response_decode_json is `resp.decode_json[T]()`.
func Http_Response_decode_json[T any](tk *Task, r Http_Response) Result[T, Json_DecodeErr] {
	return Json_decode[T](tk, r.collect(tk).F_body)
}

// collect runs a streamed body into the response.
func (r Http_Response) collect(tk *Task) Http_Response {
	if r.XStream == nil {
		return r
	}
	var buf bytes.Buffer
	bw := Http_BodyWriter{w: &bodyWriter{w: &buf}}
	r.XStream(tk, bw)
	r.XStream = nil
	r.F_body = Bytes(buf.String())
	return r
}

type bodyWriter struct {
	mu sync.Mutex
	w  io.Writer
	fl interface{ Flush() } // net/http.Flusher
	// rw: the connection's writer, set for a 101 response's hand-off
	// (decision 0313: std/websocket hijacks it)
	rw any // net/http.ResponseWriter
	// tr sets a trailer (decision 0433; set_trailer): the response
	// writer's TrailerPrefix field on a server, nil when collected
	tr func(k, v string)
}

// M_write is `w.write(data)`.
func (w Http_BodyWriter) M_write(tk *Task, data Bytes) Result[Unit, IoErr] {
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	w.w.mu.Lock()
	defer w.w.mu.Unlock()
	if _, err := w.w.w.Write([]byte(data)); err != nil {
		return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
	}
	return Ok[Unit, IoErr](Unit{})
}

// M_flush is `w.flush()`.
func (w Http_BodyWriter) M_flush(tk *Task) Result[Unit, IoErr] {
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	if w.w.fl != nil {
		w.w.fl.Flush()
	}
	return Ok[Unit, IoErr](Unit{})
}

// ---------------------------------------------------------------------------
// Server (std.md 4.4)

// Http_Server is `http.Server`.
type Http_Server struct {
	l Listener
	r Http_Router
	// with_tls (std.md 4.6): HTTPS with these certificates
	tls   []Http_TlsCert
	tlsOn bool
	// with_read_timeout, with_read_header_timeout, with_idle_timeout,
	// with_max_header_bytes, with_max_body (std.md 4.4): net/http's
	// Server fields and MaxBytesReader on each body.
	readTimeout, readHeaderTimeout, idleTimeout time.Duration
	maxHeaderBytes, maxBody                     int64
}

func (s Http_Server) M_with_read_timeout(_ *Task, d Duration) Http_Server {
	s.readTimeout = time.Duration(d)
	return s
}

func (s Http_Server) M_with_read_header_timeout(_ *Task, d Duration) Http_Server {
	s.readHeaderTimeout = time.Duration(d)
	return s
}

func (s Http_Server) M_with_idle_timeout(_ *Task, d Duration) Http_Server {
	s.idleTimeout = time.Duration(d)
	return s
}

func (s Http_Server) M_with_max_header_bytes(_ *Task, n int64) Http_Server {
	s.maxHeaderBytes = n
	return s
}

func (s Http_Server) M_with_max_body(_ *Task, n int64) Http_Server {
	s.maxBody = n
	return s
}

// Http_max_body is `http.max_body(n)`: a middleware answering 413 with
// MaxBytesReader's text for a body over n.
func Http_max_body(tk *Task, n int64) func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
	return func(tk *Task, req Http_Request, next func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
		if int64(len(req.F_body)) > n {
			h := mapCap[string, string](2)
			Map_put(tk, &h, "Content-Type", "text/plain; charset=utf-8")
			Map_put(tk, &h, "X-Content-Type-Options", "nosniff")
			return Ok[Http_Response, Never](Http_Response{F_status: 413, F_headers: h, F_body: Bytes("http: request body too large\n")})
		}
		return next(tk, req)
	}
}

// Http_TlsCert is `http.TlsCert`: a *tls.Certificate (http_net.go; nil
// in a program built without net/http).
type Http_TlsCert struct {
	c any
}

// XoDebug implements Debug.
func (Http_TlsCert) XoDebug(b *strings.Builder) { b.WriteString("<tls cert>") }

// Http_tls_cert is `http.tls_cert(cert_pem, key_pem)`: tls.X509KeyPair.
func Http_tls_cert(_ *Task, certPEM, keyPEM Bytes) Result[Http_TlsCert, Http_TlsErr] {
	c, err := tlsKeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return Err[Http_TlsCert](Http_TlsErr{F_msg: err.Error()})
	}
	return Ok[Http_TlsCert, Http_TlsErr](Http_TlsCert{c: c})
}

// M_with_tls is `srv.with_tls(certs)`: serve speaks HTTPS.
func (s Http_Server) M_with_tls(_ *Task, certs List[Http_TlsCert]) Http_Server {
	s.tls = nil
	for c := range List_All(certs) {
		s.tls = append(s.tls, c)
	}
	s.tlsOn = true
	return s
}

// errNoTLSCert is serve's error for with_tls([]).
const errNoTLSCert = "http: with_tls needs a certificate"

// XoDebug implements Debug.
func (Http_Server) XoDebug(b *strings.Builder) { b.WriteString("<server>") }

// Http_server is `http.server(l, r)`.
func Http_server(_ *Task, l Listener, r Http_Router) Http_Server { return Http_Server{l: l, r: r} }

// M_addr is `srv.addr()`: the listener's address.
func (s Http_Server) M_addr(tk *Task) string { return s.l.M_addr(tk) }

// M_serve is `srv.serve()`: runs until a stop is requested (core 7.5) or
// the task is canceled, then stops accepting, waits for in flight
// requests, and returns Ok(()). After a stop request the wait lasts until
// the requests finish or the task is canceled (end of the grace period,
// second signal); then the remaining connections are closed and their
// request tasks canceled.
func (s Http_Server) M_serve(tk *Task) Result[Unit, Http_HttpErr] {
	if s.l.mem != nil {
		return s.l.mem.serve(tk, s.r)
	}
	if s.l.replay {
		return replayServe(tk, s)
	}
	r := s.serve(tk)
	if fxMode == fxRecord || fxMode == fxRing {
		rec.add(tk, "http.serve", []any{s.l.id}, r)
	}
	return r
}

// chunkingSize is net/http's bufferBeforeChunkingSize: a response body up
// to this size gets its Content-Length from net/http, a larger one without
// a Content-Length header goes out chunked.
const chunkingSize = 2048

// bodyAllowed reports whether a response of this status has a body (RFC
// 9110: not 1xx, 204, or 304).
func bodyAllowed(status int64) bool { return status >= 200 && status != 204 && status != 304 }

// serveCtx is what the request handler of serve needs from it, so the
// handler's locals are not captured by closures (each captured local was
// an allocation per request).
type serveCtx struct {
	s       Http_Server
	tk      *Task
	reqRoot *cnode
	sch     *sched
}

// reqBlock is a request's task, thread, cancellation node, and held
// mutex set in one allocation: they live exactly as long as the request.
type reqBlock struct {
	tk   Task
	th   thread
	n    cnode
	held heldSet
}

// start makes and starts the task of request k (0 when not recorded).
func (sc *serveCtx) start(a *Arena, k int64) *Task {
	tk := sc.tk
	b := new(reqBlock)
	t := &b.tk
	*t = Task{w: tk.w, arena: a, stop: tk.stop, th: tk.th.initChild(&b.th, "request"), held: &b.held}
	t.th.req = k
	t.node = initNode(&b.n, sc.reqRoot, tk.w, nodeRequest, t)
	if t.th.mon != nil {
		t.th.mon.started()
	}
	if sc.sch != nil {
		sc.sch.start(t.th)
	}
	return t
}

// startRecorded is start under a recording (core 11.3): the request
// reaches its handler through an event of the scheduler, which logs it.
func (sc *serveCtx) startRecorded(a *Arena, req Http_Request) (*Task, int64) {
	var t *Task
	var k int64
	started := make(chan struct{})
	sc.sch.post(schedEv{
		tok: func() string {
			k = rec.newReq(sc.s.l.id, req)
			return "r" + strconv.FormatInt(k, 10)
		},
		run: func() {
			t = sc.start(a, k)
			close(started)
		},
	})
	<-started
	return t, k
}

// ---------------------------------------------------------------------------
// Client (std.md 4.5)

// Http_Client is `http.Client`.
type Http_Client struct {
	net     Net
	timeout Duration
	// with_roots (std.md 4.6): the transport trusting those roots
	// (*http.Transport, http_net.go), nil for the shared one
	tr any
	// with_redirects (decision 0404): the redirects followed at most; 0
	// returns the 3xx itself
	redirects int64
}

// M_with_redirects is `c.with_redirects(max)` (decision 0404).
func (c Http_Client) M_with_redirects(_ *Task, max int64) Http_Client {
	if max < 0 {
		max = 0
	}
	c.redirects = max
	return c
}

// M_with_timeout is `r.with_timeout(d)` (decision 0404): the request's
// own limit for the whole exchange, in place of its client's.
func (r Http_Request) M_with_timeout(_ *Task, d Duration) Http_Request {
	if d < 0 {
		d = 0
	}
	r.XTimeout = d
	return r
}

// M_with_roots is `c.with_roots(pem)`: https trusts the certificates of
// pem instead of the system's roots.
func (c Http_Client) M_with_roots(_ *Task, pem Bytes) Result[Http_Client, Http_TlsErr] {
	tr, err := rootsTransport([]byte(pem))
	if err != nil {
		return Err[Http_Client](Http_TlsErr{F_msg: err.Error()})
	}
	c.tr = tr
	return Ok[Http_Client, Http_TlsErr](c)
}

// XoDebug implements Debug.
func (Http_Client) XoDebug(b *strings.Builder) { b.WriteString("<client>") }

// Http_client is `http.client(net, timeout)`.
func Http_client(_ *Task, n Net, timeout Duration) Http_Client {
	return Http_Client{net: n, timeout: timeout, redirects: 10}
}

func clientErr(tag int, msg string) Http_ClientErr {
	// net/http's texts quote a response's field line as received: its
	// bytes become valid UTF-8 (decision 0259), as natively
	msg = textOf(msg)
	switch tag {
	case 1:
		return Http_ClientErr{Tag: 1, P: Http_ClientErr_InvalidUrl{F_msg: msg}}
	case 2:
		return Http_ClientErr{Tag: 2, P: Http_ClientErr_Connect{F_msg: msg}}
	case 4:
		return Http_ClientErr{Tag: 4, P: Http_ClientErr_Protocol{F_msg: msg}}
	}
	return Http_ClientErr{Tag: tag}
}

// M_get is `c.get(url)`.
func (c Http_Client) M_get(tk *Task, rawURL string) Result[Http_Response, Http_ClientErr] {
	return c.M_send(tk, Http_new_request(tk, "GET", rawURL, Map[string, string]{}, ""))
}

// Http_Client_post_json is `c.post_json(url, v)`.
func Http_Client_post_json[T any](tk *Task, c Http_Client, rawURL string, v T) Result[Http_Response, Http_ClientErr] {
	h := mapCap[string, string](1)
	Map_put(tk, &h, "Content-Type", "application/json")
	return c.M_send(tk, Http_new_request(tk, "POST", rawURL, h, Json_encode(tk, v)))
}

// fakeDispatcher is implemented by the fake network of tests.
type fakeDispatcher interface {
	dispatch(tk *Task, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr]
	isFake() bool
}

// M_send is `c.send(req)`. Any status is a successful Response; only
// transport failures are ClientErr.
func (c Http_Client) M_send(tk *Task, req Http_Request) Result[Http_Response, Http_ClientErr] {
	if tk.Canceled() {
		return Err[Http_Response](clientErr(5, ""))
	}
	rawURL := requestURLString(req)
	if req.F_url.F_scheme == "" || req.F_url.F_host == "" {
		return Err[Http_Response](clientErr(1, "absolute URL required: "+rawURL))
	}
	if e, denied := httpDenied(c.net, req.F_url); denied {
		// an attenuated Net (Net.only, decision 0156)
		return Err[Http_Response](e)
	}
	if fd, ok := c.net.(fakeDispatcher); ok && fd.isFake() {
		r := fd.dispatch(tk, req, rawURL)
		if r.Ok {
			r.V = r.V.collect(tk)
		}
		return r
	}
	return fx(tk, "http.send", true, func() []any { return []any{req.F_method, rawURL} }, func() Result[Http_Response, Http_ClientErr] { return c.send(tk, req, rawURL) })
}

// requestURLString is the request's URL with its raw query as given (Go's
// Client sends URL.RawQuery verbatim) while url.query still says what it
// does, else urlString's query built from url.query.
func requestURLString(req Http_Request) string {
	if req.XRawQuery == "" {
		return urlString(req.F_url)
	}
	pu := &url.URL{RawQuery: req.XRawQuery}
	want := urlOf(nil, pu).F_query
	if !EqAny(want, req.F_url.F_query) {
		return urlString(req.F_url)
	}
	u := req.F_url
	u.F_query = Map[string, string]{}
	return urlString(u) + "?" + req.XRawQuery
}

// urlString renders a parsed URL.
func urlString(u Http_Url) string {
	var b strings.Builder
	if u.F_scheme != "" {
		b.WriteString(u.F_scheme + "://")
	}
	b.WriteString(u.F_host)
	if u.F_port.Some {
		b.WriteString(":" + strconv.FormatInt(u.F_port.V, 10))
	}
	b.WriteString(u.F_path)
	first := true
	for k, v := range Map_All(u.F_query) {
		if first {
			b.WriteByte('?')
			first = false
		} else {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k) + "=" + url.QueryEscape(v))
	}
	return b.String()
}

// sortStrings sorts in place (insertion sort; small inputs).
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}
