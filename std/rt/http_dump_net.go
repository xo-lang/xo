//go:build !xo_nohttp

package rt

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
)

// The dumps (Go's httputil.DumpRequest, DumpRequestOut, DumpResponse) of
// std/http: the Xo values as net/http's, dumped by httputil itself.

// dumpHeader is a header map and its several fields as net/http's Header,
// the keys as stored (as Client.do sends them).
func dumpHeader(h Map[string, string], fields []string) http.Header {
	out := http.Header{}
	for k, v := range Map_All(h) {
		if len(fields) == 0 {
			out[k] = []string{v}
			continue
		}
		out[k] = append([]string(nil), valuesOf(h, fields, k)...)
	}
	return out
}

// dumpReq is req as a net/http client request (its URL, Host, header and
// body), or the URL's error.
func dumpReq(tk *Task, req Http_Request) (*http.Request, error) {
	var body io.Reader
	if len(req.F_body) > 0 {
		body = bytes.NewReader([]byte(req.F_body))
	}
	hr, err := http.NewRequest(req.F_method, requestURLString(req), body)
	if err != nil {
		return nil, err
	}
	if req.XHost != "" {
		hr.Host = req.XHost
	}
	hr.Header = dumpHeader(req.F_headers, req.XFields)
	return hr, nil
}

// Http_dump_request is `http.dump_request(req, body)`.
func Http_dump_request(tk *Task, req Http_Request, body bool) Bytes {
	hr, err := dumpReq(tk, req)
	if err != nil {
		return ""
	}
	b, err := httputil.DumpRequest(hr, body)
	if err != nil {
		return ""
	}
	return Bytes(b)
}

// Http_dump_request_out is `http.dump_request_out(req, body)`.
func Http_dump_request_out(tk *Task, req Http_Request, body bool) Result[Bytes, Http_ClientErr] {
	hr, err := dumpReq(tk, req)
	if err != nil {
		return Err[Bytes](clientErr(4, err.Error()))
	}
	b, err := httputil.DumpRequestOut(hr, body)
	if err != nil {
		return Err[Bytes](clientErr(4, err.Error()))
	}
	return Ok[Bytes, Http_ClientErr](Bytes(b))
}

// Http_dump_response is `http.dump_response(resp, body)`.
func Http_dump_response(tk *Task, resp Http_Response, body bool) Bytes {
	r := &http.Response{StatusCode: int(resp.F_status), Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: dumpHeader(resp.F_headers, resp.XFields), Body: io.NopCloser(bytes.NewReader([]byte(resp.F_body))), ContentLength: int64(len(resp.F_body))}
	b, err := httputil.DumpResponse(r, body)
	if err != nil {
		return ""
	}
	return Bytes(b)
}
