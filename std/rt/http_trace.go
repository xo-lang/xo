package rt

import (
	"sync"
	"time"
)

// Client trace events (decision 0677): a request's with_trace function gets
// the events of its exchange from Go's httptrace hooks, recorded as they
// happen and delivered at the native backends' points (send's return, open's
// head, an opened body's end or close), so on runs at the same moments
// everywhere.

// M_with_trace is `r.with_trace(on)`.
func (r Http_Request) M_with_trace(_ *Task, on func(*Task, Http_TraceEvent) Result[Unit, Never]) Http_Request {
	r.XTrace = on
	return r
}

// traceRec is one request's recorded events, not yet given to on.
type traceRec struct {
	mu  sync.Mutex
	evs []Http_TraceEvent
	on  func(*Task, Http_TraceEvent) Result[Unit, Never]
	// conn: GotConn was recorded; wrote is closed when WroteRequest is
	conn  bool
	wrote chan struct{}
	once  sync.Once
}

// traceWroteWait bounds deliver's wait for WroteRequest: a request whose
// connection failed before its write never reports one.
const traceWroteWait = time.Second

// newTraceRec is req's recorder, nil without with_trace.
func newTraceRec(req Http_Request) *traceRec {
	if req.XTrace == nil {
		return nil
	}
	return &traceRec{on: req.XTrace, wrote: make(chan struct{})}
}

func (r *traceRec) add(tag int, p any) {
	r.mu.Lock()
	r.evs = append(r.evs, Http_TraceEvent{Tag: tag, P: p})
	if tag == 2 {
		r.conn = true
	}
	r.mu.Unlock()
	if tag == 10 {
		r.once.Do(func() { close(r.wrote) })
	}
}

// deliver gives the events recorded so far to on, in order (none for a nil
// recorder). Once a connection was got it first waits (at most
// traceWroteWait) for WroteRequest: net/http calls WroteHeaders and
// WroteRequest on the connection's writer goroutine (HTTP/2's request
// writer, HTTP/1's writeLoop), and RoundTrip can return on the response
// before that goroutine records them, so under CPU load send returned
// without them and a later call got them (task #151: http_client_trace's
// "https: reused" on host 1 with every CPU busy). The native backends write
// the request on the calling task, so they always have them by then.
func (r *traceRec) deliver(tk *Task) {
	if r == nil {
		return
	}
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn {
		t := time.NewTimer(traceWroteWait)
		select {
		case <-r.wrote:
		case <-t.C:
		}
		t.Stop()
	}
	r.mu.Lock()
	evs := r.evs
	r.evs = nil
	r.mu.Unlock()
	for _, e := range evs {
		r.on(tk, e)
	}
}

func traceNow() Time { return Time(time.Now().UnixNano()) }

func traceErr(err error) Option[string] {
	if err == nil {
		return None[string]()
	}
	return Some(err.Error())
}

// M_at is `e.at()`.
func (e *Http_TraceEvent) M_at(*Task) Time {
	switch p := e.P.(type) {
	case Http_TraceEvent_GetConn:
		return p.F_at
	case Http_TraceEvent_GotConn:
		return p.F_at
	case Http_TraceEvent_DnsStart:
		return p.F_at
	case Http_TraceEvent_DnsDone:
		return p.F_at
	case Http_TraceEvent_ConnectStart:
		return p.F_at
	case Http_TraceEvent_ConnectDone:
		return p.F_at
	case Http_TraceEvent_TlsHandshakeStart:
		return p.F_at
	case Http_TraceEvent_TlsHandshakeDone:
		return p.F_at
	case Http_TraceEvent_WroteHeaders:
		return p.F_at
	case Http_TraceEvent_WroteRequest:
		return p.F_at
	case Http_TraceEvent_GotFirstResponseByte:
		return p.F_at
	case Http_TraceEvent_Got100Continue:
		return p.F_at
	case Http_TraceEvent_PutIdleConn:
		return p.F_at
	}
	return 0
}

var traceNames = [...]string{"GetConn", "GotConn", "DNSStart", "DNSDone", "ConnectStart", "ConnectDone", "TLSHandshakeStart", "TLSHandshakeDone", "WroteHeaders", "WroteRequest", "GotFirstResponseByte", "Got100Continue", "PutIdleConn"}

// M_name is `e.name()`: Go's hook name.
func (e *Http_TraceEvent) M_name(*Task) string {
	if e.Tag < 1 || int(e.Tag) > len(traceNames) {
		return ""
	}
	return traceNames[e.Tag-1]
}
