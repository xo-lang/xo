package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"mime/quotedprintable"
	"strings"
)

// qpCorpus: std/mime's qp_encode and qp_decode against Go's
// mime/quotedprintable on random inputs over the bytes its rules treat
// apart (=, hex digits, space, tab, CR, LF, controls, DEL, high bytes),
// with some long lines past 76 (the Writer) and 4096 (the Reader's bufio
// buffer).
func qpCorpus() string {
	r := rand.New(rand.NewSource(5))
	alpha := []byte("aZ=0F9fg \t\r\n\x80\xff~!")
	rare := []byte("\x00\x01\x7f")
	gen := func(long bool) []byte {
		n := r.Intn(40)
		if long {
			n = 4090 + r.Intn(12)
		} else if r.Intn(8) == 0 {
			n = 70 + r.Intn(20)
		}
		b := make([]byte, n)
		for i := range b {
			if n > 1000 {
				// a long line: no line break or refused byte, so the
				// buffer's limit (or the Writer's soft breaks) decides
				b[i] = 'a'
				if r.Intn(50) == 0 {
					b[i] = "=0F9 \t\x80"[r.Intn(7)]
				}
				continue
			}
			if r.Intn(60) == 0 {
				b[i] = rare[r.Intn(len(rare))]
				continue
			}
			b[i] = alpha[r.Intn(len(alpha))]
		}
		return b
	}
	var o strings.Builder
	o.WriteString(header("Go's mime/quotedprintable (BSD-3-Clause) on random inputs: decode's output or error, encode's text and Binary output"))
	o.WriteString("\nuse std/mime\n")
	o.WriteString("\n/// (input as hex, Go's decoded output as hex, or \"!\" and its error)\nconst QP_DECODE: List[(Str, Str)] = [\n")
	for i := 0; i < 412; i++ {
		in := gen(i%34 == 33)
		out, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(in)))
		want := fmt.Sprintf("%x", out)
		if err != nil {
			want = "!" + err.Error()
		}
		fmt.Fprintf(&o, "  (%q, %s),\n", fmt.Sprintf("%x", in), xoQuote(want))
	}
	o.WriteString("]\n\n/// (input as hex, Writer's output as hex, Binary Writer's output as hex)\nconst QP_ENCODE: List[(Str, Str, Str)] = [\n")
	for i := 0; i < 204; i++ {
		in := gen(i%34 == 33)
		var t, b bytes.Buffer
		w := quotedprintable.NewWriter(&t)
		w.Write(in)
		w.Close()
		wb := quotedprintable.NewWriter(&b)
		wb.Binary = true
		wb.Write(in)
		wb.Close()
		fmt.Fprintf(&o, "  (%q, %q, %q),\n", fmt.Sprintf("%x", in), fmt.Sprintf("%x", t.Bytes()), fmt.Sprintf("%x", b.Bytes()))
	}
	o.WriteString(`]

test "qp_decode matches Go's quotedprintable.Reader on random inputs" {
  for (input, want) in QP_DECODE {
    let got = match mime.qp_decode(Bytes.from_hex(input) ?? b"") {
      Ok(b) => b.to_hex()
      Err(e) => "!" + e.msg
    }
    expect(got) == want
  }
}

test "qp_encode matches Go's quotedprintable.Writer, text and Binary, on random inputs" {
  for (input, want, want_b) in QP_ENCODE {
    let b = Bytes.from_hex(input) ?? b""
    expect(mime.qp_encode(b).to_hex()) == want
    expect(mime.qp_encode(b, binary: true).to_hex()) == want_b
  }
}
`)
	return o.String()
}

// xoQuote is s as an Xo string literal (Go's %q escapes, and $).
func xoQuote(s string) string {
	return strings.ReplaceAll(fmt.Sprintf("%q", s), "$", `\$`)
}
