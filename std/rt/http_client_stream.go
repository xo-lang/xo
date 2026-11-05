package rt

import (
	"errors"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// A streamed response (decision 0404 stage B): Client.open's head, then
// the body read as it arrives from resp.Body.

// Http_ClientStream is `http.ClientStream`: the head and the body's
// state, shared by its copies.
type Http_ClientStream struct {
	Xhead Http_ResponseHead
	Xs    *clientStream
}

type clientStream struct {
	mu   sync.Mutex
	body io.ReadCloser
	// mem: the whole body of a FakeNet response, read from off
	mem    []byte
	off    int
	done   bool
	idle   time.Duration
	cancel func()
	timer  *time.Timer
	// trailer: the response's trailers once body has ended (net/http's
	// Response.Trailer, decision 0433)
	trailer func() map[string][]string
	// memch: a duplex FakeNet exchange's chunks (openMem), in place of
	// body; end runs once when it ends or the stream closes
	memch *Chan[Bytes]
	end   func(tk *Task)
}

// XoDebug implements Debug.
func (Http_ClientStream) XoDebug(b *strings.Builder) { b.WriteString("<client stream>") }

// M_with_idle_timeout is `r.with_idle_timeout(d)` (decision 0404).
func (r Http_Request) M_with_idle_timeout(_ *Task, d Duration) Http_Request {
	if d < 0 {
		d = 0
	}
	r.XIdle = d
	return r
}

// M_with_body_reader is `r.with_body_reader(b)` (decision 0404 stage C):
// the body read from b as it is sent, never replayed.
func (r Http_Request) M_with_body_reader(_ *Task, b Http_BodyReader) Http_Request {
	r.F_body = ""
	r.XSrc = bodyReaderSrc{b.r}
	return r
}

// bodySrc is a request body sent as it is read (Http_Request.XSrc).
type bodySrc = io.Reader

// bodyReaderSrc reads a BodyReader's source for a client request.
type bodyReaderSrc struct{ r *bodyReader }

func (s bodyReaderSrc) Read(p []byte) (int, error) {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	if s.r.eof {
		return 0, io.EOF
	}
	n, err := s.r.src.Read(p)
	if err == io.EOF {
		s.r.eof = true
	}
	return n, err
}

// ReadTk is Read through the task (readSrc).
func (s bodyReaderSrc) ReadTk(tk *Task, p []byte) (int, error) {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	if s.r.eof {
		return 0, io.EOF
	}
	n, err := readSrc(tk, s.r.src, p)
	if err == io.EOF {
		s.r.eof = true
	}
	return n, err
}

// CloseTk is Close through the task.
func (s bodyReaderSrc) CloseTk(tk *Task) { closeSrc(tk, s.r.src) }

// Close closes the BodyReader's source when it has one to close (a pipe's
// reader, a server request's body): net/http closes a request's body when
// it is done with the request. It takes no lock: a Read waiting in the
// source holds s.r.mu.
func (s bodyReaderSrc) Close() error {
	if c, ok := s.r.src.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// M_open is `c.open(req)`: the head, the body left on the stream.
func (c Http_Client) M_open(tk *Task, req Http_Request) Result[Http_ClientStream, Http_ClientErr] {
	if tk.Canceled() {
		return Err[Http_ClientStream](clientErr(5, ""))
	}
	rawURL := requestURLString(req)
	if req.F_url.F_scheme == "" || req.F_url.F_host == "" {
		return Err[Http_ClientStream](clientErr(1, "absolute URL required: "+rawURL))
	}
	if e, denied := httpDenied(c.net, req.F_url); denied {
		return Err[Http_ClientStream](e)
	}
	if fd, ok := c.net.(fakeDispatcher); ok && fd.isFake() {
		if req.XSrc != nil {
			return openMem(tk, fd, req, rawURL)
		}
		r := fd.dispatch(tk, req, rawURL)
		if !r.Ok {
			return Err[Http_ClientStream](r.E)
		}
		resp := r.V.collect(tk)
		s := &clientStream{mem: []byte(resp.F_body)}
		return Ok[Http_ClientStream, Http_ClientErr](Http_ClientStream{Xhead: Http_ResponseHead{F_status: resp.F_status, F_headers: resp.F_headers}, Xs: s})
	}
	return fx(tk, "http.open", true, func() []any { return []any{req.F_method, rawURL} }, func() Result[Http_ClientStream, Http_ClientErr] {
		body, cancel, head, trailer, e := c.openNet(tk, req, rawURL)
		if body == nil {
			return Err[Http_ClientStream](e)
		}
		idle := time.Duration(c.timeout)
		if req.XIdle > 0 {
			idle = time.Duration(req.XIdle)
		}
		s := &clientStream{body: body, idle: idle, cancel: cancel, trailer: trailer}
		// a dropped stream closes its body (the native counted handle's
		// free hook)
		runtime.SetFinalizer(s, func(x *clientStream) { x.release() })
		return Ok[Http_ClientStream, Http_ClientErr](Http_ClientStream{Xhead: head, Xs: s})
	})
}

// M_head is `s.head()`.
func (s Http_ClientStream) M_head(_ *Task) Http_ResponseHead { return s.Xhead }

// M_read is `s.read(max)`: the next bytes, None at the end; a body cut
// short is Protocol (unexpected EOF).
func (s Http_ClientStream) M_read(tk *Task, max int64) Result[Option[Bytes], Http_ClientErr] {
	if max < 1 {
		max = 32768
	}
	if tk.Canceled() {
		return Err[Option[Bytes]](clientErr(5, ""))
	}
	x := s.Xs
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.done {
		return Ok[Option[Bytes], Http_ClientErr](None[Bytes]())
	}
	if x.memch != nil {
		o := Chan_recv(tk, x.memch)
		if !o.Some {
			x.done = true
			x.endMem(tk)
			return Ok[Option[Bytes], Http_ClientErr](None[Bytes]())
		}
		return Ok[Option[Bytes], Http_ClientErr](Some(o.V))
	}
	if x.body == nil {
		if x.off >= len(x.mem) {
			x.done = true
			return Ok[Option[Bytes], Http_ClientErr](None[Bytes]())
		}
		end := min(len(x.mem), x.off+int(max))
		b := Bytes(x.mem[x.off:end])
		x.off = end
		return Ok[Option[Bytes], Http_ClientErr](Some(b))
	}
	buf := make([]byte, min(max, readMax))
	for {
		x.arm()
		tk.leave()
		n, err := x.body.Read(buf)
		tk.enter()
		fired := x.disarm()
		if n > 0 {
			if err != nil && err != io.EOF {
				// the bytes first; the error on the next read
				return Ok[Option[Bytes], Http_ClientErr](Some(Bytes(buf[:n])))
			}
			if err == io.EOF {
				x.finish()
			}
			return Ok[Option[Bytes], Http_ClientErr](Some(Bytes(buf[:n])))
		}
		if err == io.EOF {
			x.finish()
			return Ok[Option[Bytes], Http_ClientErr](None[Bytes]())
		}
		if err != nil {
			x.done = true
			x.release()
			var ne net.Error
			switch {
			case tk.Canceled():
				return Err[Option[Bytes]](clientErr(5, ""))
			case fired || (errors.As(err, &ne) && ne.Timeout()):
				return Err[Option[Bytes]](clientErr(3, ""))
			}
			return Err[Option[Bytes]](clientErr(4, err.Error()))
		}
	}
}

// M_read_all is `s.read_all(max_size)`.
func (s Http_ClientStream) M_read_all(tk *Task, maxSize int64) Result[Bytes, Http_ClientErr] {
	var out []byte
	for {
		r := s.M_read(tk, 65536)
		if !r.Ok {
			return Err[Bytes](r.E)
		}
		if !r.V.Some {
			return Ok[Bytes, Http_ClientErr](Bytes(out))
		}
		out = append(out, r.V.V...)
		if int64(len(out)) > maxSize {
			s.M_close(tk)
			return Err[Bytes](clientErr(4, "http: response body exceeds max_size ("+strconv.FormatInt(maxSize, 10)+" bytes)"))
		}
	}
}

// M_close is `s.close()`: idempotent.
func (s Http_ClientStream) M_close(tk *Task) {
	x := s.Xs
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.done = true
	x.release()
	x.endMem(tk)
}

// endMem ends a duplex FakeNet exchange once (openMem's end).
func (x *clientStream) endMem(tk *Task) {
	if f := x.end; f != nil {
		x.end = nil
		f(tk)
	}
}

// finish ends a body read to its end: the connection goes back to the
// pool (Close after EOF).
func (x *clientStream) finish() {
	x.done = true
	x.release()
}

func (x *clientStream) release() {
	if x.body != nil {
		x.body.Close()
		x.body = nil
	}
	if x.cancel != nil {
		x.cancel()
		x.cancel = nil
	}
	x.mem = nil
}

// arm starts the idle limit of one read: past it the exchange is
// cancelled and the read fails.
func (x *clientStream) arm() {
	if x.idle <= 0 || x.cancel == nil {
		return
	}
	cancel := x.cancel
	x.timer = time.AfterFunc(x.idle, cancel)
}

// disarm stops arm's timer: whether it fired.
func (x *clientStream) disarm() bool {
	if x.timer != nil {
		if !x.timer.Stop() {
			x.timer = nil
			return true
		}
		x.timer = nil
	}
	return false
}

// memHead is the head of a duplex FakeNet exchange, or its failure.
type memHead struct {
	resp   Http_Response
	err    Http_ClientErr
	failed bool
}

// memChunkWriter is the response body writer of a duplex FakeNet exchange:
// each write is a chunk for the client's stream.
type memChunkWriter struct {
	tk     *Task
	ch     *Chan[Bytes]
	closed *atomic.Bool
}

func (w memChunkWriter) Write(p []byte) (int, error) {
	if w.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	Chan_send(w.tk, w.ch, Bytes(append([]byte(nil), p...)))
	return len(p), nil
}

// openMem is Client.open under FakeNet for a request whose body is a
// source (http.pipe): the handler runs in a task with the source as its
// request body, so it answers while the sender still writes, and the head
// and the body chunks come back through channels, so the test scheduler's
// record and replay hold (issue 9, 2026-11-04).
func openMem(tk *Task, fd fakeDispatcher, req Http_Request, rawURL string) Result[Http_ClientStream, Http_ClientErr] {
	src := req.XSrc
	req = withStreamBody(req, src)
	req.XSrc = nil
	headCh := Chan_new[memHead](tk, 1)
	chunks := Chan_new[Bytes](tk, 16)
	closed := new(atomic.Bool)
	sc := fd.serveScope(tk, req)
	own := sc == nil
	if own {
		sc = NewScope(tk)
	}
	Spawn(tk, sc, func(tk *Task) Result[Unit, IoErr] {
		r := fd.dispatchRaw(tk, req, rawURL)
		if !r.Ok {
			Chan_send(tk, headCh, memHead{failed: true, err: r.E})
		} else {
			resp := r.V
			Chan_send(tk, headCh, memHead{resp: resp})
			if resp.XStream != nil {
				resp.XStream(tk, Http_BodyWriter{w: &bodyWriter{w: memChunkWriter{tk: tk, ch: chunks, closed: closed}}})
			} else if len(resp.F_body) > 0 {
				Chan_send(tk, chunks, resp.F_body)
			}
		}
		Chan_close(tk, chunks)
		return Ok[Unit, IoErr](Unit{})
	}, true, false)
	end := func(tk *Task) {
		closed.Store(true)
		for Chan_try_recv(tk, chunks).Some {
		}
		closeSrc(tk, src)
		if own {
			sc.Close(nil, true, 0)
		}
	}
	h := Chan_recv(tk, headCh)
	if !h.Some {
		end(tk)
		return Err[Http_ClientStream](clientErr(5, ""))
	}
	if h.V.failed {
		end(tk)
		return Err[Http_ClientStream](h.V.err)
	}
	s := &clientStream{memch: chunks, end: end}
	return Ok[Http_ClientStream, Http_ClientErr](Http_ClientStream{Xhead: Http_ResponseHead{F_status: h.V.resp.F_status, F_headers: h.V.resp.F_headers}, Xs: s})
}
