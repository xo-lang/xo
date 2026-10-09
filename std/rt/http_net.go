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
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, hr *http.Request) {
		var a *Arena
		if useArena {
			a = getArena()
		}
		var body Bytes
		if a != nil {
			body = a.readBody(hr.Body)
		} else {
			body = readBody(hr)
		}
		// the body is read to its end: closing it tells net/http so, which
		// otherwise drains it again after the handler (io.CopyN into
		// io.Discard: a LimitReader and, whenever a GC emptied the pool,
		// an 8 KiB buffer per request)
		hr.Body.Close()
		var h Map[string, string]
		if a != nil {
			h = a.strMap()
		} else {
			h = mapCap[string, string](len(hr.Header))
		}
		for k, vs := range hr.Header {
			Map_put(nil, &h, k, strings.Join(vs, ", "))
		}
		u := urlOf(a, hr.URL)
		u.F_host = hr.Host
		if hostOnly, _, err := net.SplitHostPort(hr.Host); err == nil {
			u.F_host = hostOnly
		}
		ip := hr.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		req := Http_Request{F_method: hr.Method, F_path: hr.URL.Path, F_url: u, F_headers: h, F_body: body, F_remote_ip: ip}
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
			wh.Set(k, v)
		}
		if n := len(resp.F_body); n > chunkingSize && resp.XStream == nil && bodyAllowed(resp.F_status) {
			// a body net/http cannot hold in its 2 KiB buffer would go
			// out chunked (a Fprintf per chunk); its length is known
			if _, ok := wh["Content-Length"]; !ok {
				wh["Content-Length"] = []string{strconv.Itoa(n)}
			}
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
			resp.XStream(reqTask, Http_BodyWriter{w: &bodyWriter{w: out, fl: fl}})
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
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(s.l.l) }()
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
		if srv.Shutdown(ctx) != nil {
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
		ended(false)
		return Ok[Unit, Http_HttpErr](Unit{})
	}
}

// readBody reads a request body (not into an arena): nothing when the
// request declares an empty body, one exact allocation when it declares
// its length, and the bytes become the Bytes value without a copy.
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
	for k, v := range Map_All(req.F_headers) {
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
		Map_put(tk, &h, k, strings.Join(vs, ", "))
	}
	return Ok[Http_Response, Http_ClientErr](Http_Response{F_status: int64(resp.StatusCode), F_headers: h, F_body: Bytes(body)})
}
