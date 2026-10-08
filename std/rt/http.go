package rt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
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
}

type routeSeg struct {
	lit   string
	param string
	isInt bool
	rest  bool
}

// Http_Router is `http.Router`: a value; handle and wrap take a `var`
// receiver.
type Http_Router struct {
	routes List[*route]
	mws    List[func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never]]
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

// match matches a path against a route: the parameters, whether an Int
// segment failed to parse (its name), and whether it matched at all. The
// parameter table is arena memory when a is not nil.
func (r *route) match(a *Arena, path string) (Map[string, string], string, bool) {
	parts := splitPath(path)
	var params Map[string, string]
	put := func(k, v string) {
		if params.t == nil {
			params = a.strMap()
		}
		params.M_put(nil, k, v)
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
	r.routes.M_push(nil, rt)
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
	logRecord(tk, levelError, "handler failed", fields)
	return jsonError(tk, 500, "internal error")
}

func jsonError(tk *Task, status int64, msg string) Http_Response {
	var m Map[string, string]
	m.M_put(tk, "error", msg)
	return Http_json(tk, status, m)
}

// M_wrap is `r.wrap(m)`: middleware run in registration order, outermost
// first (std.md 4.1).
func (r *Http_Router) M_wrap(_ *Task, m func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never]) {
	r.mws.M_push(nil, m)
}

// M_call is `r.call(req)`: in process dispatch. A streamed body is
// collected into the response. The response goes back to Xo code, so no
// arena response may be built while it runs (see Arena).
func (r Http_Router) M_call(tk *Task, req Http_Request) Http_Response {
	if a := tk.arena; a != nil && a.resp {
		a.resp = false
		defer func() { a.resp = true }()
	}
	return r.call(tk, req).collect(tk)
}

// call dispatches through the middleware without collecting a stream.
func (r Http_Router) call(tk *Task, req Http_Request) Http_Response {
	next := func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		return Ok[Http_Response, Never](r.dispatch(tk, req))
	}
	for i := len(r.mws.s) - 1; i >= 0; i-- {
		mw, inner := r.mws.s[i], next
		next = func(tk *Task, req Http_Request) Result[Http_Response, Never] {
			return mw(tk, req, inner)
		}
	}
	return next(tk, req).V
}

// dispatch finds the route and runs it isolated.
func (r Http_Router) dispatch(tk *Task, req Http_Request) Http_Response {
	var best *route
	var bestParams Map[string, string]
	bestBad := ""
	methodMismatch := false
	for _, rt := range r.routes.s {
		params, bad, ok := rt.match(tk.arena, req.F_path)
		if !ok {
			continue
		}
		if rt.method != "" && rt.method != strings.ToUpper(req.F_method) {
			methodMismatch = true
			continue
		}
		if best == nil || rt.literals() > best.literals() {
			best, bestParams, bestBad = rt, params, bad
		}
	}
	if best == nil {
		if methodMismatch {
			return jsonError(tk, 405, "method not allowed")
		}
		return jsonError(tk, 404, "not found")
	}
	if bestBad != "" {
		return jsonError(tk, 400, "path segment `"+bestBad+"` must be an Int")
	}
	req.XParams = bestParams
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
		logRecord(tk, levelError, "handler fault", []logField{{k: "fault", v: fault.Error()}})
		resp = jsonError(tk, 500, "internal error")
	}()
	return f(tk)
}

// ---------------------------------------------------------------------------
// Requests and responses (std.md 4.3)

// M_param is `req.param(name)`: faults if the pattern has no such segment.
func (r Http_Request) M_param(_ *Task, name string) string {
	v := r.XParams.M_get(nil, name)
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
	return r.F_url.F_query.M_get(nil, name)
}

// M_header is `req.header(name)`, case insensitive.
func (r Http_Request) M_header(_ *Task, name string) Option[string] {
	return headerGet(r.F_headers, name)
}

func headerGet(h Map[string, string], name string) Option[string] {
	if v := h.M_get(nil, textproto.CanonicalMIMEHeaderKey(name)); v.Some {
		return v
	}
	for k, v := range h.All() {
		if strings.EqualFold(k, name) {
			return Some(v)
		}
	}
	return Option[string]{}
}

// Http_Request_json is `req.json[T]()`.
func Http_Request_json[T any](tk *Task, r Http_Request) Result[T, Json_DecodeErr] {
	return Json_decode[T](tk, r.F_body)
}

// Http_new_request is `http.new_request(method, url, headers, body)`:
// remote_ip is "127.0.0.1".
func Http_new_request(tk *Task, method, rawURL string, headers Map[string, string], body Bytes) Http_Request {
	u := parseURLLoose(rawURL)
	var h Map[string, string]
	for k, v := range headers.All() {
		h.M_put(tk, textproto.CanonicalMIMEHeaderKey(k), v)
	}
	return Http_Request{F_method: strings.ToUpper(method), F_path: u.F_path, F_url: u, F_headers: h, F_body: body, F_remote_ip: "127.0.0.1"}
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
	u := Http_Url{F_scheme: pu.Scheme, F_host: pu.Hostname(), F_path: pu.Path}
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
	for k := range q {
		keys = append(keys, k)
	}
	sortStrings(keys)
	if a != nil {
		u.F_query = a.strMap()
	}
	for _, k := range keys {
		u.F_query.M_put(nil, k, q.Get(k))
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
	var h Map[string, string]
	h.M_put(tk, "Content-Type", "application/json")
	return Http_Response{F_status: status, F_headers: h, F_body: Json_encode(tk, v)}
}

// Http_text is `http.text(status, s)`.
func Http_text(tk *Task, status int64, s string) Http_Response {
	var h Map[string, string]
	h.M_put(tk, "Content-Type", "text/plain; charset=utf-8")
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(s)}
}

// Http_empty is `http.empty(status)`.
func Http_empty(_ *Task, status int64) Http_Response { return Http_Response{F_status: status} }

// Http_stream is `http.stream(status, f)`: the body is produced by f while
// the response is sent. Dispatched in process, the body is collected.
func Http_stream(_ *Task, status int64, f func(*Task, Http_BodyWriter) Result[Unit, IoErr]) Http_Response {
	return Http_Response{F_status: status, XStream: f}
}

// M_with_header is `resp.with_header(name, value)`.
func (r Http_Response) M_with_header(tk *Task, name, value string) Http_Response {
	r.F_headers.M_put(tk, textproto.CanonicalMIMEHeaderKey(name), value)
	return r
}

// M_header is `resp.header(name)`, case insensitive.
func (r Http_Response) M_header(_ *Task, name string) Option[string] {
	return headerGet(r.F_headers, name)
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
	fl http.Flusher
}

// M_write is `w.write(data)`.
func (w Http_BodyWriter) M_write(tk *Task, data Bytes) Result[Unit, IoErr] {
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	w.w.mu.Lock()
	defer w.w.mu.Unlock()
	if _, err := io.WriteString(w.w.w, string(data)); err != nil {
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
}

// XoDebug implements Debug.
func (Http_Server) XoDebug(b *strings.Builder) { b.WriteString("<server>") }

// Http_server is `http.server(l, r)`.
func Http_server(_ *Task, l Listener, r Http_Router) Http_Server { return Http_Server{l: l, r: r} }

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
	if s.l.l == nil {
		return Err[Unit](Http_HttpErr{Tag: 1, P: Http_HttpErr_Io{F_msg: "no listener"}})
	}
	// request scoped arenas (arena.go): not with middleware, which sees
	// the request and the response
	useArena := arenasOn && len(s.r.mws.s) == 0
	// request tasks hang below reqRoot, not below the serving task: a
	// canceled serve still lets them finish, unless the shutdown is cut
	// short
	reqRoot := newNodeOf(nil, tk.w, nodeServer, nil)
	// not below the serving task, so `xo dump` finds it here
	extraRoots.Store(reqRoot, true)
	defer extraRoots.Delete(reqRoot)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, hr *http.Request) {
		var a *Arena
		if useArena {
			a = getArena()
		}
		reqTask := &Task{w: tk.w, arena: a, stop: tk.stop, th: tk.th.newChild("request")}
		node := newNodeOf(reqRoot, tk.w, nodeRequest, reqTask)
		reqTask.node = node
		defer node.detach()
		th := reqTask.th
		if th.mon != nil {
			th.mon.started()
		}
		if sch := th.sch; sch != nil {
			sch.start(th)
			<-th.turn
		} else {
			clearLabels()
		}
		defer func() {
			th.mu.Lock()
			th.done = true
			th.mu.Unlock()
			if th.mon != nil {
				th.mon.ended()
			}
			if sch := th.sch; sch != nil {
				sch.end(th)
			}
		}()
		var body Bytes
		if a != nil {
			body = a.readBody(hr.Body)
		} else {
			data, _ := io.ReadAll(hr.Body)
			body = Bytes(data)
		}
		h := a.strMap()
		for k, vs := range hr.Header {
			h.M_put(reqTask, k, strings.Join(vs, ", "))
		}
		u := parseURLArena(a, hr.URL.String())
		u.F_host = hr.Host
		if hostOnly, _, err := net.SplitHostPort(hr.Host); err == nil {
			u.F_host = hostOnly
		}
		ip := hr.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		req := Http_Request{F_method: hr.Method, F_path: hr.URL.Path, F_url: u, F_headers: h, F_body: body, F_remote_ip: ip}
		if a != nil {
			a.resp = true
		}
		resp := s.r.call(reqTask, req)
		for k, v := range resp.F_headers.All() {
			w.Header().Set(k, v)
		}
		w.WriteHeader(int(resp.F_status))
		if resp.XStream != nil {
			fl, _ := w.(http.Flusher)
			resp.XStream(reqTask, Http_BodyWriter{w: &bodyWriter{w: w, fl: fl}})
		} else {
			// net/http copies the bytes into its own buffer
			io.WriteString(w, string(resp.F_body))
		}
		if a != nil {
			// the handler returned (every task it started in a scope has
			// ended) and the response is written: nothing holds arena
			// memory now (see Arena). A panic skips this; the arena is
			// then left to the garbage collector.
			reqTask.arena = nil
			a.release()
		}
	})}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(s.l.l) }()
	tk.leave()
	defer tk.enter()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return Ok[Unit, Http_HttpErr](Unit{})
		}
		return Err[Unit](Http_HttpErr{Tag: 1, P: Http_HttpErr_Io{F_msg: err.Error()}})
	case <-tk.stopDone():
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-tk.node.done:
				cancel()
			case <-ctx.Done():
			}
		}()
		if srv.Shutdown(ctx) != nil {
			srv.Close()
			reqRoot.cancel()
		}
		return Ok[Unit, Http_HttpErr](Unit{})
	case <-tk.node.done:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		return Ok[Unit, Http_HttpErr](Unit{})
	}
}

// ---------------------------------------------------------------------------
// Client (std.md 4.5)

// Http_Client is `http.Client`.
type Http_Client struct {
	net     Net
	timeout Duration
}

// XoDebug implements Debug.
func (Http_Client) XoDebug(b *strings.Builder) { b.WriteString("<client>") }

// Http_client is `http.client(net, timeout)`.
func Http_client(_ *Task, n Net, timeout Duration) Http_Client {
	return Http_Client{net: n, timeout: timeout}
}

func clientErr(tag int, msg string) Http_ClientErr {
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
	var h Map[string, string]
	h.M_put(tk, "Content-Type", "application/json")
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
	rawURL := urlString(req.F_url)
	if req.F_url.F_scheme == "" || req.F_url.F_host == "" {
		return Err[Http_Response](clientErr(1, "absolute URL required: "+rawURL))
	}
	if fd, ok := c.net.(fakeDispatcher); ok && fd.isFake() {
		r := fd.dispatch(tk, req, rawURL)
		if r.Ok {
			r.V = r.V.collect(tk)
		}
		return r
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-tk.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	hr, err := http.NewRequestWithContext(ctx, req.F_method, rawURL, bytes.NewReader([]byte(req.F_body)))
	if err != nil {
		return Err[Http_Response](clientErr(1, err.Error()))
	}
	for k, v := range req.F_headers.All() {
		hr.Header.Set(k, v)
	}
	hc := &http.Client{Timeout: time.Duration(c.timeout)}
	tk.leave()
	resp, err := hc.Do(hr)
	tk.enter()
	if err != nil {
		var ne net.Error
		switch {
		case tk.Canceled():
			return Err[Http_Response](clientErr(5, ""))
		case errors.As(err, &ne) && ne.Timeout():
			return Err[Http_Response](clientErr(3, ""))
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			var oe *net.OpError
			if errors.As(ue.Err, &oe) {
				return Err[Http_Response](clientErr(2, oe.Error()))
			}
		}
		return Err[Http_Response](clientErr(4, err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Err[Http_Response](clientErr(4, err.Error()))
	}
	var h Map[string, string]
	for k, vs := range resp.Header {
		h.M_put(tk, k, strings.Join(vs, ", "))
	}
	return Ok[Http_Response, Http_ClientErr](Http_Response{F_status: int64(resp.StatusCode), F_headers: h, F_body: Bytes(body)})
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
	for k, v := range u.F_query.All() {
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
