//go:build !xo_nohttp

package rt

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// rawExchange sends req to the in memory listener at hostport as HTTP/1.1
// with Connection: close and reads the response to the end of the
// connection. A body is sent whole; redirects and compression are not
// followed (std.md FakeNet).
func (f Testing_FakeNet) rawExchange(tk *Task, req Http_Request, hostport string) Result[Http_Response, Http_ClientErr] {
	d := f.M_dial(tk, hostport)
	if !d.Ok {
		return Err[Http_Response](clientErr(2, "dial tcp "+hostport+": "+d.E.M_display(tk)))
	}
	conn := d.V
	defer conn.M_close(tk)
	path := (&url.URL{Path: pathKeepInvalid(req.F_url.F_path)}).EscapedPath()
	if rp, ok := requestRawPath(req); ok {
		path = rp
	}
	if path == "" {
		path = "/"
	}
	if req.XRawQuery != "" {
		path += "?" + req.XRawQuery
	}
	host := req.F_url.F_host
	if req.XHost != "" {
		host = req.XHost
	}
	var b strings.Builder
	b.WriteString(req.F_method + " " + path + " HTTP/1.1\r\nHost: " + host + "\r\nConnection: close\r\n")
	for k, v := range Map_All(req.F_headers) {
		lk := strings.ToLower(k)
		if lk == "host" || lk == "content-length" || strings.HasPrefix(k, ":") {
			continue
		}
		b.WriteString(k + ": " + v + "\r\n")
	}
	body := string(req.F_body)
	if body != "" || req.F_method == "POST" || req.F_method == "PUT" || req.F_method == "PATCH" {
		b.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n")
	}
	b.WriteString("\r\n" + body)
	if w := conn.M_write(tk, Bytes(b.String())); !w.Ok {
		return Err[Http_Response](clientErr(2, "write: "+w.E.M_display(tk)))
	}
	var got bytes.Buffer
	for {
		r := conn.M_read(tk, 65536)
		if !r.Ok {
			return Err[Http_Response](clientErr(4, "read: "+r.E.M_display(tk)))
		}
		if !r.V.Some {
			break
		}
		got.WriteString(string(r.V.V))
	}
	hreq := &http.Request{Method: req.F_method}
	resp, err := http.ReadResponse(bufio.NewReader(&got), hreq)
	if err != nil {
		return Err[Http_Response](clientErr(4, err.Error()))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	hs := Map[string, string]{}
	var fields []string
	for k, vs := range resp.Header {
		for _, v := range vs {
			fields = append(fields, k, v)
		}
		Map_put(nil, &hs, k, strings.Join(vs, ", "))
	}
	return Ok[Http_Response, Http_ClientErr](Http_Response{F_status: int64(resp.StatusCode), F_headers: hs, F_body: Bytes(string(data)), XFields: fields})
}
