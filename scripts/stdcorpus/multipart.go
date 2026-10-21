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
	o.WriteString(multipartTests)
	return o.String()
}

const multipartTests = `
fn part_text(p: multipart.Part) -> Str {
  let hs = p.headers.keys().map(fn(k) { "${k}=${p.headers.get(k) ?? ""}" }).sort()
  "${p.name}|${p.filename ?? "-"}|${hs.join(";")}|${p.data.len()} ${crypto.sha256(p.data)[0..4].to_hex()}"
}

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "parse gives Go's parts and errors" {
  var bad: List[(Int, List[Str], List[Str])] = []
  for (i, (body, boundary, _, want, _)) in BODIES.indexed() {
    // the parts before an error are not returned: compare the error alone
    let got = match multipart.parse(body, boundary) {
      Ok(ps) => ps.map(part_text)
      Err(e) => ["err ${e.msg}"]
    }
    let w = if want.len() > 0 && want[want.len() - 1].starts_with("err ") { [want[want.len() - 1]] } else { want }
    if got != w {
      bad.push((i, got, w))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse_form gives Go's values, files, and errors" {
  var bad: List[(Int, Str, Str)] = []
  for (i, (body, boundary, max_parts, _, want)) in BODIES.indexed() {
    let got = match multipart.parse_form(body, boundary, max_parts: max_parts) {
      Ok(f) => {
        let fs = f.files.keys().map(fn(k) { "${k}=[${(f.files.get(k) ?? []).map(part_text).join(", ")}]" }).sort()
        "ok ${f.values.encode()} files ${fs.join(" ")}"
      }
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((i, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
