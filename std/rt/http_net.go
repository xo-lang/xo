//go:build !xo_nohttp

package rt

// The parts of std/http that use Go's net/http: the real server and
// client. A program that does not import std/http is built with the tag
// xo_nohttp (internal/build), which replaces them with http_nonet.go, so
// its binary does not link net/http and what it imports (crypto/tls,
// x509, mime, compress/gzip, idna; about 1.3 MB).

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// serve is M_serve on a real listener. Under a recording (core 11.3) a
// request reaches its handler through an event of the scheduler, which
// logs it, and the end of the serve is an event too.
func (s Http_Server) serve(tk *Task) Result[Unit, Http_HttpErr] {
	if s.l.l == nil {
		return Err[Unit](Http_HttpErr{Tag: 1, P: Http_HttpErr_Io{F_msg: "no listener"}})
	}
	if s.tlsOn && len(s.tls) == 0 {
		return Err[Unit](Http_HttpErr{Tag: 1, P: Http_HttpErr_Io{F_msg: errNoTLSCert}})
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
	sch := tk.th.sch
	recording := sch != nil && sch.rec != nil
	sc := &serveCtx{s: s, tk: tk, reqRoot: reqRoot, sch: sch}
	// the hand-over functions of 101 responses still running (decision
	// 0429): net/http's Shutdown does not track a hijacked connection, so
	// a stop waits for them here, as the native backends' srv_wait does
	var handed sync.WaitGroup
	srv := &http.Server{ReadTimeout: s.readTimeout, ReadHeaderTimeout: s.readHeaderTimeout, IdleTimeout: s.idleTimeout, MaxHeaderBytes: int(s.maxHeaderBytes), Handler: http.HandlerFunc(func(w http.ResponseWriter, hr *http.Request) {
		faultProbe(tk, hr)
		if s.maxBody > 0 {
			if hr.ContentLength > s.maxBody {
				// over with_max_body (std.md 4.4): 413 with MaxBytesReader's text
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				io.WriteString(w, "http: request body too large\n")
				return
			}
			hr.Body = http.MaxBytesReader(w, hr.Body, s.maxBody)
		}
		var a *Arena
		if useArena {
			a = getArena()
		}
		// a handle_stream route (decision 0310) reads the body itself; a
		// recording server keeps it whole for the replay
		stream := rec == nil && !recording && s.r.streams(hr.Method, urlTextOf(hr.URL.Path))
		var body Bytes
		if stream {
			// net/http drains or closes what the handler left unread
		} else if a != nil {
			body = a.readBody(hr.Body)
		} else {
			body = readBody(hr)
		}
		if !stream {
			// the body is read to its end: closing it tells net/http so,
			// which otherwise drains it again after the handler (io.CopyN
			// into io.Discard: a LimitReader and, whenever a GC emptied the
			// pool, an 8 KiB buffer per request)
			hr.Body.Close()
		}
		var h Map[string, string]
		if a != nil {
			h = a.strMap()
		} else {
			h = mapCap[string, string](len(hr.Header))
		}
		// outside bytes (decision 0259): U+FFFD in the fields, percent
		// escapes in the URL parts
		var fields []string
		for k, vs := range hr.Header {
			Map_put(nil, &h, textOf(k), textOf(fieldView(k, vs)))
			if len(vs) > 1 {
				fields = appendFields(fields, k, vs)
			}
		}
		u := urlOf(a, hr.URL)
		u.F_host = urlTextOf(hr.Host)
		if hostOnly, _, err := net.SplitHostPort(hr.Host); err == nil {
			u.F_host = urlTextOf(hostOnly)
		}
		ip := hr.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		if hr.TLS != nil && u.F_scheme == "" {
			u.F_scheme = "https"
		}
		req := Http_Request{XRawPath: hr.URL.EscapedPath(), XHost: urlTextOf(hr.Host), XTLS: hr.TLS != nil, XTLSState: tlsStateOf(hr.TLS), F_method: hr.Method, F_path: urlTextOf(hr.URL.Path), F_url: u, F_headers: h, F_body: body, F_remote_ip: ip, XRawQuery: hr.URL.RawQuery, XFields: fields}
		if stream {
			req = withStreamBody(req, hr.Body)
			streamTrailer(req, func() map[string][]string { return hr.Trailer })
		}
		// the recorder keeps what it is given past the request: never
		// arena memory (recCopy)
		recReq := req
		if rec != nil {
			recReq = recCopyReq(a, req)
		}
		var reqTask *Task
		var k int64
		switch {
		case recording:
			reqTask, k = sc.startRecorded(a, recReq)
		case fxMode == fxRing:
			k = rec.newReq(s.l.id, recReq)
			reqTask = sc.start(a, k)
		default:
			reqTask = sc.start(a, 0)
		}
		th := reqTask.th
		defer reqTask.node.detach()
		if sch != nil {
			<-th.turn
		} else {
			clearLabels()
		}
		defer endThread(th)
		if rec != nil {
			defer rec.endReq(k)
		}
		if a != nil {
			a.resp = true
		}
		resp := s.r.call(reqTask, req)
		wh := w.Header()
		for k, v := range Map_All(resp.F_headers) {
			if len(resp.XFields) == 0 {
				wh.Set(k, v)
				continue
			}
			// a name with several fields sends each (decision 0288)
			wh.Del(k)
			for _, x := range valuesOf(resp.F_headers, resp.XFields, k) {
				wh.Add(k, x)
			}
		}
		if n := len(resp.F_body); n > chunkingSize && resp.XStream == nil && bodyAllowed(resp.F_status) {
			// a body net/http cannot hold in its 2 KiB buffer would go
			// out chunked (a Fprintf per chunk); its length is known
			if _, ok := wh["Content-Length"]; !ok {
				wh["Content-Length"] = []string{strconv.Itoa(n)}
			}
		}
		if resp.F_status == 101 && resp.XStream != nil {
			// a connection hand-off (decision 0313): the stream's function
			// hijacks the connection and writes everything itself; it is
			// in flight until it returns (decision 0429)
			handed.Add(1)
			resp.XStream(reqTask, Http_BodyWriter{w: &bodyWriter{rw: w}})
			handed.Done()
			if a != nil {
				reqTask.arena = nil
				a.release()
			}
			return
		}
		w.WriteHeader(int(resp.F_status))
		if resp.XStream != nil {
			fl, _ := w.(http.Flusher)
			var out io.Writer = w
			var tee *bytes.Buffer
			if rec != nil {
				tee = &bytes.Buffer{}
				out = io.MultiWriter(w, tee)
			}
			tr := func(k, v string) { wh.Set(http.TrailerPrefix+k, v) }
			resp.XStream(reqTask, Http_BodyWriter{w: &bodyWriter{w: out, fl: fl, tr: tr}})
			if tee != nil {
				resp.XStream = nil
				resp.F_body = Bytes(tee.String())
			}
		} else {
			// net/http copies the bytes into its own buffer: HTTP/1 and
			// HTTP/2 both pass a string through a bufio.Writer whose
			// writer has no WriteString. HTTP/2 (TLS listeners only; no
			// h2c) gets a heap copy of an arena body all the same: had
			// the bytes reached its frame writer directly, a frame of a
			// reset stream could still read them after Write returned
			// (decision 0066, audit update 2026-10-08)
			body := string(resp.F_body)
			if a != nil && a.respUsed && hr.ProtoMajor != 1 {
				body = strings.Clone(body)
			}
			io.WriteString(w, body)
		}
		if rec != nil {
			rec.reqResp(k, recCopyResp(a, resp))
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
	// ended logs the end of the serve under a recording: x when the
	// requests still running are canceled
	ended := func(cancel bool) {
		if !recording {
			if cancel {
				reqRoot.cancel()
			}
			return
		}
		t := "e"
		if cancel {
			t = "x"
		}
		t += strconv.FormatInt(s.l.id, 10)
		sch.post(schedEv{tok: func() string { return t }, run: func() {
			if cancel {
				reqRoot.cancel()
			}
		}})
	}
	// TLS handshake failures and HTTP/2 protocol errors of clients are
	// not logged (decisions 0106, 0121)
	srv.ErrorLog = log.New(handshakeLogFilter{}, "", log.LstdFlags)
	errc := make(chan error, 1)
	if s.tlsOn {
		// HTTP/2 when ALPN agrees on h2, else HTTP/1.1 (decision 0121,
		// as the native backends' xo_h2.c). A plain listener speaks
		// HTTP/1 only (no h2c, TestServeNoH2C).
		srv.Protocols = new(http.Protocols)
		srv.Protocols.SetHTTP1(true)
		srv.Protocols.SetHTTP2(true)
		cfg := &tls.Config{}
		for _, c := range s.tls {
			cfg.Certificates = append(cfg.Certificates, *c.c.(*tls.Certificate))
		}
		srv.TLSConfig = cfg
		go func() { errc <- srv.ServeTLS(s.l.l, "", "") }()
	} else {
		go func() { errc <- srv.Serve(s.l.l) }()
	}
	tk.leave()
	defer tk.enter()
	select {
	case err := <-errc:
		ended(false)
		if errors.Is(err, http.ErrServerClosed) {
			return Ok[Unit, Http_HttpErr](Unit{})
		}
		return Err[Unit](Http_HttpErr{Tag: 1, P: Http_HttpErr_Io{F_msg: err.Error()}})
	case <-tk.stopDone():
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-tk.node.doneCh():
				cancel()
			case <-ctx.Done():
			}
		}()
		if srv.Shutdown(ctx) != nil || !waitHanded(ctx, &handed) {
			srv.Close()
			ended(true)
		} else {
			ended(false)
		}
		return Ok[Unit, Http_HttpErr](Unit{})
	case <-tk.node.doneCh():
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		waitHanded(ctx, &handed)
		ended(false)
		return Ok[Unit, Http_HttpErr](Unit{})
	}
}

// waitHanded waits for the hand-over functions still running (decision
// 0429) until ctx ends; false when ctx ended first.
func waitHanded(ctx context.Context, wg *sync.WaitGroup) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// readBody reads a request body (not into an arena): nothing when the
// request declares an empty body, one exact allocation when it declares
// its length, and the bytes become the Bytes value without a copy.
// tlsStateOf is a request's tls_state from Go's r.TLS, nil for plain.
func tlsStateOf(s *tls.ConnectionState) *tlsState {
	if s == nil {
		return nil
	}
	return &tlsState{version: tls.VersionName(s.Version), cipher: tls.CipherSuiteName(s.CipherSuite), serverName: s.ServerName, resumed: s.DidResume, peer: s.PeerCertificates}
}

func readBody(hr *http.Request) Bytes {
	if hr.ContentLength == 0 || hr.Body == nil || hr.Body == http.NoBody {
		return ""
	}
	var data []byte
	if n := hr.ContentLength; n > 0 && n <= 1<<20 {
		data = make([]byte, n)
		m, _ := io.ReadFull(hr.Body, data)
		data = data[:m]
	} else {
		data, _ = io.ReadAll(hr.Body)
	}
	if len(data) == 0 {
		return ""
	}
	// nothing else holds data
	return Bytes(unsafe.String(&data[0], len(data)))
}

// send is M_send over the real network.
func (c Http_Client) send(tk *Task, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	resp, cancel, _, e := c.do(tk, req, rawURL, false)
	if resp == nil {
		return Err[Http_Response](e)
	}
	defer cancel()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Err[Http_Response](clientErr(4, err.Error()))
	}
	h, fields := responseFields(tk, resp)
	return Ok[Http_Response, Http_ClientErr](Http_Response{F_status: int64(resp.StatusCode), F_headers: h, F_body: Bytes(body), XFields: fields})
}

// openNet is Client.open's exchange (decision 0404): the response's body
// to read (nil on a failure, then e), the cancel that ends it, the head.
// http_nonet.go has its stub, so a program without std/http links no
// net/http.
func (c Http_Client) openNet(tk *Task, req Http_Request, rawURL string) (io.ReadCloser, func(), Http_ResponseHead, func() map[string][]string, Http_ClientErr) {
	resp, cancel, head, e := c.do(tk, req, rawURL, true)
	if resp == nil {
		return nil, nil, head, nil, e
	}
	// net/http fills (or makes) resp.Trailer when the body ends
	return resp.Body, cancel, head, func() map[string][]string { return resp.Trailer }, e
}

// do sends req and reads the response's head: the response (nil on a
// failure, then e), the cancel that ends the exchange (after the body),
// and the head. stream (Client.open, decision 0404): the client's
// timeout bounds the head only, not the body.
func (c Http_Client) do(tk *Task, req Http_Request, rawURL string, stream bool) (resp *http.Response, cancelFn func(), head Http_ResponseHead, e Http_ClientErr) {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if resp == nil {
			cancel()
		}
	}()
	go func() {
		select {
		case <-tk.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	var body io.Reader = bytes.NewReader([]byte(req.F_body))
	if req.XSrc != nil {
		// with_body_reader (decision 0404 stage C): sent as it is read,
		// no GetBody, so never replayed
		body = req.XSrc
	}
	hr, err := http.NewRequestWithContext(ctx, req.F_method, rawURL, body)
	if err != nil {
		return nil, nil, head, clientErr(1, err.Error())
	}
	if ts, ok := req.XSrc.(*trailerSrc); ok {
		// with_trailers (decision 0433): net/http writes hr.Trailer
		// after the body, which the source fills at its end
		// with_trailers (decision 0433): the names declared in the head
		// (Go's client sends the keys of hr.Trailer as the Trailer header)
		hr.Trailer = http.Header{}
		for _, k := range ts.names {
			hr.Trailer[k] = nil
		}
		ts.hdr, ts.tk = hr.Trailer, tk
	}
	if req.XSrc != nil {
		hr.ContentLength = -1
		if v := Map_get(tk, req.F_headers, "Content-Length"); v.Some {
			if n, err := strconv.ParseInt(v.V, 10, 64); err == nil && n >= 0 {
				hr.ContentLength = n
			}
		}
	}
	for k, v := range Map_All(req.F_headers) {
		if len(req.XFields) == 0 {
			hr.Header.Set(k, v)
			continue
		}
		hr.Header.Del(k)
		for _, x := range valuesOf(req.F_headers, req.XFields, k) {
			hr.Header.Add(k, x)
		}
	}
	tr := sharedTransport
	if c.tr != nil {
		tr = c.tr.(*http.Transport)
	}
	timeout := c.timeout
	if req.XTimeout > 0 {
		// with_timeout (decision 0404)
		timeout = req.XTimeout
	}
	hc := &http.Client{Timeout: time.Duration(timeout), Transport: tr}
	var headTimer *time.Timer
	var headFired atomic.Bool
	if stream {
		// the head within the timeout; each read has its own limit
		hc.Timeout = 0
		if timeout > 0 {
			headTimer = time.AfterFunc(time.Duration(timeout), func() {
				headFired.Store(true)
				cancel()
			})
		}
	}
	o, only := c.net.(*onlyNet)
	// with_redirects (decision 0404): 0 returns the 3xx as it came;
	// a redirect must stay within an attenuated Net (decision 0156)
	hc.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if c.redirects <= 0 {
			return http.ErrUseLastResponse
		}
		// at most c.redirects followed (via holds the requests made)
		if len(via) > int(c.redirects) {
			return fmt.Errorf("stopped after %d redirects", c.redirects)
		}
		if !only {
			return nil
		}
		port := r.URL.Port()
		if port == "" {
			port = "80"
			if r.URL.Scheme == "https" {
				port = "443"
			}
		}
		if !o.admits(r.URL.Hostname(), port) {
			return errors.New("denied by Net.only: redirect to " + net.JoinHostPort(r.URL.Hostname(), port))
		}
		return nil
	}
	tk.leave()
	r, err := hc.Do(hr)
	tk.enter()
	if headTimer != nil && !headTimer.Stop() && err == nil {
		// the limit passed as the head arrived
		r.Body.Close()
		return nil, nil, head, clientErr(3, "")
	}
	if err != nil {
		var ne net.Error
		switch {
		case tk.Canceled():
			return nil, nil, head, clientErr(5, "")
		case headFired.Load() || (errors.As(err, &ne) && ne.Timeout()):
			return nil, nil, head, clientErr(3, "")
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			var oe *net.OpError
			if errors.As(ue.Err, &oe) {
				return nil, nil, head, clientErr(2, oe.Error())
			}
		}
		return nil, nil, head, clientErr(4, err.Error())
	}
	h, _ := responseFields(tk, r)
	return r, cancel, Http_ResponseHead{F_status: int64(r.StatusCode), F_headers: h}, e
}

// responseFields is resp's header as Xo sees it: each name's fields
// joined, and the names with several fields (decision 0288).
func responseFields(tk *Task, resp *http.Response) (Map[string, string], []string) {
	var h Map[string, string]
	var fields []string
	for k, vs := range resp.Header {
		// a response's fields: outside bytes (decision 0259); a repeated
		// name keeps each field (decision 0288), Set-Cookie is not joined
		Map_put(tk, &h, textOf(k), textOf(fieldView(k, vs)))
		if len(vs) > 1 {
			fields = appendFields(fields, k, vs)
		}
	}
	return h, fields
}

// handshakeLogFilter is the ErrorLog of serve: net/http's log lines on
// standard error, less "http: TLS handshake error" (a client's failed
// handshake; the native server does not log it either, decision 0106)
// and the HTTP/2 server's lines about a client's frames (protocol
// errors, a bad preface, a missing SETTINGS frame, a GOAWAY received, a
// stream handler's panic, which reset the stream; decision 0121).
type handshakeLogFilter struct{}

func (handshakeLogFilter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("http: TLS handshake error from ")) || bytes.Contains(p, []byte(" http2: ")) ||
		bytes.Contains(p, []byte(" timeout waiting for SETTINGS frames from ")) {
		return len(p), nil
	}
	return os.Stderr.Write(p)
}

// sharedTransport is the transport of clients without with_roots:
// http.DefaultTransport's settings (proxies from the environment, the
// 10 s TLS handshake timeout, pooled connections), HTTP/2 over TLS when
// the server offers it and HTTP/1.1 otherwise, as the native client
// speaks (decision 0121).
var sharedTransport = newTransport(nil)

func newTransport(roots *x509.CertPool) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	t.Protocols.SetHTTP2(true)
	if roots != nil || noMLKEM() {
		t.TLSClientConfig = &tls.Config{RootCAs: roots}
	}
	if noMLKEM() {
		t.TLSClientConfig.CurvePreferences = []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521}
	}
	return t
}

// noMLKEM is XO_TLS_MLKEM=0 (spec/std.md section 4): the client offers
// no post quantum hybrid, as the native client (std/native/llrt/tls13.xo
// client_groups) and Go's GODEBUG=tlsmlkem=0.
func noMLKEM() bool { return os.Getenv("XO_TLS_MLKEM") == "0" }

// tlsKeyPair is http.tls_cert: a *tls.Certificate.
func tlsKeyPair(certPEM, keyPEM []byte) (any, error) {
	c, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// rootsTransport is Client.with_roots: a transport trusting the
// certificates of pem only.
func rootsTransport(pem []byte) (any, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("tls: no certificates found in PEM input")
	}
	return newTransport(pool), nil
}

// faultProbe is std/native/llrt/http1.xo's fault_probe on the Go backend
// (a test hook, golden http_conn_fault): with XO_HTTP_FAULT_PROBE=1 a
// request for /xo-fault-probe/<at> faults outside the handler's
// isolation. "conn" (HTTP/1.1) and "frame" (HTTP/2) end the connection
// as the native server's isolation of a connection does: logged as
// "http: panic serving", then http.ErrAbortHandler (net/http closes
// without a response or a log line of its own); "stream" (HTTP/2) panics
// in the handler, which net/http answers by resetting the stream.
func faultProbe(tk *Task, hr *http.Request) {
	at, ok := strings.CutPrefix(hr.URL.Path, "/xo-fault-probe/")
	if !ok || os.Getenv("XO_HTTP_FAULT_PROBE") != "1" {
		return
	}
	switch {
	case hr.ProtoMajor == 2 && at == "stream":
		FaultHere("http fault probe: stream")
	case (hr.ProtoMajor == 1 && at == "conn") || (hr.ProtoMajor == 2 && at == "frame"):
		logRecord(tk, levelError, "http: panic serving", []logField{{k: "fault", v: "http fault probe: " + at}}, "")
		panic(http.ErrAbortHandler)
	}
}
