package rt

import (
	"bytes"
	"net/http"
	"net/url"
	"time"
)

// serveRecorder is the http.ResponseWriter Http_serve_content hands to
// net/http's ServeContent: the fields, the status, and the body it wrote.
type serveRecorder struct {
	h    http.Header
	code int
	body bytes.Buffer
}

func (r *serveRecorder) Header() http.Header { return r.h }

func (r *serveRecorder) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}

func (r *serveRecorder) Write(p []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(p)
}

// Http_serve_content is `http.serve_content(req, name, modtime, content,
// headers)` (decision 0306): the response net/http's ServeContent writes
// for req, with headers as the fields set on the ResponseWriter first.
func Http_serve_content(tk *Task, req Http_Request, name string, modtime Option[Time], content Bytes, headers Map[string, string]) Http_Response {
	hr := &http.Request{Method: req.F_method, URL: &url.URL{Path: req.F_path, RawQuery: req.XRawQuery}, Header: http.Header{}, Host: req.F_url.F_host}
	for k, v := range Map_All(req.F_headers) {
		hr.Header.Set(k, v)
	}
	w := &serveRecorder{h: http.Header{}}
	for k, v := range Map_All(headers) {
		w.h.Set(k, v)
	}
	var mt time.Time
	if modtime.Some {
		mt = time.Unix(0, int64(modtime.V)).UTC()
	}
	http.ServeContent(w, hr, name, mt, bytes.NewReader([]byte(content)))
	code := w.code
	if code == 0 {
		code = http.StatusOK
	}
	var h Map[string, string]
	var fields []string
	for k, vs := range w.h {
		Map_put(tk, &h, k, fieldView(k, vs))
		if len(vs) > 1 {
			fields = appendFields(fields, k, vs)
		}
	}
	return Http_Response{F_status: int64(code), F_headers: h, F_body: Bytes(w.body.Bytes()), XFields: fields}
}
