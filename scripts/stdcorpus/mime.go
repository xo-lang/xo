package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	mimeWordCorpus(&o)
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

// mimeWordCorpus: Go's mime WordEncoder.Encode, WordDecoder.Decode and
// DecodeHeader on encodedword_test.go's inputs (those that are UTF-8, as
// a Str is) and on seeded random ones; a result that is not UTF-8 is
// written with U+FFFD per bad byte, as std/mime gives it.
func mimeWordCorpus(o *strings.Builder) {
	r := rand.New(rand.NewSource(2047))
	fix := func(s string) string { return string([]rune(s)) }
	enc := []string{
		"François-Jérôme", "Café", "La Seleção", "", "A", "a", "123 456",
		"\t !\"#$%&'()*+,-./ :;<>?@[\\]^_`{|}~", strings.Repeat("é", 10), strings.Repeat("é", 11),
		strings.Repeat("à", 30), strings.Repeat("é", 60), strings.Repeat("ï", 25), strings.Repeat("ô", 37),
		"{$firstname} Bienvendio a Apostolica, aquà inicia el camino de tu", "日本語のテキスト", "a\x00b", "tab\tonly", "line\nbreak",
	}
	pieces := []string{"a", " ", "_", "=", "?", "é", "日", "😀", "\t", "\n", "z", "Q", "~", "\x7f"}
	for i := 0; i < 60; i++ {
		var b strings.Builder
		for n := r.Intn(70); n > 0; n-- {
			b.WriteString(pieces[r.Intn(len(pieces))])
		}
		enc = append(enc, b.String())
	}
	o.WriteString("\n/// encode_word: (s, charset, b, Go's WordEncoder.Encode)\nconst WORD_ENC: List[(Str, Str, Bool, Str)] = [\n")
	for _, src := range enc {
		for _, cs := range []string{"utf-8", "iso-8859-1", "UTF-8"} {
			for _, e := range []mime.WordEncoder{mime.QEncoding, mime.BEncoding} {
				fmt.Fprintf(o, "  (%s, %q, %v, %s),\n", xq(src), cs, e == mime.BEncoding, xq(e.Encode(cs, src)))
			}
		}
	}
	o.WriteString("]\n")
	words := []string{
		"=?UTF-8?Q?=C2=A1Hola,_se=C3=B1or!?=", "=?UTF-8?Q?Fran=C3=A7ois-J=C3=A9r=C3=B4me?=", "=?UTF-8?q?ascii?=", "=?utf-8?B?QW5kcsOp?=",
		"=?ISO-8859-1?Q?Rapha=EBl_Dupont?=", "=?utf-8?b?IkFudG9uaW8gSm9zw6kiIDxqb3NlQGV4YW1wbGUub3JnPg==?=", "=?UTF-8?A?Test?=",
		"=?UTF-8?Q?A=B?=", "=?UTF-8?Q?=A?=", "=?UTF-8?A?A?=", "=????=", "=?UTF-8???=", "=?UTF-8?Q??=",
		"=?us-ascii?q?a=FFb?=", "=?utf-8?q?=80=C3?=", "=?koi8-r?q?x?=", "=?UTF-8?b?QW5kcs?=", "=?UTF-8?q?=g0?=", "=?UTF-8?q?a\x01?=",
		"=?UTF-8?q?a=0D=0A?=", "=?UTF-8?q?tab\there?=", "=?a?q?b?c?=", "Jean", "=?UTF-8?Q?x", "=?ISO-8859-1?B?6enp?=",
	}
	headers := append([]string{}, words...)
	headers = append(headers, "=?", "=?UTF-8?", "=?UTF-8?=", "=?UTF-8?Q", "=?UTF-8?Q?", "=?UTF-8?Q?=", "=?UTF-8?Q?A", "=?UTF-8?Q?A?",
		"=?ISO-8859-1?Q?a?=", "=?ISO-8859-1?Q?a?= b", "=?ISO-8859-1?Q?a?= =?ISO-8859-1?Q?b?=", "=?ISO-8859-1?Q?a?=  =?ISO-8859-1?Q?b?=",
		"=?ISO-8859-1?Q?a?= \r\n\t =?ISO-8859-1?Q?b?=", "=?ISO-8859-1?Q?a_b?=",
		"=?UTF-8?b?garbage?= =?UTF-8?b?QW5kcsOp?= =?UTF-8?b?garbage?=", "=?UTF-8?b?QW5kcsOp", "=?UTF-8?x?y?=?UTF-8?x?y=?",
		"Re: =?utf-8?q?caf=C3=A9?= x =?utf-8?b?w6k=?=", "a =?koi8-r?q?x?= b", "pre=?utf-8?q?a?=post")
	for i := 0; i < 40; i++ {
		var b strings.Builder
		for n := 1 + r.Intn(4); n > 0; n-- {
			src := enc[r.Intn(len(enc))]
			e := []mime.WordEncoder{mime.QEncoding, mime.BEncoding}[r.Intn(2)]
			b.WriteString(e.Encode("utf-8", src))
			b.WriteString([]string{" ", "  ", "\r\n ", " x ", ""}[r.Intn(5)])
		}
		headers = append(headers, b.String())
	}
	dec := new(mime.WordDecoder)
	res := func(s string, err error) string {
		if err != nil {
			return "ERR " + err.Error()
		}
		return fix(s)
	}
	o.WriteString("\n/// decode_word: (word, Go's Decode: the text, or ERR and the error)\nconst WORD_DEC: List[(Str, Str)] = [\n")
	for _, w := range words {
		fmt.Fprintf(o, "  (%s, %s),\n", xq(w), xq(res(dec.Decode(w))))
	}
	o.WriteString("]\n\n/// decode_header: (header, Go's DecodeHeader)\nconst HEADER_DEC: List[(Str, Str)] = [\n")
	for _, h := range headers {
		fmt.Fprintf(o, "  (%s, %s),\n", xq(h), xq(res(dec.DecodeHeader(h))))
	}
	o.WriteString("]\n")
	// a CharsetReader (TestStdAPICovered, 2026-11-16): each other charset
	// as <charset:hex of the bytes>, and charset "fail" an error
	withDec := &mime.WordDecoder{CharsetReader: func(charset string, in io.Reader) (io.Reader, error) {
		if charset == "fail" {
			return nil, errors.New("Test error")
		}
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		return strings.NewReader("<" + charset + ":" + hex.EncodeToString(b) + ">"), nil
	}}
	withWords := append([]string{"=?KOI8-R?B?8sXT?=", "=?windows-1252?Q?b=E0r?=", "=?ISO-8859-15?Q?f=F5=F6?=", "=?fail?q?x?=", "=?FAIL?b?eA==?=", "=?x-unknown?q?=FF_=00?="}, words...)
	withHeaders := append([]string{"=?ISO-8859-15?Q?f=F5=F6?=  =?windows-1252?Q?b=E0r?=", "a =?koi8-r?q?x?= b =?fail?q?y?=", "=?big5?b?pqjB2Q==?= =?utf-8?q?ok?="}, headers...)
	withHeaders = append(withHeaders, withWords...)
	o.WriteString("\n/// decode_word_with: (word, Go's WordDecoder with that CharsetReader: the text, or ERR and the error)\nconst WORD_DEC_WITH: List[(Str, Str)] = [\n")
	for _, w := range withWords {
		fmt.Fprintf(o, "  (%s, %s),\n", xq(w), xq(res(withDec.Decode(w))))
	}
	o.WriteString("]\n\n/// decode_header_with: (header, Go's DecodeHeader with that CharsetReader)\nconst HEADER_DEC_WITH: List[(Str, Str)] = [\n")
	for _, h := range withHeaders {
		fmt.Fprintf(o, "  (%s, %s),\n", xq(h), xq(res(withDec.DecodeHeader(h))))
	}
	o.WriteString("]\n")
	o.WriteString(`
fn word_res(r: Result[Str, mime.MimeErr]) -> Str {
  match r {
    Ok(s) => s
    Err(e) => "ERR " + e.display()
  }
}

test "encode_word, decode_word and decode_header give Go's mime results" {
  var bad: List[Str] = []
  for (s, cs, b, want) in WORD_ENC {
    let got = mime.encode_word(s, charset: cs, encoding: if b { mime.WordEncoding.B } else { mime.WordEncoding.Q })
    if got != want {
      bad.push("enc ${s.quote()} ${cs} ${b}: ${got.quote()}")
    }
    // Go splits only UTF-8 words (TestEncodedWordLength)
    for w in got.split(" ") {
      if cs.to_lower() == "utf-8" && w.len() > 75 {
        bad.push("long ${w}")
      }
    }
  }
  for (w, want) in WORD_DEC {
    let got = word_res(mime.decode_word(w))
    if got != want {
      bad.push("word ${w.quote()}: ${got.quote()}")
    }
  }
  for (h, want) in HEADER_DEC {
    let got = word_res(mime.decode_header(h))
    if got != want {
      bad.push("header ${h.quote()}: ${got.quote()}")
    }
  }
  expect(first_bad(bad)) == []
}

fn show(charset: Str, content: Bytes) -> Str ! mime.MimeErr {
  if charset == "fail" {
    return Err(mime.MimeErr{msg: "Test error"})
  }
  Ok("<${charset}:${content.to_hex()}>")
}

test "decode_word_with and decode_header_with, as Go's WordDecoder with a CharsetReader" {
  var bad: List[Str] = []
  for (w, want) in WORD_DEC_WITH {
    let got = word_res(mime.decode_word_with(w, show))
    if got != want {
      bad.push("word ${w.quote()}: ${got.quote()}")
    }
  }
  for (h, want) in HEADER_DEC_WITH {
    let got = word_res(mime.decode_header_with(h, show))
    if got != want {
      bad.push("header ${h.quote()}: ${got.quote()}")
    }
  }
  expect(first_bad(bad)) == []
}
`)
}
