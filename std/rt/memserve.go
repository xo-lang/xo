package rt

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
)

// Raw connections to a served in memory listener (task #120): net/http's
// Server serves every connection its listener accepts, a WebSocket client
// writing its own handshake among them. An in memory connection is read
// and written through a task (memEnd.read and write wait on the test
// scheduler), so the HTTP/1.1 loop runs in a task of the serving scope
// instead of in net/http's connection goroutines.

// memTaskIO reads and writes end e as task tk.
type memTaskIO struct {
	e  *memEnd
	tk *Task
}

func (m memTaskIO) Read(p []byte) (int, error) { return memRead(m.e, m.tk, p) }

func (m memTaskIO) Write(p []byte) (int, error) {
	if r := m.e.write(m.tk, Bytes(p)); !r.Ok {
		return 0, memNetError(r.E)
	}
	return len(p), nil
}

// memRead is memEnd.read into p: io.EOF at the end of the stream,
// os.ErrDeadlineExceeded past a read deadline.
func memRead(e *memEnd, tk *Task, p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r := e.read(tk, int64(len(p)))
	if !r.Ok {
		return 0, memNetError(r.E)
	}
	if !r.V.Some {
		return 0, io.EOF
	}
	return copy(p, r.V.V), nil
}

// memNetError is a NetErr as the error a net.Conn would return.
func memNetError(e NetErr) error {
	if e.Tag == 3 {
		return os.ErrDeadlineExceeded
	}
	if p, ok := e.P.(NetErr_Io); ok && p.F_msg != "" {
		return errors.New(p.F_msg)
	}
	return errors.New(netClosedText)
}

// memConnWrite writes b to conn as task tk: through the task for an in
// memory end, else directly.
func memConnWrite(tk *Task, conn net.Conn, b []byte) error {
	if e, ok := conn.(*memEnd); ok {
		_, err := memTaskIO{e, tk}.Write(b)
		return err
	}
	_, err := conn.Write(b)
	return err
}

// memHijack is the hand-off a 101 response takes on a raw in memory
// connection (std/websocket hijacks bodyWriter.rw): the end itself and
// the reader holding what was read past the request.
type memHijack struct {
	e  *memEnd
	br *bufio.Reader
}

func (h memHijack) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.e, bufio.NewReadWriter(h.br, nil), nil
}

// memServeRaw starts the accept loop of served listener l in scope sc:
// each raw connection is served in a background task by serveRawMem.
func (l *memListener) memServeRaw(tk *Task, sc *Scope, r Http_Router) {
	Spawn(tk, sc, func(t *Task) Result[Unit, Never] {
		for {
			c := l.q.accept(t)
			if !c.Ok {
				return Ok[Unit, Never](Unit{})
			}
			e, ok := c.V.c.(*memEnd)
			if !ok {
				continue
			}
			Spawn(t, sc, func(t2 *Task) Result[Unit, Never] {
				serveRawMem(t2, e, r)
				return Ok[Unit, Never](Unit{})
			}, true, false)
		}
	}, true, false)
}

// serveRawMem serves the HTTP/1.1 requests of raw connection e until it
// closes, asks to close, or hands itself off (a 101 response).
func serveRawMem(tk *Task, e *memEnd, r Http_Router) {
	defer e.close()
	rw := memTaskIO{e, tk}
	br := bufio.NewReader(rw)
	for {
		hr, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		body, err := readAllBody(hr)
		if err != nil {
			return
		}
		resp := r.call(tk, rawMemRequest(hr, body, e.peer))
		if resp.F_status == 101 && resp.XStream != nil {
			// a connection hand-off (decision 0313): the stream's function
			// owns the connection until it returns
			resp.XStream(tk, Http_BodyWriter{w: &bodyWriter{rw: memHijack{e: e, br: br}}})
			return
		}
		out := resp.F_body
		if resp.XStream != nil {
			var buf bytes.Buffer
			resp.XStream(tk, Http_BodyWriter{w: &bodyWriter{w: &buf}})
			out = Bytes(buf.Bytes())
		}
		h := http.Header{}
		for k, v := range Map_All(resp.F_headers) {
			if len(resp.XFields) == 0 {
				h.Set(k, v)
				continue
			}
			for _, x := range valuesOf(resp.F_headers, resp.XFields, k) {
				h.Add(k, x)
			}
		}
		if bodyAllowed(resp.F_status) {
			h.Set("Content-Length", strconv.Itoa(len(out)))
		} else {
			out = ""
		}
		hresp := &http.Response{StatusCode: int(resp.F_status), ProtoMajor: 1, ProtoMinor: 1, Header: h, ContentLength: int64(len(out)), Body: io2nop(out), Request: hr, Close: hr.Close}
		if hresp.Write(rw) != nil || hr.Close {
			return
		}
	}
}

// readAllBody reads a request's whole body.
func readAllBody(hr *http.Request) (Bytes, error) {
	if hr.Body == nil {
		return "", nil
	}
	defer hr.Body.Close()
	b, err := io.ReadAll(hr.Body)
	return Bytes(b), err
}

// io2nop is a response body of b.
func io2nop(b Bytes) io.ReadCloser { return io.NopCloser(bytes.NewReader([]byte(b))) }

// rawMemRequest is the Http_Request of hr read on a raw connection from
// peer, as http_net.go's handler builds it.
func rawMemRequest(hr *http.Request, body Bytes, peer string) Http_Request {
	h := mapCap[string, string](len(hr.Header))
	var fields []string
	for k, vs := range hr.Header {
		Map_put(nil, &h, textOf(k), textOf(fieldView(k, vs)))
		if len(vs) > 1 {
			fields = appendFields(fields, k, vs)
		}
	}
	u := urlOf(nil, hr.URL)
	u.F_host = urlTextOf(hr.Host)
	if hostOnly, _, err := net.SplitHostPort(hr.Host); err == nil {
		u.F_host = urlTextOf(hostOnly)
	}
	ip := peer
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return Http_Request{XRawPath: hr.URL.EscapedPath(), XHost: urlTextOf(hr.Host), F_method: hr.Method, F_path: urlTextOf(hr.URL.Path), F_url: u, F_headers: h, F_body: body, F_remote_ip: ip, XRawQuery: hr.URL.RawQuery, XFields: fields}
}

// wsMemReader is br for WebSocket connection c, or, when c's connection
// is an in memory end, a reader of what br holds and then of the end as
// the task reading c (wsConn.rtk): the handshake was read as one task,
// the messages may be read as another.
func wsMemReader(c *wsConn, br *bufio.Reader) *bufio.Reader {
	e, ok := c.conn.(*memEnd)
	if !ok {
		return br
	}
	held, _ := br.Peek(br.Buffered())
	pre := append([]byte(nil), held...)
	return bufio.NewReader(io.MultiReader(bytes.NewReader(pre), wsEndReader{c: c, e: e}))
}

// wsEndReader reads end e as the task reading c.
type wsEndReader struct {
	c *wsConn
	e *memEnd
}

func (r wsEndReader) Read(p []byte) (int, error) { return memRead(r.e, r.c.rtk, p) }
