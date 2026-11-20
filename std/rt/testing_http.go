//go:build !xo_nohttp

package rt

import (
	"crypto/tls"
	"fmt"
	"strings"
)

type fakeHandler struct {
	method string
	prefix string // scheme://host[:port]
	r      *route
	rest   string // the path pattern as written
	h      func(*Task, Http_Request) Result[Http_Response, Never]
	// handle_tls (decision 0544): the certificate an https request is checked against
	cert *tls.Certificate
}

type fakeFail struct {
	method string
	prefix string
	r      *route
	err    Http_ClientErr
	once   bool
}

// parseFakePattern splits "[METHOD ]https://host/path/{x}".
func parseFakePattern(p string) (string, string, *route, string) {
	method := ""
	if i := strings.IndexByte(p, ' '); i > 0 && !strings.Contains(p[:i], "/") {
		method, p = strings.ToUpper(p[:i]), strings.TrimSpace(p[i+1:])
	}
	prefix := ""
	rest := p
	if i := strings.Index(p, "://"); i >= 0 {
		j := strings.IndexByte(p[i+3:], '/')
		if j < 0 {
			prefix, rest = p, "/"
		} else {
			prefix, rest = p[:i+3+j], p[i+3+j:]
		}
	}
	_, segs := parsePattern(rest)
	return method, prefix, &route{segs: segs}, rest
}

// M_handle registers a handler for "[METHOD ]<absolute url pattern>"; a second one for the same pattern replaces the first.
func (f Testing_FakeNet) M_handle(_ *Task, pattern string, h func(*Task, Http_Request) Result[Http_Response, Never]) {
	m, prefix, r, rest := parseFakePattern(pattern)
	f.n.mu.Lock()
	// a second handler for the same method, prefix and path replaces the first (last wins)
	for _, old := range f.n.handlers {
		if old.method == m && old.prefix == prefix && old.rest == rest {
			old.h = h
			f.n.mu.Unlock()
			return
		}
	}
	f.n.handlers = append(f.n.handlers, &fakeHandler{method: m, prefix: prefix, r: r, rest: rest, h: h})
	f.n.mu.Unlock()
}

// M_fail makes every matching request fail until clear_failures.
func (f Testing_FakeNet) M_fail(_ *Task, pattern string, err Http_ClientErr) {
	m, prefix, r, _ := parseFakePattern(pattern)
	f.n.mu.Lock()
	f.n.fails = append(f.n.fails, &fakeFail{method: m, prefix: prefix, r: r, err: err})
	f.n.mu.Unlock()
}

// M_fail_next makes the next matching request fail.
func (f Testing_FakeNet) M_fail_next(_ *Task, pattern string, err Http_ClientErr) {
	m, prefix, r, _ := parseFakePattern(pattern)
	f.n.mu.Lock()
	f.n.fails = append(f.n.fails, &fakeFail{method: m, prefix: prefix, r: r, err: err, once: true})
	f.n.mu.Unlock()
}

// M_requests returns every request seen, including failed ones.
func (f Testing_FakeNet) M_requests(*Task) List[Http_Request] {
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	return ListOf(append([]Http_Request(nil), f.n.reqs...)...)
}

func matchFake(method, prefix string, r *route, req Http_Request, rawURL string) (Map[string, string], bool) {
	if method != "" && method != strings.ToUpper(req.F_method) {
		return Map[string, string]{}, false
	}
	if prefix != "" && !strings.HasPrefix(rawURL, prefix) {
		return Map[string, string]{}, false
	}
	// the escaped path, split before unescaping (a client request's F_path is decoded)
	params, _, ok := r.match(nil, req.M_escaped_path(nil))
	return params, ok
}

// dispatch serves a client request in process.
func (f Testing_FakeNet) dispatch(tk *Task, c Http_Client, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	return f.dispatchWith(tk, c, req, rawURL, false)
}

// dispatchRaw is dispatch whose served router's streamed response is left
// to the caller to run (Client.open of a duplex request).
func (f Testing_FakeNet) dispatchRaw(tk *Task, c Http_Client, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	return f.dispatchWith(tk, c, req, rawURL, true)
}

// serveScope is the scope of the listener serving req's address, once its
// serve has started.
func (f Testing_FakeNet) serveScope(tk *Task, req Http_Request) *Scope {
	n := f.n
	hostport := req.F_url.F_host
	if req.F_url.F_port.Some {
		hostport += fmt.Sprint(":", req.F_url.F_port.V)
	} else if strings.EqualFold(req.F_url.F_scheme, "https") {
		hostport += ":443"
	} else {
		hostport += ":80"
	}
	n.mu.Lock()
	srv := n.listeners[hostport]
	n.mu.Unlock()
	if srv == nil {
		return nil
	}
	if !srv.served.isSet() && !srv.ready.isSet() && !srv.ready.wait(tk, true, waitAccept, nil) {
		return nil
	}
	if !srv.served.isSet() {
		return nil
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.sc
}

func (f Testing_FakeNet) dispatchWith(tk *Task, c Http_Client, req Http_Request, rawURL string, raw bool) Result[Http_Response, Http_ClientErr] {
	tk.Yield()
	n := f.n
	if req.XHost == "" {
		req.XHost = fakeAuthority(req.F_url)
	}
	n.mu.Lock()
	n.reqs = append(n.reqs, req)
	for i, fl := range n.fails {
		if _, ok := matchFake(fl.method, fl.prefix, fl.r, req, rawURL); ok {
			if fl.once {
				n.fails = append(n.fails[:i], n.fails[i+1:]...)
			}
			n.mu.Unlock()
			return Err[Http_Response](fl.err)
		}
	}
	var best *fakeHandler
	var params Map[string, string]
	for _, h := range n.handlers {
		if p, ok := matchFake(h.method, h.prefix, h.r, req, rawURL); ok {
			// Go's ServeMux picks the most specific pattern: the Router's
			// score, then a method over none (fakenetfuzz, task #233)
			if best == nil || fakeScore(h) > fakeScore(best) {
				best, params = h, p
			}
		}
	}
	var srv *memListener
	if best == nil {
		hostport := req.F_url.F_host
		if req.F_url.F_port.Some {
			hostport += fmt.Sprint(":", req.F_url.F_port.V)
		} else if strings.EqualFold(req.F_url.F_scheme, "https") {
			// the default port, as a real dial would use: a listener on
			// host:443 serves https://host/ (aws-19, 2026-11-04)
			hostport += ":443"
		} else {
			hostport += ":80"
		}
		srv = n.listeners[hostport]
	}
	n.mu.Unlock()
	if best != nil {
		if best.cert != nil && strings.EqualFold(req.F_url.F_scheme, "https") {
			why, st := fakeTLS(tk, c, req, best.cert)
			if why != "" {
				return Err[Http_Response](clientErr(2, why))
			}
			req.XTLS, req.XTLSState = true, st
		}
		req.XParams = params
		return Ok[Http_Response, Http_ClientErr](best.h(tk, req).V)
	}
	if srv != nil && !srv.served.isSet() {
		// like a real socket after listen: the connection waits in the
		// backlog until http.server(l, r).serve() starts or a task accepts
		if !srv.ready.isSet() && !srv.ready.wait(tk, true, waitAccept, nil) {
			return Err[Http_Response](clientErr(5, ""))
		}
		if !srv.served.isSet() {
			// a bare Listener: HTTP/1.1 over the fake TCP
			return f.rawExchange(tk, req, srv.addr)
		}
	}
	if srv != nil {
		if r, st, ok := srv.begin(); ok {
			defer srv.end()
			htk := tk
			if st != nil {
				// the handler sees the serving program's stop state
				htk = &Task{node: tk.node, w: tk.w, held: tk.held, shield: tk.shield, stop: st, th: tk.th}
			}
			if raw {
				return Ok[Http_Response, Http_ClientErr](noArenaResp(func(tk *Task, req Http_Request) Http_Response { return r.call(tk, req) })(htk, req))
			}
			return Ok[Http_Response, Http_ClientErr](r.M_call(htk, req))
		}
	}
	return Err[Http_Response](clientErr(2, "no fake handler for "+req.F_method+" "+rawURL))
}

// begin starts a request on the listener: it reports the router and the
// serving program's stop state, or false when nothing is served.
func (l *memListener) begin() (Http_Router, *stopState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.r == nil {
		return Http_Router{}, nil, false
	}
	l.inflight++
	return *l.r, l.stop, true
}

// serve routes requests for the listener's address to r until a stop is
// requested (then it waits for the requests in flight, core 7.5) or the
// task is canceled.
func (l *memListener) serve(tk *Task, r Http_Router) Result[Unit, Http_HttpErr] {
	l.mu.Lock()
	l.r = &r
	l.stop = tk.stop
	sc := NewScope(tk)
	l.sc = sc
	l.mu.Unlock()
	defer func() { sc.Close(nil, true, 0) }()
	// the raw connections dialed to it are served too (task #120)
	l.memServeRaw(tk, sc, r)
	l.served.fire()
	l.ready.fire()
	stopped := false
	if tk.stop != nil {
		stopped = tk.stop.ev.wait(tk, true, waitStop, nil)
	} else if !tk.Canceled() {
		// a task canceled before it got here (a background serve whose
		// scope ended first) must not block: nothing would wake it
		tk.blockOn(waitServe, nil, true)
		<-tk.node.doneCh()
		tk.unblock()
	}
	l.mu.Lock()
	l.r = nil
	var drained *event
	if stopped && l.inflight > 0 {
		drained = newEvent()
		l.drained = drained
	}
	l.mu.Unlock()
	if drained != nil {
		// cancellation (grace period, second signal) cuts the wait short
		drained.wait(tk, true, waitDrain, nil)
	}
	return Ok[Unit, Http_HttpErr](Unit{})
}

// the HTTP types of the fake network's fields (testing.go): without
// std/http (xo_nohttp, task #472) they are empty structs
type (
	fakeReq   = Http_Request
	memRouter = Http_Router
)

// fakeScore ranks a fake handler as std/native/http.xo's fake_match does:
// more literal segments, then more segments before any rest, then an
// exact path over a subtree or rest, then a method over none.
func fakeScore(h *fakeHandler) int {
	fixed := 0
	for _, s := range h.r.segs {
		if !s.rest {
			fixed++
		}
	}
	sc := (h.r.literals()*256 + fixed) * 2
	if !h.r.hasRest() {
		sc++
	}
	sc *= 2
	if h.method != "" {
		sc++
	}
	return sc
}
