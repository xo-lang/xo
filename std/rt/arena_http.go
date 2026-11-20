//go:build !xo_nohttp

package rt

import (
	"strings"
	"unsafe"
)

// heapCopy moves every arena backed part of a request to the heap, for a
// handler that may let the request escape.
func (r Http_Request) heapCopy() Http_Request {
	r.F_headers = copyMap(r.F_headers)
	r.XParams = copyMap(r.XParams)
	r.F_url.F_query = copyMap(r.F_url.F_query)
	r.F_body = Bytes(strings.Clone(string(r.F_body)))
	return r
}

// recCopyReq returns req for the recorder (replay.go), which keeps it
// past the request: its body, arena memory when a is not nil, copied to
// the heap. The tables need no copy: the recorder encodes their entries,
// and the strings in them are heap strings. Decision 0066, hole 1: before
// this copy, a recorded body was overwritten by the next request that
// reused the arena.
func recCopyReq(a *Arena, req Http_Request) Http_Request {
	if a != nil {
		req.F_body = Bytes(strings.Clone(string(req.F_body)))
	}
	return req
}

// recCopyResp is recCopyReq for a response, whose body is arena memory
// when it was built by Http_json_arena.
func recCopyResp(a *Arena, resp Http_Response) Http_Response {
	if a != nil {
		resp.F_body = Bytes(strings.Clone(string(resp.F_body)))
	}
	return resp
}

// Http_Router_arenaLast marks the route registered last as one whose
// handler confines its request (internal/gen proved it), so the request it
// gets may be arena memory.
func Http_Router_arenaLast(r *Http_Router) {
	if n := len(r.routes.s); n > 0 {
		r.routes.s[n-1].arena = true
	}
}

// Http_json_arena is `http.json(status, v)` in return position of a
// confined handler: the body and header table are arena memory.
func Http_json_arena[T any](tk *Task, status int64, v T) Http_Response {
	a := arenaResp(tk)
	if a == nil {
		return Http_json(tk, status, v)
	}
	a.respUsed = true
	w := JsonW{b: a.jbuf[:0], count: a.jcount[:0]}
	JsonEnc(tk, &w, v)
	a.jbuf, a.jcount = w.b[:0], w.count[:0]
	h := a.strMap()
	Map_put(tk, &h, "Content-Type", "application/json")
	var body Bytes
	if len(w.b) > 0 {
		body = Bytes(unsafe.String(&w.b[0], len(w.b)))
	}
	return Http_Response{F_status: status, F_headers: h, F_body: body}
}

// Http_text_arena is `http.text(status, s)` in return position of a
// confined handler: the header table is arena memory.
func Http_text_arena(tk *Task, status int64, s string) Http_Response {
	a := arenaResp(tk)
	if a == nil {
		return Http_text(tk, status, s)
	}
	a.respUsed = true
	h := a.strMap()
	Map_put(tk, &h, "Content-Type", "text/plain; charset=utf-8")
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(s)}
}
