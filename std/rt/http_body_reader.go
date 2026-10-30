package rt

import (
	"io"
	"strings"
	"sync"
)

// Http_BodyReader is the body of a request to a handle_stream route
// (decision 0310).
type Http_BodyReader struct{ r *bodyReader }

// XoDebug implements Debug.
func (Http_BodyReader) XoDebug(b *strings.Builder) { b.WriteString("<body reader>") }

type bodyReader struct {
	mu  sync.Mutex
	src io.Reader
	eof bool
	// trailer: the request's trailers once src has ended (net/http's
	// Request.Trailer, decision 0433), nil when there are none
	trailer func() map[string][]string
}

// streamBody names the request value that carries the server's unread
// body (net/http's r.Body) to a handle_stream route; no Key has an empty
// type, so no Request.value finds it.
const streamBody = "\x00body"

// withStreamBody is req carrying the unread body src.
func withStreamBody(req Http_Request, src io.Reader) Http_Request {
	req.XValues = &reqValues{name: streamBody, v: &bodyReader{src: src}, next: req.XValues}
	return req
}

// streamTrailer gives req's streamed body (withStreamBody) its trailers.
func streamTrailer(req Http_Request, f func() map[string][]string) {
	for n := req.XValues; n != nil; n = n.next {
		if n.name == streamBody && n.t == nil {
			n.v.(*bodyReader).trailer = f
			return
		}
	}
}

// bodyReaderOf is the reader of req's body: the server's stream, else
// req.body (Router.call, a request made in Xo).
func bodyReaderOf(req Http_Request) Http_BodyReader {
	for n := req.XValues; n != nil; n = n.next {
		if n.name == streamBody && n.t == nil {
			return Http_BodyReader{r: n.v.(*bodyReader)}
		}
	}
	return Http_BodyReader{r: &bodyReader{src: strings.NewReader(string(req.F_body))}}
}

// readMax is the most one read asks of the source.
const readMax = 1 << 20

// M_read is `b.read(max)`: the next bytes, at most max, None at the end.
func (b Http_BodyReader) M_read(tk *Task, max int64) Result[Option[Bytes], IoErr] {
	if max < 1 {
		FaultHere("BodyReader.read: max must be at least 1, got %d", max)
	}
	if tk.Canceled() {
		return Err[Option[Bytes]](IoErr{Tag: 3})
	}
	r := b.r
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eof {
		return Ok[Option[Bytes], IoErr](None[Bytes]())
	}
	buf := make([]byte, min(max, readMax))
	for {
		tk.leave()
		n, err := r.src.Read(buf)
		tk.enter()
		if err == io.EOF {
			r.eof = true
		}
		if n > 0 {
			return Ok[Option[Bytes], IoErr](Some(Bytes(buf[:n])))
		}
		if err == io.EOF {
			return Ok[Option[Bytes], IoErr](None[Bytes]())
		}
		if pe, ok := err.(pipeErr); ok {
			// http.pipe's close_with_error: the IoErr as given
			return Err[Option[Bytes]](pe.e)
		}
		if err != nil {
			return Err[Option[Bytes]](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
		}
	}
}

// M_read_all is `b.read_all()`: the rest of the body.
func (b Http_BodyReader) M_read_all(tk *Task) Result[Bytes, IoErr] {
	if tk.Canceled() {
		return Err[Bytes](IoErr{Tag: 3})
	}
	r := b.r
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eof {
		return Ok[Bytes, IoErr]("")
	}
	tk.leave()
	data, err := io.ReadAll(r.src)
	tk.enter()
	if pe, ok := err.(pipeErr); ok {
		return Err[Bytes](pe.e)
	}
	if err != nil {
		return Err[Bytes](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
	}
	r.eof = true
	return Ok[Bytes, IoErr](Bytes(data))
}
