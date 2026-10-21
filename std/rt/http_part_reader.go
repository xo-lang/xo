package rt

import (
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
	"sync"
)

// Http_PartReader is a multipart body read part by part (decision 0310
// phase 3): Go's multipart.Reader over a BodyReader's source.
type Http_PartReader struct{ p *partReader }

// XoDebug implements Debug.
func (Http_PartReader) XoDebug(b *strings.Builder) { b.WriteString("<part reader>") }

type partReader struct {
	mu   sync.Mutex
	r    *multipart.Reader
	cur  *multipart.Part
	last bool
	err  error
}

// bodySource reads a BodyReader's source under its lock.
type bodySource struct{ r *bodyReader }

func (s bodySource) Read(p []byte) (int, error) {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	if s.r.eof {
		return 0, io.EOF
	}
	n, err := s.r.src.Read(p)
	if err == io.EOF {
		s.r.eof = true
	}
	return n, err
}

// M_multipart is `b.multipart(req)`: net/http's Request.MultipartReader
// checks (multipart/form-data with a boundary), then a reader over b.
func (b Http_BodyReader) M_multipart(tk *Task, req Http_Request) Result[Http_PartReader, Multipart_MultipartErr] {
	notMultipart := Err[Http_PartReader](Multipart_MultipartErr{F_msg: "request Content-Type isn't multipart/form-data"})
	ct := headerGet(req.F_headers, "Content-Type").V
	if ct == "" {
		return notMultipart
	}
	m := Mime_parse_media_type(tk, ct)
	if !m.Ok || m.V.F_kind != "multipart/form-data" {
		return notMultipart
	}
	bd := Map_get(tk, m.V.F_params, "boundary")
	if !bd.Some {
		return Err[Http_PartReader](Multipart_MultipartErr{F_msg: "no multipart boundary param in Content-Type"})
	}
	return Ok[Http_PartReader, Multipart_MultipartErr](Http_PartReader{p: &partReader{r: multipart.NewReader(bodySource{b.r}, bd.V)}})
}

// M_next is `p.next()`: NextPart, None at io.EOF; an error sticks.
func (p Http_PartReader) M_next(tk *Task) Result[Option[Http_StreamPart], Multipart_MultipartErr] {
	r := p.p
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return Err[Option[Http_StreamPart]](Multipart_MultipartErr{F_msg: r.err.Error()})
	}
	if r.last {
		return Ok[Option[Http_StreamPart], Multipart_MultipartErr](None[Http_StreamPart]())
	}
	part, err := r.r.NextPart()
	if err == io.EOF {
		r.last = true
		r.cur = nil
		return Ok[Option[Http_StreamPart], Multipart_MultipartErr](None[Http_StreamPart]())
	}
	if err != nil {
		r.err = err
		r.cur = nil
		return Err[Option[Http_StreamPart]](Multipart_MultipartErr{F_msg: err.Error()})
	}
	r.cur = part
	sp := Http_StreamPart{F_name: part.FormName()}
	if f := part.FileName(); f != "" {
		sp.F_filename = Some(f)
	} else {
		sp.F_filename = None[string]()
	}
	keys := make([]string, 0, len(part.Header))
	for k := range part.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vs := part.Header[k]
		if len(vs) == 0 {
			continue
		}
		Map_put(tk, &sp.F_headers, k, vs[0])
		for _, v := range vs {
			sp.XFields = append(sp.XFields, k, v)
		}
	}
	return Ok[Option[Http_StreamPart], Multipart_MultipartErr](Some(sp))
}

// M_read is `p.read(max)`: Part.Read of the open part, None at its end
// or before the first part.
func (p Http_PartReader) M_read(tk *Task, max int64) Result[Option[Bytes], Multipart_MultipartErr] {
	if max < 1 {
		FaultHere("PartReader.read: max must be at least 1, got %d", max)
	}
	r := p.p
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return Err[Option[Bytes]](Multipart_MultipartErr{F_msg: r.err.Error()})
	}
	if r.cur == nil {
		return Ok[Option[Bytes], Multipart_MultipartErr](None[Bytes]())
	}
	buf := make([]byte, min(max, readMax))
	for {
		n, err := r.cur.Read(buf)
		if n > 0 {
			return Ok[Option[Bytes], Multipart_MultipartErr](Some(Bytes(buf[:n])))
		}
		if errors.Is(err, io.EOF) {
			return Ok[Option[Bytes], Multipart_MultipartErr](None[Bytes]())
		}
		if err != nil {
			return Err[Option[Bytes]](Multipart_MultipartErr{F_msg: err.Error()})
		}
	}
}

// M_header_values is `p.header_values(name)`: MIMEHeader.Values.
func (p Http_StreamPart) M_header_values(_ *Task, name string) List[string] {
	k := textproto.CanonicalMIMEHeaderKey(name)
	var vs []string
	for i := 0; i+1 < len(p.XFields); i += 2 {
		if p.XFields[i] == k {
			vs = append(vs, p.XFields[i+1])
		}
	}
	return ListOf(vs...)
}

