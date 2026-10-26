package main

import (
	"fmt"
	"math/rand"
	"mime"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"
)

// mimeParse is std/rt's parse_media_type: the kind and the UTF-8 params
// sorted, or err TEXT.
func mimeParse(v string) string {
	kind, ps, err := mime.ParseMediaType(v)
	if err != nil {
		return "err " + textOf(err.Error())
	}
	var ks []string
	for k, p := range ps {
		if utf8.ValidString(p) {
			ks = append(ks, k+"="+p)
		}
	}
	sort.Strings(ks)
	return "ok " + kind + ";" + strings.Join(ks, ";")
}

func mimeValue(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString([]string{"text/html", "multipart/form-data", "application/json", "TEXT/Plain", "image/*", "form-data", "attachment", "text/html", "form-data", "text/", "/x", "a/b/c", "x y/z", "ü/x", ""}[r.Intn(15)])
	for i, n := 0, r.Intn(4); i < n; i++ {
		b.WriteString([]string{"; ", "; ", ";", " ;", ";;", ""}[r.Intn(6)])
		name := []string{"charset", "boundary", "name", "filename", "Name", "a", "q", "title*", "title*0", "title*1", "title*0*", "title*1*", "charset", "name", "x y", ""}[r.Intn(16)]
		b.WriteString(name)
		b.WriteString([]string{"=", "=", "=", "=", "=", " = ", ""}[r.Intn(7)])
		switch r.Intn(9) {
		case 0, 1, 6, 7:
			b.WriteString([]string{"utf-8", "abc", "a.b", "--x", "1", "é", "a b", "日本", "�", "a\tb", "a\r\nb", "a\x00b", "a\u0080b"}[r.Intn(13)])
		case 2, 3:
			b.WriteString("\"")
			for j, m := 0, r.Intn(4); j < m; j++ {
				b.WriteString([]string{"a", " ", "b", "c d", "\\\"", "\\\\", ";", "é", "\\", "=", "\x00", "\xff", "\x1b", "\u0080", "日", "�", "\r\n ", "\t"}[r.Intn(18)])
			}
			if r.Intn(6) > 0 {
				b.WriteString("\"")
			}
		case 4:
			b.WriteString([]string{"UTF-8''caf%C3%A9", "us-ascii'en'a%20b", "iso-8859-1''%E9", "utf-8''%zz", "''x", "UTF-8'en'%FF", "utf-8''a%2"}[r.Intn(7)])
		case 5:
			b.WriteString([]string{"%41", "x%", "a\"b", "\xff"}[r.Intn(4)])
		}
	}
	return b.String()
}

func mimeSniff(r *rand.Rand) []byte {
	pre := []string{"", "<!DOCTYPE HTML>", " \t\n<html", "<?xml", "%PDF-", "%!PS-Adobe-", "\xFE\xFF", "\xFF\xFE", "\xEF\xBB\xBF", "GIF87a", "GIF89a", "\x89PNG\x0D\x0A\x1A\x0A", "\xFF\xD8\xFF", "BM", "RIFF\x00\x00\x00\x00WEBPVP", "RIFF\x00\x00\x00\x00WAVE", "\x1F\x8B\x08", "PK\x03\x04", "Rar!\x1A\x07\x00", "\x00\x61\x73\x6D", "OggS\x00", "ID3", "\x1A\x45\xDF\xA3", "\x00\x00\x00\x18ftypmp42", "wOFF", "wOF2", "<SCRIPT", "<!--", "<p", "<br", "{\"a\":1}", "\x00\x01binary", "MThd\x00\x00\x00\x06", ".snd", "FORM\x00\x00\x00\x00AIFF", "\x00\x00\x01\xBA", "\x00\x00\x01\x00", "\x00\x00\x02\x00"}
	b := []byte(pre[r.Intn(len(pre))])
	for i, n := 0, r.Intn(40); i < n; i++ {
		switch r.Intn(4) {
		case 0:
			b = append(b, byte(r.Intn(256)))
		default:
			b = append(b, "abc <>\t\n\x1b"[r.Intn(9)])
		}
	}
	if r.Intn(5) == 0 && len(b) > 0 {
		b = b[:r.Intn(len(b))]
	}
	return b
}

func mimeCorpus() string {
	r := rand.New(rand.NewSource(6))
	var o strings.Builder
	o.WriteString(header("mime.ParseMediaType and FormatMediaType and net/http.DetectContentType as std/rt/mime.go calls them, on seeded random media types (RFC 2231 continuations and charsets, quoting, bad bytes) and sniffed byte prefixes"))
	o.WriteString("use std/mime\n\n/// (value, Go's result: ok KIND;k=v... sorted, or err TEXT)\nconst PARSE: List[(Str, Str)] = [\n")
	for i := 0; i < 4000; i++ {
		// a Str holds no invalid UTF-8: Go parses the text the Xo side gets
		v := textOf(mimeValue(r))
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(v), xq(textOf(mimeParse(v))))
	}
	o.WriteString("]\n\n/// (kind, params as k=v lines, Go's FormatMediaType)\nconst FORMAT: List[(Str, List[(Str, Str)], Str)] = [\n")
	for i := 0; i < 600; i++ {
		kind := []string{"text/html", "TEXT/HTML", "form-data", "a b/c", "text/", "x/y", "é/x", "multipart/mixed"}[r.Intn(8)]
		ps := map[string]string{}
		var pairs []string
		for j, n := 0, r.Intn(4); j < n; j++ {
			k := []string{"charset", "Boundary", "name", "a b", "q*", "x", "é"}[r.Intn(7)]
			v := []string{"utf-8", "a b", "\"q\"", "é", "", "x;y", "a\\b", "\x01"}[r.Intn(8)]
			if _, ok := ps[k]; ok {
				continue
			}
			ps[k] = v
			pairs = append(pairs, fmt.Sprintf("(%s, %s)", xq(k), xq(v)))
		}
		fmt.Fprintf(&o, "  (%s, [%s], %s),\n", xq(kind), strings.Join(pairs, ", "), xq(mime.FormatMediaType(kind, ps)))
	}
	o.WriteString("]\n\n/// (data, Go's DetectContentType)\nconst SNIFF: List[(Bytes, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		b := mimeSniff(r)
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(b), xq(http.DetectContentType(b)))
	}
	o.WriteString("]\n")
	o.WriteString(mimeTests)
	return o.String()
}

const mimeTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "parse_media_type gives Go's kind, params, and errors" {
  var bad: List[(Str, Str, Str)] = []
  for (v, want) in PARSE {
    let got = match mime.parse_media_type(v) {
      Ok(m) => "ok ${m.kind};${m.params.keys().sort().map(fn(k) { "${k}=${m.params.get(k) ?? ""}" }).join(";")}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((v, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "format_media_type writes Go's text" {
  var bad: List[(Str, Str, Str)] = []
  for (kind, pairs, want) in FORMAT {
    var ps: Map[Str, Str] = {}
    for (k, v) in pairs {
      ps[k] = v
    }
    let got = mime.format_media_type(kind, ps)
    if got != want {
      bad.push((kind, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "detect_content_type sniffs as Go's DetectContentType" {
  var bad: List[(Bytes, Str, Str)] = []
  for (data, want) in SNIFF {
    let got = mime.detect_content_type(data)
    if got != want {
      bad.push((data, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
