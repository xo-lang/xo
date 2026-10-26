package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strings"
)

// mpNames is std/rt's multipartNames with Go's mime.ParseMediaType.
func mpNames(h textproto.MIMEHeader) (string, string) {
	kind, ps, err := mime.ParseMediaType(h.Get("Content-Disposition"))
	if err != nil {
		return "", ""
	}
	name := ""
	if kind == "form-data" {
		name = ps["name"]
	}
	f := ps["filename"]
	if f == "" {
		return name, ""
	}
	return name, path.Base(f)
}

// mpPart is the corpus's text of a part: name, file name, the headers'
// first values sorted, and the data's length and sha256 prefix.
func mpPart(h textproto.MIMEHeader, d []byte) string {
	name, file := mpNames(h)
	if file == "" {
		file = "-"
	}
	var hs []string
	for k, vs := range h {
		hs = append(hs, textOf(k)+"="+textOf(vs[0]))
	}
	sort.Strings(hs)
	s := sha256.Sum256(d)
	return textOf(name) + "|" + textOf(file) + "|" + strings.Join(hs, ";") + "|" + fmt.Sprintf("%d %s", len(d), hex.EncodeToString(s[:4]))
}

// mpParse is std/rt's Multipart_parse: each part's text, or err TEXT.
func mpParse(body []byte, boundary string) []string {
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var out []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			return out
		}
		if err != nil {
			return append(out, "err "+textOf(err.Error()))
		}
		d, err := io.ReadAll(p)
		if err != nil {
			return append(out, "err "+textOf(err.Error()))
		}
		out = append(out, mpPart(p.Header, d))
	}
}

// mpForm is std/rt's parse_form with max_parts: the values encoded and
// the file parts by field, or err TEXT.
func mpForm(body []byte, boundary string, maxParts int) string {
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	vs := url.Values{}
	files := map[string][]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "err " + textOf(err.Error())
		}
		if maxParts <= 0 {
			return "err " + multipart.ErrMessageTooLarge.Error()
		}
		maxParts--
		name, file := mpNames(p.Header)
		// ReadForm skips a part without a form name unread
		if name == "" {
			continue
		}
		d, err := io.ReadAll(p)
		if err != nil {
			return "err " + textOf(err.Error())
		}
		if file == "" {
			vs.Add(textOf(name), textOf(string(d)))
			continue
		}
		files[name] = append(files[name], mpPart(p.Header, d))
	}
	var fs []string
	for k, v := range files {
		fs = append(fs, textOf(k)+"=["+strings.Join(v, ", ")+"]")
	}
	sort.Strings(fs)
	return "ok " + vs.Encode() + " files " + strings.Join(fs, " ")
}

func mpBody(r *rand.Rand, boundary string) []byte {
	var b strings.Builder
	nl := "\r\n"
	if r.Intn(8) == 0 {
		nl = "\n"
	}
	if r.Intn(4) == 0 {
		b.WriteString([]string{"preamble" + nl, "This is a message with multiple parts in MIME format." + nl, nl}[r.Intn(3)])
	}
	n := r.Intn(5)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(nl)
		}
		b.WriteString("--" + boundary + []string{"", "", "", " ", "\t ", "x"}[r.Intn(6)] + nl)
		name := []string{"a", "field", "f name", "ü", "x\"y", ""}[r.Intn(6)]
		switch r.Intn(7) {
		case 0, 1, 2:
			fmt.Fprintf(&b, "Content-Disposition: form-data; name=%q%s", name, nl)
		case 3:
			fmt.Fprintf(&b, "Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"%sContent-Type: %s%s", name, []string{"a.txt", "dir/b.bin", `C:\x\y.png`, "", "../../etc/passwd"}[r.Intn(5)], nl, []string{"text/plain", "application/octet-stream", "image/png"}[r.Intn(3)], nl)
		case 4:
			fmt.Fprintf(&b, "Content-Disposition: form-data; name*=UTF-8''%s%s", url.PathEscape(name), nl)
		case 5:
			fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%s%s", []string{"plain.txt", "\"q.txt\"", "bad name"}[r.Intn(3)], nl)
		default:
			fmt.Fprintf(&b, "content-type: text/plain; charset=utf-8%s", nl)
		}
		if r.Intn(5) == 0 {
			fmt.Fprintf(&b, "Content-Transfer-Encoding: quoted-printable%s", nl)
		}
		if r.Intn(6) == 0 {
			fmt.Fprintf(&b, "X-Folded: one%s two%s", nl, nl)
		}
		if r.Intn(10) == 0 {
			fmt.Fprintf(&b, "bad header line%s", nl)
		}
		b.WriteString(nl)
		for j, m := 0, r.Intn(4); j < m; j++ {
			b.WriteString([]string{"hello", "=3D=41", "soft=" + nl + "break", "--" + boundary[:len(boundary)/2], nl, "é", "\x00\xff", "line" + nl + "two"}[r.Intn(8)])
		}
	}
	if n > 0 || r.Intn(3) == 0 {
		b.WriteString(nl + "--" + boundary + []string{"--", "--", "--", "", "-"}[r.Intn(5)] + []string{"", nl, nl + "epilogue"}[r.Intn(3)])
	}
	return []byte(b.String())
}

func multipartCorpus() string {
	r := rand.New(rand.NewSource(4))
	var o strings.Builder
	o.WriteString(header("mime/multipart's Reader (NextPart, each part read whole) as std/rt/multipart.go calls it, on seeded random bodies, some cut short or with a byte changed"))
	o.WriteString("use std/multipart\nuse std/crypto\n\n/// (body, boundary, max_parts, Go's parts (or a last \"err TEXT\"), Go's form)\nconst BODIES: List[(Bytes, Str, Int, List[Str], Str)] = [\n")
	for i := 0; i < 1500; i++ {
		boundary := []string{"xyz", "MyBoundary", "----WebKitFormBoundary7MA4YWxkTrZu0gW", "a b", "1234567890", "=_qp"}[r.Intn(6)]
		body := mpBody(r, boundary)
		switch {
		case i%6 == 4 && len(body) > 0:
			body = body[:r.Intn(len(body))]
		case i%6 == 5 && len(body) > 0:
			body = append([]byte{}, body...)
			body[r.Intn(len(body))] = "-\r\n:=\" ;x"[r.Intn(9)]
		}
		maxParts := []int{1000, 1000, 2, 1, 0}[r.Intn(5)]
		ps := mpParse(body, boundary)
		qs := make([]string, len(ps))
		for k, p := range ps {
			qs[k] = xq(p)
		}
		fmt.Fprintf(&o, "  (%s, %s, %d, [%s], %s),\n", xbytes(body), xq(boundary), maxParts, strings.Join(qs, ", "), xq(mpForm(body, boundary, maxParts)))
	}
	o.WriteString("]\n")
	o.WriteString(multipartWriteTables(r))
	o.WriteString(multipartTests)
	return o.String()
}

// mpwPart is one part the corpus writes: its name, file name (when it has
// one), the headers it arrives with, and its data.
type mpwPart struct {
	name    string
	hasFile bool
	file    string
	headers [][2]string
	data    []byte
}

// mpwEscape is std's escape_quotes: a synthesized Content-Disposition keeps
// its quoted name well formed.
func mpwEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", "%0D", "\n", "%0A")
	return r.Replace(s)
}

// mpwWrite is Go's multipart.Writer bytes for parts at boundary. std's write
// synthesizes the two headers Go's Writer leaves to its caller:
// Content-Disposition when the part's map lacks that exact key (with the
// filename when the part has one), and Content-Type
// application/octet-stream when it synthesized one for a file part and the
// map lacks Content-Type. The same synthesis runs here, so both sides write
// the same headers, sorted. The headers go through textOf first: an Xo Str
// holds U+FFFD where a header value's bytes were not UTF-8, so that is the
// input both sides must see.
func mpwWrite(parts []mpwPart, boundary string) []byte {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	if err := w.SetBoundary(boundary); err != nil {
		panic(err)
	}
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		for _, kv := range p.headers {
			h[textOf(kv[0])] = []string{textOf(kv[1])}
		}
		if _, ok := h["Content-Disposition"]; !ok {
			d := `form-data; name="` + mpwEscape(textOf(p.name)) + `"`
			if p.hasFile {
				d += `; filename="` + mpwEscape(textOf(p.file)) + `"`
			}
			h["Content-Disposition"] = []string{d}
			if p.hasFile {
				if _, ok := h["Content-Type"]; !ok {
					h["Content-Type"] = []string{"application/octet-stream"}
				}
			}
		}
		pw, err := w.CreatePart(h)
		if err != nil {
			panic(err)
		}
		if _, err := pw.Write(p.data); err != nil {
			panic(err)
		}
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// mpwPartText is one part as the generated Xo literal reads it.
func mpwPartText(p mpwPart) string {
	var hs []string
	for _, kv := range p.headers {
		hs = append(hs, fmt.Sprintf("(%s, %s)", xq(textOf(kv[0])), xq(textOf(kv[1]))))
	}
	// the file name is always a Str literal; has_file says whether the part
	// carries one (None when it does not)
	file := xq(textOf(p.file))
	return fmt.Sprintf("(%s, %v, %s, [%s], %s)", xq(textOf(p.name)), p.hasFile, file, strings.Join(hs, ", "), xbytes(p.data))
}

// mpwHeaders are the header sets a written part carries: the shapes a
// writer must reproduce byte for byte, including values that are not UTF-8,
// a folded value, an empty value, a name with a space, and a
// Content-Disposition with quoted and percent escaped parameters.
var mpwHeaders = [][][2]string{
	{{"Content-Disposition", `form-data; name="a"`}},
	{{"Content-Disposition", `form-data; name="a"; filename="a.txt"`}},
	{{"Content-Disposition", `form-data; name="f name"; filename="dir/b.bin"`}, {"Content-Type", "application/octet-stream"}},
	{{"Content-Disposition", `form-data; name="q%22x"`}},
	{{"Content-Disposition", "form-data; name=\"\xff\xfe\""}},
	{{"Content-Disposition", `form-data; name="x"; filename="C:\x\y.png"`}},
	{{"Content-Disposition", `attachment; filename="a b"`}, {"X-Folded", "one\r\n two"}},
	{{"content-disposition", `form-data; name="lower"`}},
	{{"Content-Disposition", ""}},
	{{"Bad Name", "v"}, {"Content-Disposition", `form-data; name="bad key"`}},
	{{"Content-Disposition", `form-data; name="é日"`}, {"Content-Transfer-Encoding", "quoted-printable"}},
}

// mpwData are the bodies a written part carries: empty, text, bytes that
// are not UTF-8, a boundary like line that must not close the part, CRLF
// and a lone LF, and a long run.
var mpwData = [][]byte{
	{}, []byte("hello"), []byte("line1\r\nline2\r\n"), []byte("lf\nonly\n"),
	{0x00, 0xff, 0xfe}, []byte("--not-a-boundary\r\n"), []byte("é日"), bytes.Repeat([]byte("x"), 300),
}

// multipartWriteTables emits what multipart.write must produce for parts
// whose headers it copies, against Go's multipart.Writer, and what
// form_data_content_type must produce for a boundary Go's Writer would
// quote or not.
func multipartWriteTables(r *rand.Rand) string {
	var o strings.Builder
	o.WriteString("\n/// (parts: (name, has file, file, headers, data), boundary, Go's multipart.Writer bytes as hex)\nconst WRITE: List[(List[(Str, Bool, Str, List[(Str, Str)], Bytes)], Str, Str)] = [\n")
	for i := 0; i < 500; i++ {
		boundary := []string{"xyz", "MyBoundary", "----WebKitFormBoundary7MA4YWxkTrZu0gW", "1234567890", "=_qp", "a.b-c_d"}[r.Intn(6)]
		parts := make([]mpwPart, 0, 3)
		for k, n := 0, 1+r.Intn(3); k < n; k++ {
			p := mpwPart{
				name:    []string{"a", "field", "f name", "é", "x\"y", ""}[r.Intn(6)],
				hasFile: r.Intn(2) == 0,
				file:    []string{"a.txt", "dir/b.bin", `C:\x\y.png`, "", "é.txt"}[r.Intn(5)],
				headers: mpwHeaders[r.Intn(len(mpwHeaders))],
				data:    mpwData[r.Intn(len(mpwData))],
			}
			parts = append(parts, p)
		}
		var ps []string
		for _, p := range parts {
			ps = append(ps, mpwPartText(p))
		}
		fmt.Fprintf(&o, "  ([%s], %s, %s),\n", strings.Join(ps, ", "), xq(boundary), xq(hex.EncodeToString(mpwWrite(parts, boundary))))
	}
	o.WriteString("]\n\n/// (boundary, Go's Writer.FormDataContentType)\nconst FDC: List[(Str, Str)] = [\n")
	for _, b := range []string{
		"xyz", "1234567890", "=_qp", "a.b-c_d", "a+b", "a_b", "a'b",
		"----WebKitFormBoundary7MA4YWxkTrZu0gW", "ABCDEF0123456789",
		"a-b_c.d+e", "0", "1234567890123456789012345678901234567890123456789012345678901234567890",
	} {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if err := w.SetBoundary(b); err != nil {
			// Go's Writer refuses a boundary it cannot write; Xo's
			// form_data_content_type takes any text, so there is nothing
			// to compare this row against
			continue
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(textOf(b)), xq(textOf(w.FormDataContentType())))
	}
	o.WriteString("]\n")
	return o.String()
}

const multipartTests = `
fn part_text(p: multipart.Part) -> Str {
  let hs = p.headers.keys().map(fn(k) { "${k}=${p.headers.get(k) ?? ""}" }).sort()
  "${p.name}|${p.filename ?? "-"}|${hs.join(";")}|${p.data.len()} ${crypto.sha256(p.data)[0..4].to_hex()}"
}

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

// The rows the corpus needs: a regeneration that loses rows fails here
// instead of quietly comparing less.
test "parse gives Go's parts and errors" {
  expect(BODIES.len() >= 1500)
  var bad: List[(Int, List[Str], List[Str])] = []
  for (i, (body, boundary, _, want, _)) in BODIES.indexed() {
    // the parts before an error are not returned: compare the error alone
    let got = match multipart.parse(body, boundary) {
      Ok(ps) => ps.map(part_text)
      Err(e) => ["err ${e.display()}"]
    }
    let w = if want.len() > 0 && want[want.len() - 1].starts_with("err ") { [want[want.len() - 1]] } else { want }
    if got != w {
      bad.push((i, got, w))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse_form gives Go's values, files, and errors" {
  expect(BODIES.len() >= 1500)
  var bad: List[(Int, Str, Str)] = []
  for (i, (body, boundary, max_parts, _, want)) in BODIES.indexed() {
    let got = match multipart.parse_form(body, boundary, max_parts: max_parts) {
      Ok(f) => {
        let fs = f.files.keys().map(fn(k) { "${k}=[${(f.files.get(k) ?? []).map(part_text).join(", ")}]" }).sort()
        "ok ${f.values.encode()} files ${fs.join(" ")}"
      }
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

fn part_file(has_file: Bool, file: Str) -> Str? {
  if has_file {
    return Some(file)
  }
  None
}

fn write_part(name: Str, has_file: Bool, file: Str, hs: List[(Str, Str)], data: Bytes) -> multipart.Part {
  var m: Map[Str, Str] = {}
  for (k, v) in hs {
    m.put(k, v)
  }
  multipart.Part{name: name, filename: part_file(has_file, file), headers: m, data: data}
}

test "write produces Go's multipart.Writer bytes for the headers it copies" {
  expect(WRITE.len() >= 500)
  var bad: List[(Int, Str, Str)] = []
  for (i, (parts, boundary, want)) in WRITE.indexed() {
    let ps = parts.map(fn(p) {
      let (name, has_file, file, hs, data) = p
      write_part(name, has_file, file, hs, data)
    })
    let got = multipart.write(ps, boundary).to_hex()
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "a part without a Content-Disposition is written so Go's reader reads it back" {
  // Xo synthesizes the header (std/multipart.write), so there is no Go
  // Writer output to compare: the check is that what Xo writes parses back
  // to the same name, file name and data. A name with CR or LF never comes
  // back the same: Xo's write escapes those as %0D and %0A so the header
  // stays well formed, and Go's mime reader, which does not percent decode
  // a quoted value, reads the escaped text back. That escape is the
  // documented behavior and is pinned here on purpose.
  var bad: List[(Str, Str, Str, Str, Str)] = []
  for (name, want_name) in [
    ("a", "a"),
    ("f name", "f name"),
    ("é日", "é日"),
    ("x\"y", "x\"y"),
    ("a\rb", "a%0Db"),
    ("a\nb", "a%0Ab"),
    ("", ""),
  ] {
    let p = write_part(name, true, "dir/é.txt", [], b"\x00\xff")
    let body = multipart.write([p], "xyz")
    let got = match multipart.parse(body, "xyz") {
      Ok(ps) if ps.len() == 1 => {
        let q = ps[0]
        "${q.name}|${q.filename ?? "-"}|${q.data.to_hex()}|${q.headers.get("Content-Type") ?? "-"}"
      }
      Ok(_) => "count"
      Err(e) => "err ${e.display()}"
    }
    let want = "${want_name}|é.txt|00ff|application/octet-stream"
    if got != want {
      bad.push((name, got, want, "", ""))
    }
  }
  expect(first_bad(bad)) == []
}

test "form_data_content_type is Go's Writer.FormDataContentType" {
  // Go's Writer refuses a boundary it cannot write, and Xo's
  // form_data_content_type takes any text, so only the boundaries Go
  // accepted are rows here (12 at the seed).
  expect(FDC.len() >= 12)
  var bad: List[(Str, Str, Str)] = []
  for (boundary, want) in FDC {
    let got = multipart.form_data_content_type(boundary)
    if got != want {
      bad.push((boundary, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
