package rt

import (
	"io"
	"net/textproto"
	"strings"
	"sync"
)

// Http_PartReader is a multipart body read part by part (decision 0310
// phase 3): std/multipart's Reader (generated from std/multipart.xo,
// decision 0597) over a BodyReader's source, as on the native backends.
type Http_PartReader struct{ p *partReader }

func (Http_PartReader) XoDebug(b *strings.Builder) { b.WriteString("<part reader>") }

type partReader struct {
	mu sync.Mutex
	r  Multipart_Reader
}

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

// pull is the Reader's source: the body's next bytes, None at its end, or
// its read error's text.
func (s bodySource) pull(_ *Task) Result[Option[Bytes], string] {
	buf := make([]byte, 32768)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			return Ok[Option[Bytes], string](Some(Bytes(buf[:n])))
		}
		if err == io.EOF {
			return Ok[Option[Bytes], string](None[Bytes]())
		}
		if err != nil {
			return Err[Option[Bytes]](err.Error())
		}
	}
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
	return Ok[Http_PartReader, Multipart_MultipartErr](Http_PartReader{p: &partReader{r: Multipart_reader(tk, bodySource{b.r}.pull, bd.V)}})
}

// M_next is `p.next()`: Reader.next_part, the head as a StreamPart.
func (p Http_PartReader) M_next(tk *Task) Result[Option[Http_StreamPart], Multipart_MultipartErr] {
	r := p.p
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.r.M_next_part(tk)
	if !h.Ok {
		return Err[Option[Http_StreamPart]](h.E)
	}
	if !h.V.Some {
		return Ok[Option[Http_StreamPart], Multipart_MultipartErr](None[Http_StreamPart]())
	}
	v := h.V.V
	sp := Http_StreamPart{F_name: v.F_name, F_filename: v.F_filename, F_headers: v.F_headers}
	sp.XFields = append(sp.XFields, v.F_fields.s...)
	return Ok[Option[Http_StreamPart], Multipart_MultipartErr](Some(sp))
}

// M_read is `p.read(max)`: Reader.read of the open part, None at its end
// or before the first part.
func (p Http_PartReader) M_read(tk *Task, max int64) Result[Option[Bytes], Multipart_MultipartErr] {
	if max < 1 {
		FaultHere("PartReader.read: max must be at least 1, got %d", max)
	}
	r := p.p
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.r.M_read(tk, max)
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
