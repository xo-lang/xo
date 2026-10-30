package rt

// http.gzip_responses on the Go backend (std.md 4.7, decision 0305):
// gzhttp's rules, the same as std/native/http.xo's gzip_response.

import (
	"compress/gzip"
	"net/textproto"
	"strconv"
	"strings"
)

// gzipAccepts is whether Accept-Encoding allows gzip: a gzip or *
// element whose q is not 0.
func gzipAccepts(v string) bool {
	for _, el := range strings.Split(v, ",") {
		parts := strings.Split(el, ";")
		token := strings.ToLower(strings.TrimSpace(parts[0]))
		if token != "gzip" && token != "*" {
			continue
		}
		q := 1.0
		for _, p := range parts[1:] {
			p = strings.ToLower(strings.TrimSpace(p))
			if strings.HasPrefix(p, "q=") {
				f, err := strconv.ParseFloat(strings.TrimSpace(p[2:]), 64)
				if err != nil {
					f = 0
				}
				q = f
			}
		}
		if q > 0 {
			return true
		}
	}
	return false
}

func gzipCompressedType(ct string) bool {
	t := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	return strings.HasPrefix(t, "image/") || strings.HasPrefix(t, "video/") || t == "application/zip" || t == "application/gzip" || t == "font/woff2"
}

// gzipWriterTo writes a gzip.Writer's output to a BodyWriter.
type gzipWriterTo struct {
	tk *Task
	w  Http_BodyWriter
}

func (g gzipWriterTo) Write(p []byte) (int, error) {
	if r := g.w.M_write(g.tk, Bytes(p)); !r.Ok {
		return 0, ioErrOf(r.E)
	}
	return len(p), nil
}

type ioErrValue struct{ e IoErr }

func (e ioErrValue) Error() string { return "write failed" }

func ioErrOf(e IoErr) error { return ioErrValue{e} }

type gzipFlusher struct {
	tk  *Task
	gz  *gzip.Writer
	out Http_BodyWriter
}

func (f gzipFlusher) Flush() {
	f.gz.Flush()
	f.out.M_flush(f.tk)
}

func gzipStream(inner func(*Task, Http_BodyWriter) Result[Unit, IoErr], level int) func(*Task, Http_BodyWriter) Result[Unit, IoErr] {
	return func(tk *Task, w Http_BodyWriter) Result[Unit, IoErr] {
		gz, _ := gzip.NewWriterLevel(gzipWriterTo{tk: tk, w: w}, level)
		r := inner(tk, Http_BodyWriter{w: &bodyWriter{w: gz, fl: gzipFlusher{tk: tk, gz: gz, out: w}, tr: w.w.tr}})
		if err := gz.Close(); err != nil {
			if ie, ok := err.(ioErrValue); ok {
				return Err[Unit](ie.e)
			}
		}
		return r
	}
}

func gzipResponse(tk *Task, req Http_Request, resp Http_Response, min int64, level int) Http_Response {
	st := resp.F_status
	if st < 200 || st == 204 || st == 304 || st == 206 {
		return resp
	}
	if resp.M_header(tk, "Content-Encoding").Some || resp.M_header(tk, "Content-Range").Some {
		return resp
	}
	streamed := resp.XStream != nil
	ct := ""
	if v := resp.M_header(tk, "Content-Type"); v.Some {
		ct = v.V
	}
	if ct == "" && !streamed && len(resp.F_body) > 0 {
		ct = Mime_detect_content_type(tk, resp.F_body)
	}
	if gzipCompressedType(ct) {
		return resp
	}
	vary := false
	for _, v := range resp.M_header_values(tk, "Vary").s {
		if strings.Contains(strings.ToLower(v), "accept-encoding") {
			vary = true
		}
	}
	if !vary {
		resp = resp.M_add_header(tk, "Vary", "Accept-Encoding")
	}
	ae := ""
	if v := req.M_header(tk, "Accept-Encoding"); v.Some {
		ae = v.V
	}
	if !gzipAccepts(ae) {
		return resp
	}
	if streamed && ct == "" {
		return resp
	}
	if !streamed && int64(len(resp.F_body)) < min {
		return resp
	}
	resp = resp.M_with_header(tk, "Content-Encoding", "gzip")
	k := textproto.CanonicalMIMEHeaderKey("Content-Length")
	Map_remove(tk, &resp.F_headers, k)
	resp.XFields = withoutField(resp.XFields, k)
	if ct != "" {
		resp = resp.M_with_header(tk, "Content-Type", ct)
	}
	if et := resp.M_header(tk, "ETag"); et.Some && !strings.HasPrefix(et.V, "W/") {
		resp = resp.M_with_header(tk, "ETag", "W/"+et.V)
	}
	if !streamed {
		resp.F_body = Compress_gzip(tk, resp.F_body, compressLevelOf(level))
		return resp
	}
	resp.XStream = gzipStream(resp.XStream, level)
	return resp
}

// compressLevelOf is the Level of Go's level n.
func compressLevelOf(n int) Compress_Level {
	return Compress_Level{Tag: 6, P: Compress_Level_Of{F_n: int64(n)}}
}

// Http_gzip_responses is `http.gzip_responses(min_size, level)`.
func Http_gzip_responses(_ *Task, min int64, level Compress_Level) func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
	lv := compressLevel(level)
	return func(tk *Task, req Http_Request, next func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
		r := next(tk, req)
		return Ok[Http_Response, Never](gzipResponse(tk, req, r.V, min, lv))
	}
}
