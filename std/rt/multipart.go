package rt

import (
	"bytes"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/textproto"
	"path"
	"sort"
	"strings"
)

// std/multipart (std.md 26, decision 0282): Go's mime/multipart over a
// body in memory, as std/native/multipart.xo ports it. Outside bytes
// become Str through textOf (decision 0259); the field and file names come
// from std/mime's parse_media_type of the Content-Disposition text, as
// the native code does.

// multipartMaxMemory is http's defaultMaxMemory, ParseMultipartForm's
// ReadForm limit.
const multipartMaxMemory = 32 << 20

func (e Multipart_MultipartErr) M_display(*Task) string { return e.F_msg }

func multipartErr(err error) Multipart_MultipartErr {
	return Multipart_MultipartErr{F_msg: textOf(err.Error())}
}

// multipartNames is a part's form name and file name (Part.FormName and
// FileName, with a Unix filepath.Base on every OS).
func multipartNames(tk *Task, h textproto.MIMEHeader) (string, string) {
	r := Mime_parse_media_type(tk, textOf(h.Get("Content-Disposition")))
	if !r.Ok {
		return "", ""
	}
	m := r.V
	name := ""
	if m.F_kind == "form-data" {
		name = Map_get(tk, m.F_params, "name").V
	}
	f := Map_get(tk, m.F_params, "filename").V
	if f == "" {
		return name, ""
	}
	return name, path.Base(f)
}

func multipartPart(tk *Task, h textproto.MIMEHeader, data []byte) Multipart_Part {
	name, file := multipartNames(tk, h)
	var hs Map[string, string]
	for k, vs := range h {
		Map_put(tk, &hs, textOf(k), textOf(vs[0]))
	}
	p := Multipart_Part{F_name: name, F_headers: hs, F_data: Bytes(data)}
	if file != "" {
		p.F_filename = Some(file)
	}
	return p
}

func Multipart_parse(tk *Task, body Bytes, boundary string) Result[List[Multipart_Part], Multipart_MultipartErr] {
	mr := multipart.NewReader(strings.NewReader(string(body)), boundary)
	var parts []Multipart_Part
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Err[List[Multipart_Part]](multipartErr(err))
		}
		d, err := io.ReadAll(p)
		if err != nil {
			return Err[List[Multipart_Part]](multipartErr(err))
		}
		parts = append(parts, multipartPart(tk, p.Header, d))
	}
	return Ok[List[Multipart_Part], Multipart_MultipartErr](ListFromSlice(parts))
}

// Multipart_parse_form is Reader.ReadForm(32 MB) as a loop over NextPart,
// so the fields keep their order (ReadForm returns a map) and max_parts
// is the call's (Go's is GODEBUG multipartmaxparts, process wide). One
// difference from the native port: NextPart's header limits are per part
// (10 MB, 10,000 headers) where ReadForm's are shared by the form.
func Multipart_parse_form(tk *Task, body Bytes, boundary string, maxParts int64) Result[Multipart_Form, Multipart_MultipartErr] {
	return multipartForm(tk, body, boundary, maxParts)
}

func multipartForm(tk *Task, body Bytes, boundary string, maxParts int64) Result[Multipart_Form, Multipart_MultipartErr] {
	tooLarge := Err[Multipart_Form](Multipart_MultipartErr{F_msg: multipart.ErrMessageTooLarge.Error()})
	mr := multipart.NewReader(strings.NewReader(string(body)), boundary)
	var vs Url_Values
	var files Map[string, List[Multipart_Part]]
	maxFile := int64(multipartMaxMemory)
	maxMem := int64(multipartMaxMemory + 10<<20)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Err[Multipart_Form](multipartErr(err))
		}
		if maxParts <= 0 {
			return tooLarge
		}
		maxParts--
		name, file := multipartNames(tk, p.Header)
		if name == "" {
			continue
		}
		maxMem -= int64(len(name)) + 200
		if maxMem < 0 {
			return tooLarge
		}
		var b bytes.Buffer
		if file == "" {
			n, err := io.CopyN(&b, p, maxMem+1)
			if err != nil && err != io.EOF {
				return Err[Multipart_Form](multipartErr(err))
			}
			maxMem -= n
			if maxMem < 0 {
				return tooLarge
			}
			vs.M_add(tk, name, textOf(b.String()))
			continue
		}
		size := int64(400)
		for k, hv := range p.Header {
			size += int64(len(k)) + 200
			for _, v := range hv {
				size += int64(len(v))
			}
		}
		maxMem -= size + 300
		if maxMem < 0 {
			return tooLarge
		}
		if _, err := io.Copy(&b, p); err != nil {
			return Err[Multipart_Form](multipartErr(err))
		}
		if n := int64(b.Len()); n <= maxFile {
			maxFile -= n
			maxMem -= n
		}
		fs := Map_get(tk, files, name).V
		Map_put(tk, &files, name, ListFromSlice(append(append([]Multipart_Part{}, List_Slice(fs)...), multipartPart(tk, p.Header, b.Bytes()))))
	}
	return Ok[Multipart_Form, Multipart_MultipartErr](Multipart_Form{F_values: vs, F_files: files})
}

var multipartQuote = strings.NewReplacer("\\", "\\\\", `"`, "\\\"", "\r", "%0D", "\n", "%0A")

func Multipart_write(tk *Task, parts List[Multipart_Part], boundary string) Bytes {
	var b bytes.Buffer
	i := 0
	for p := range List_All(parts) {
		i++
		if i == 1 {
			b.WriteString("--" + boundary + "\r\n")
		} else {
			b.WriteString("\r\n--" + boundary + "\r\n")
		}
		h := map[string]string{}
		for k, v := range Map_All(p.F_headers) {
			h[k] = v
		}
		if _, ok := h["Content-Disposition"]; !ok {
			d := `form-data; name="` + multipartQuote.Replace(p.F_name) + `"`
			if p.F_filename.Some {
				d += `; filename="` + multipartQuote.Replace(p.F_filename.V) + `"`
				if _, ok := h["Content-Type"]; !ok {
					h["Content-Type"] = "application/octet-stream"
				}
			}
			h["Content-Disposition"] = d
		}
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(k + ": " + h[k] + "\r\n")
		}
		b.WriteString("\r\n")
		b.WriteString(string(p.F_data))
	}
	b.WriteString("\r\n--" + boundary + "--\r\n")
	return Bytes(b.String())
}

func Multipart_form_data_content_type(_ *Task, boundary string) string {
	b := boundary
	if strings.ContainsAny(b, `()<>@,;:\"/[]?= `) {
		b = `"` + b + `"`
	}
	return "multipart/form-data; boundary=" + b
}

func Multipart_random_boundary(tk *Task, r Rand) string {
	return hex.EncodeToString([]byte(r.M_bytes(tk, 30)))
}
