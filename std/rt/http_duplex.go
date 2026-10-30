package rt

import (
	"io"
	"strings"
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
type Http_PipeWriter struct{ w *io.PipeWriter }

// XoDebug implements Debug.
func (Http_PipeWriter) XoDebug(b *strings.Builder) { b.WriteString("<pipe writer>") }

// pipeErr carries close_with_error's IoErr to the reader, which returns
// it as it was given.
type pipeErr struct{ e IoErr }

func (p pipeErr) Error() string { return p.e.M_display(nil) }

// Http_pipe is `http.pipe()`: each write waits until the reader has taken
// its bytes (io.Pipe), so a writer never runs ahead of the request.
func Http_pipe(_ *Task) Tuple2[Http_PipeWriter, Http_BodyReader] {
	pr, pw := io.Pipe()
	return Tuple2[Http_PipeWriter, Http_BodyReader]{F0: Http_PipeWriter{w: pw}, F1: Http_BodyReader{r: &bodyReader{src: pr}}}
}

// M_write is `w.write(data)`: Io("io: read/write on closed pipe") once
// either end is closed.
func (w Http_PipeWriter) M_write(tk *Task, data Bytes) Result[Unit, IoErr] {
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	tk.leave()
	_, err := w.w.Write([]byte(data))
	tk.enter()
	if err != nil {
		return Err[Unit](IoErr{Tag: 2, P: IoErr_Io{F_msg: err.Error()}})
	}
	return Ok[Unit, IoErr](Unit{})
}

// M_close is `w.close()`: the reader's next read is None.
func (w Http_PipeWriter) M_close(_ *Task) { w.w.Close() }

// M_close_with_error is `w.close_with_error(e)`: the reader's next read
// is Err(e) (CloseWithError).
func (w Http_PipeWriter) M_close_with_error(_ *Task, e IoErr) { w.w.CloseWithError(pipeErr{e}) }
