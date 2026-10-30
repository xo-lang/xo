package rt

import (
	"io"
	"strings"
	"sync"
)

// Full duplex streams (decision 0433): Response.with_full_duplex (Go's
// ResponseController.EnableFullDuplex) and http.pipe (Go's io.Pipe) for a
// request body written while the response is read. No net/http here.

// M_with_full_duplex is `resp.with_full_duplex()`: the stream's function
// first asks the server for full duplex (bodyWriter.fd), so it may read
// the request body while it writes.
func (r Http_Response) M_with_full_duplex(_ *Task) Http_Response {
	inner := r.XStream
	if inner == nil {
		return r
	}
	r.XStream = func(tk *Task, w Http_BodyWriter) Result[Unit, IoErr] {
		if w.w.fd != nil {
			w.w.fd()
		}
		return inner(tk, w)
	}
	return r
}

// Http_PipeWriter is `http.PipeWriter`: the writing end of http.pipe.
type Http_PipeWriter struct {
	w *io.PipeWriter
	h *pipeHold
}

// pipeHold keeps the pieces written before the reader's first read
// (decision 0444): io.Pipe's Write waits for a reader, so writing and
// then opening the request in one task never returned.
type pipeHold struct {
	mu      sync.Mutex
	started bool
	closed  bool
	held    [][]byte
}

// heldReader is the pipe's reading end: the held pieces first, then
// io.Pipe's.
type heldReader struct {
	h *pipeHold
	r *io.PipeReader
}

func (r heldReader) Read(p []byte) (int, error) {
	r.h.mu.Lock()
	r.h.started = true
	if len(r.h.held) > 0 {
		n := copy(p, r.h.held[0])
		if n < len(r.h.held[0]) {
			r.h.held[0] = r.h.held[0][n:]
		} else {
			r.h.held = r.h.held[1:]
		}
		r.h.mu.Unlock()
		return n, nil
	}
	r.h.mu.Unlock()
	return r.r.Read(p)
}

// XoDebug implements Debug.
func (Http_PipeWriter) XoDebug(b *strings.Builder) { b.WriteString("<pipe writer>") }

// pipeErr carries close_with_error's IoErr to the reader, which returns
// it as it was given.
type pipeErr struct{ e IoErr }

func (p pipeErr) Error() string { return p.e.M_display(nil) }

// Http_pipe is `http.pipe()`: before the reader's first read a write is
// held and returns (decision 0444); from then on each write waits until
// the reader has taken its bytes (io.Pipe), so a writer never runs ahead
// of the request.
func Http_pipe(_ *Task) Tuple2[Http_PipeWriter, Http_BodyReader] {
	pr, pw := io.Pipe()
	h := &pipeHold{}
	return Tuple2[Http_PipeWriter, Http_BodyReader]{F0: Http_PipeWriter{w: pw, h: h}, F1: Http_BodyReader{r: &bodyReader{src: heldReader{h: h, r: pr}}}}
}

// M_write is `w.write(data)`: Io("io: read/write on closed pipe") once
// either end is closed.
func (w Http_PipeWriter) M_write(tk *Task, data Bytes) Result[Unit, IoErr] {
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	if len(data) == 0 {
		return Ok[Unit, IoErr](Unit{})
	}
	w.h.mu.Lock()
	if !w.h.started {
		closed := w.h.closed
		if !closed {
			w.h.held = append(w.h.held, []byte(data))
		}
		w.h.mu.Unlock()
		if closed {
			return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: io.ErrClosedPipe.Error()}})
		}
		return Ok[Unit, IoErr](Unit{})
	}
	w.h.mu.Unlock()
	tk.leave()
	_, err := w.w.Write([]byte(data))
	tk.enter()
	if err != nil {
		return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
	}
	return Ok[Unit, IoErr](Unit{})
}

// M_close is `w.close()`: the reader's next read is None.
func (w Http_PipeWriter) M_close(_ *Task) {
	w.h.mu.Lock()
	w.h.closed = true
	w.h.mu.Unlock()
	w.w.Close()
}

// M_close_with_error is `w.close_with_error(e)`: the reader's next read
// is Err(e) (CloseWithError).
func (w Http_PipeWriter) M_close_with_error(_ *Task, e IoErr) {
	w.h.mu.Lock()
	w.h.closed = true
	w.h.mu.Unlock()
	w.w.CloseWithError(pipeErr{e})
}
