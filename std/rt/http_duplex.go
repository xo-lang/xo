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
	n       int // the held bytes, at most pipeHoldMax
	// the scheduler-visible pipe of a test world (a blocking io.Pipe call
	// under the test scheduler is a deadlock fault, sched.leave): writes
	// are sends on ch, reads receives; cur is a received piece the reader
	// has not finished, rclosed the reader's close, err close_with_error's
	ch      *Chan[[]byte]
	cur     []byte
	rclosed bool
	err     error
}

// tkReader is a reader that waits through the task (scheduler visible).
type tkReader interface {
	ReadTk(tk *Task, p []byte) (int, error)
}

// tkCloser is a closer that needs the task.
type tkCloser interface {
	CloseTk(tk *Task)
}

// readSrc reads src for tk: through the task when the source can and the
// task runs under the test scheduler, else outside it (tk.leave).
func readSrc(tk *Task, src io.Reader, buf []byte) (int, error) {
	if tr, ok := src.(tkReader); ok && tk != nil && tk.th != nil && tk.th.sch != nil {
		return tr.ReadTk(tk, buf)
	}
	tk.leave()
	n, err := src.Read(buf)
	tk.enter()
	return n, err
}

// closeSrc closes a request source for tk.
func closeSrc(tk *Task, src any) {
	if tc, ok := src.(tkCloser); ok {
		tc.CloseTk(tk)
	} else if c, ok := src.(io.Closer); ok {
		c.Close()
	}
}

// pipeHoldMax is the bytes a pipe holds before its reader's first read
// (decision 0444): past them a write waits for the reader, as io.Pipe's
// writes do (std/native http.xo PIPE_HOLD_MAX).
const pipeHoldMax = 65536

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

// ReadTk is Read through the task: the pipe of a test world waits on its
// channel, so the scheduler sees the wait.
func (r heldReader) ReadTk(tk *Task, p []byte) (int, error) {
	h := r.h
	if h.ch == nil {
		tk.leave()
		n, err := r.Read(p)
		tk.enter()
		return n, err
	}
	h.mu.Lock()
	h.started = true
	if len(h.held) > 0 {
		n := copy(p, h.held[0])
		if n < len(h.held[0]) {
			h.held[0] = h.held[0][n:]
		} else {
			h.held = h.held[1:]
		}
		h.mu.Unlock()
		return n, nil
	}
	if len(h.cur) == 0 {
		h.mu.Unlock()
		o := Chan_recv(tk, h.ch)
		h.mu.Lock()
		if !o.Some {
			err := h.err
			h.mu.Unlock()
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		h.cur = o.V
	}
	n := copy(p, h.cur)
	h.cur = h.cur[n:]
	h.mu.Unlock()
	return n, nil
}

// CloseTk is Close through the task: a writer waiting to send is released.
func (r heldReader) CloseTk(tk *Task) {
	h := r.h
	if h.ch == nil {
		r.Close()
		return
	}
	h.mu.Lock()
	h.closed = true
	h.rclosed = true
	h.mu.Unlock()
	for Chan_try_recv(tk, h.ch).Some {
	}
}

// Close is the client's: done with the request (its response ended or
// failed), the transport closes the body, so a write after it, and one
// waiting for the reader, fails with io.ErrClosedPipe and the transport's
// writer stops reading (it waited in mapRoundTripError for a body
// nobody would end, 2026-10-30).
func (r heldReader) Close() error {
	r.h.mu.Lock()
	r.h.closed = true
	r.h.mu.Unlock()
	return r.r.Close()
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
func Http_pipe(tk *Task) Tuple2[Http_PipeWriter, Http_BodyReader] {
	pr, pw := io.Pipe()
	h := &pipeHold{}
	if tk != nil && tk.th != nil && tk.th.sch != nil {
		h.ch = Chan_new[[]byte](tk, 0)
	}
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
	if !w.h.started && (w.h.closed || w.h.n+len(data) <= pipeHoldMax) {
		closed := w.h.closed
		if !closed {
			w.h.held = append(w.h.held, []byte(data))
			w.h.n += len(data)
		}
		w.h.mu.Unlock()
		if closed {
			return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: io.ErrClosedPipe.Error()}})
		}
		return Ok[Unit, IoErr](Unit{})
	}
	if w.h.ch != nil {
		wc := w.h.closed
		w.h.mu.Unlock()
		if wc {
			return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: io.ErrClosedPipe.Error()}})
		}
		Chan_send(tk, w.h.ch, append([]byte(nil), data...))
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
func (w Http_PipeWriter) M_close(tk *Task) {
	w.h.mu.Lock()
	already := w.h.closed
	w.h.closed = true
	w.h.mu.Unlock()
	if w.h.ch != nil {
		if !already {
			Chan_close(tk, w.h.ch)
		}
		return
	}
	w.w.Close()
}

// M_close_with_error is `w.close_with_error(e)`: the reader's next read
// is Err(e) (CloseWithError).
func (w Http_PipeWriter) M_close_with_error(tk *Task, e IoErr) {
	w.h.mu.Lock()
	already := w.h.closed
	w.h.closed = true
	if w.h.ch != nil && !already {
		w.h.err = pipeErr{e}
	}
	w.h.mu.Unlock()
	if w.h.ch != nil {
		if !already {
			Chan_close(tk, w.h.ch)
		}
		return
	}
	w.w.CloseWithError(pipeErr{e})
}
