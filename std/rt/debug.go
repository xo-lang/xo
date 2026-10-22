package rt

// std/debug on the Go backend (decision 0325): the same routes over
// runtime/pprof. The profile answers are pprof (gzip protobuf), where the
// native backends answer decision 0240's container; the hz query is
// ignored (Go samples at 100 Hz), and heap profiling is always on.

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync/atomic"
)

// debugBusy is 1 while a /profile request runs: one at a time.
var debugBusy int32

func debugInt(req Http_Request, name string, dflt, lo, hi int64) int64 {
	o := Map_get(nil, req.F_url.F_query, name)
	if !o.Some {
		return dflt
	}
	n, err := strconv.ParseInt(o.V, 10, 64)
	if err != nil {
		return dflt
	}
	return min(max(n, lo), hi)
}

func debugAnswer(status int64, ctype string, body string, extra ...string) Http_Response {
	h := mapCap[string, string](2)
	Map_put(nil, &h, "Content-Type", ctype)
	for i := 0; i+1 < len(extra); i += 2 {
		Map_put(nil, &h, extra[i], extra[i+1])
	}
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(body)}
}

// Debug_router is `debug.router(clock, auth)`.
func Debug_router(tk *Task, clock Clock, auth func(*Task, Http_Request) Result[bool, Never]) Http_Router {
	r := Http_router(tk)
	allowed := func(tk *Task, req Http_Request) bool {
		a := auth(tk, req)
		return a.Ok && a.V
	}
	Http_Router_handle(tk, &r, "GET /profile", func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		if !allowed(tk, req) {
			return Ok[Http_Response, Never](Http_text(tk, 403, "forbidden\n"))
		}
		secs := debugInt(req, "seconds", 30, 1, 300)
		if !atomic.CompareAndSwapInt32(&debugBusy, 0, 1) {
			return Ok[Http_Response, Never](Http_text(tk, 409, "a profile is running already\n"))
		}
		defer atomic.StoreInt32(&debugBusy, 0)
		var buf bytes.Buffer
		if err := pprof.StartCPUProfile(&buf); err != nil {
			return Ok[Http_Response, Never](Http_text(tk, 409, "a profile is running already\n"))
		}
		clock.M_sleep(tk, Duration(secs)*1000000000)
		pprof.StopCPUProfile()
		return Ok[Http_Response, Never](debugAnswer(200, "application/octet-stream", buf.String(), "X-Profile-Format", "pprof"))
	})
	Http_Router_handle(tk, &r, "GET /heap", func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		if !allowed(tk, req) {
			return Ok[Http_Response, Never](Http_text(tk, 403, "forbidden\n"))
		}
		var buf bytes.Buffer
		if err := pprof.Lookup("heap").WriteTo(&buf, 0); err != nil {
			return Ok[Http_Response, Never](Http_text(tk, 503, "no heap profile\n"))
		}
		return Ok[Http_Response, Never](debugAnswer(200, "application/octet-stream", buf.String(), "X-Profile-Format", "pprof"))
	})
	Http_Router_handle(tk, &r, "GET /sched", func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		if !allowed(tk, req) {
			return Ok[Http_Response, Never](Http_text(tk, 403, "forbidden\n"))
		}
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		lines := []string{
			"goroutines " + strconv.Itoa(runtime.NumGoroutine()),
			"gomaxprocs " + strconv.Itoa(runtime.GOMAXPROCS(0)),
			"threads " + strconv.Itoa(pprof.Lookup("threadcreate").Count()),
			"heap_inuse_bytes " + strconv.FormatUint(ms.HeapInuse, 10),
			"gc_count " + strconv.FormatUint(uint64(ms.NumGC), 10),
		}
		return Ok[Http_Response, Never](debugAnswer(200, "text/plain; charset=utf-8", strings.Join(lines, "\n")+"\n"))
	})
	return r
}

// Debug_heap_start is `debug.heap_start(rate)`: Go samples the heap
// always, so there is nothing to start.
func Debug_heap_start(_ *Task, _ int64) bool { return false }
