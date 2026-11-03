// A fuzz harness for std/mail against Go's net/mail (2026-11-02, decision
// 0461: a parser of untrusted input is Early on testing until it is
// fuzzed).
//
// std/mail is Go's net/mail written in Xo on every backend (decisions
// 0282 and 0312): parse_address and parse_address_list are ParseAddress
// and ParseAddressList, Address.display is Address.String, read_message is
// ReadMessage, parse_date is ParseDate, with Go's error texts. The answers
// come from net/mail.
//
// Each case is one of: an address or a list of them built from what a
// hostile sender writes (plain, quoted and escaped display names, RFC 2047
// encoded words in utf-8, iso-8859-1, us-ascii, an unsupported charset or
// malformed, comments, groups, angle brackets, local parts quoted or with
// dots in the wrong place, domain literals, non-ASCII domains, missing or
// doubled parts), cut short or with a byte changed; a message header block
// (canonical and odd case names, repeated fields, folded lines, lines with
// no colon or a space before it, empty values, LF or CRLF, a missing blank
// line) and a body; or a date in the forms ParseDate takes and ones it
// refuses.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/mailfuzz/mailfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestMailFuzzCorpusCurrent checks it is what this
// generator writes (XO_MAILFUZZ_WRITE=1 rewrites it). TestMailFuzz runs a
// fresh seed (XO_MAILFUZZ_SEED, default 2; XO_MAILFUZZ_N cases, default
// 2000) on XO_MAILFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"fmt"
	"io"
	"math/rand"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// mlCase is one call and Go's answer.
type mlCase struct {
	kind string // "addr", "list", "msg", "date"
	text string
	want string
}

var mlNames = []string{"Ann Lee", "\"Lee, Ann\"", `"Ann \"The\" Lee"`, `"a\\b"`, "=?utf-8?q?Ann_L=C3=A9e?=", "=?UTF-8?B?QW5uIEzDqWU=?=", "=?iso-8859-1?q?caf=E9?=", "=?us-ascii?q?plain?=", "=?koi8-r?q?x?=", "=?utf-8?x?y?=", "=?utf-8?q?unterminated", "=?utf-8?q?a?= =?utf-8?q?b?=", "Ann (comment) Lee", "Zoë", "", "\"\"", "Ann.Lee", "Ann, Lee", "Ann <x"}
var mlSpecs = []string{"ann@example.com", "a.b+c@sub.example.org", "\"quoted local\"@x.com", "\"a\\\"b\"@x.com", "a@[127.0.0.1]", "a@[bad", "a@", "@b", "a b@c", "a..b@c", ".a@c", "a.@c", "a@b..c", "a@b.c.", "ann@例え.jp", "ünï@x.com", "a", "a@b@c", "a@b", "a@-b.com", "a@b.c (comment)"}

var mlGoodNames = []string{"Ann Lee", "\"Lee, Ann\"", "=?utf-8?q?Ann_L=C3=A9e?=", "Zoë", "\"\""}
var mlGoodSpecs = []string{"ann@example.com", "a.b+c@sub.example.org", "\"quoted local\"@x.com", "a@[127.0.0.1]", "ünï@x.com"}

func mlPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// mlGoodAddr is a well-formed address, for most list members.
func mlGoodAddr(r *rand.Rand) string {
	if r.Intn(2) == 0 {
		return mlPick(r, mlGoodSpecs)
	}
	return mlPick(r, mlGoodNames) + " <" + mlPick(r, mlGoodSpecs) + ">"
}

func mlAddr(r *rand.Rand) string {
	switch r.Intn(6) {
	case 0:
		return mlPick(r, mlSpecs)
	case 1:
		return mlPick(r, mlSpecs) + " (" + mlPick(r, []string{"Ann", "a (nested) c", "", "unterminated"}) + ")"
	case 2:
		return "<" + mlPick(r, mlSpecs) + ">"
	default:
		return mlPick(r, mlNames) + mlPick(r, []string{" ", "", "  "}) + "<" + mlPick(r, mlSpecs) + ">"
	}
}

func mlList(r *rand.Rand) string {
	var parts []string
	for i, n := 0, 1+r.Intn(3); i < n; i++ {
		a := mlGoodAddr(r)
		if r.Intn(6) == 0 {
			a = mlAddr(r)
		}
		if r.Intn(6) == 0 {
			parts = append(parts, mlPick(r, []string{"team", "Team Name", "\"q\"", ""})+": "+a+mlPick(r, []string{", " + mlGoodAddr(r), ""})+mlPick(r, []string{";", ";", "", ";;"}))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, mlPick(r, []string{", ", ", ", ",", " , ", ";", ",,"}))
}

var mlFields = []string{"From: a@b.c", "To: x@y.z", "subject: hi", "SUBJECT: Hello", "X-Custom:  value  ", "x-custom: second", "Content-Type: text/plain; charset=utf-8", "Subject: é", "X-Empty:", "X-Empty2: ", "Received: from a\n by b", "Received: from a\n\tby b\n  with c", "bad line", ": no name", "Name : spaced", "Na me: x", "X-Ü: v", "\tleading fold", "Date: Mon, 02 Jan 2006 15:04:05 -0700", "Message-ID: <id@x>", "k:v"}

func mlMessage(r *rand.Rand) string {
	var lines []string
	for i, n := 0, r.Intn(6); i < n; i++ {
		f := mlFields[r.Intn(len(mlFields))]
		if r.Intn(2) == 0 { // half the lines well formed
			for strings.HasPrefix(f, "bad") || strings.HasPrefix(f, ":") || strings.HasPrefix(f, "\t") || strings.Contains(f, " :") || strings.HasPrefix(f, "Na me") {
				f = mlFields[r.Intn(len(mlFields))]
			}
		}
		lines = append(lines, f)
	}
	eol := mlPick(r, []string{"\n", "\n", "\r\n"})
	head := strings.Join(lines, eol)
	head = strings.ReplaceAll(head, "\n", eol)
	if eol == "\r\n" {
		head = strings.ReplaceAll(head, "\r\r\n", "\r\n")
	}
	switch r.Intn(8) {
	case 0: // no blank line, no body
		return head
	case 1: // a blank line only
		return head + eol + eol
	default:
		return head + eol + eol + mlPick(r, []string{"body", "line one\nline two\n", "", "From: not a header\n", "é\r\n"})
	}
}

var mlDates = []string{
	"Mon, 02 Jan 2006 15:04:05 -0700", "02 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 +0000 (UTC)", "2 Jan 06 15:04 GMT", "Mon, 02 Jan 2006 15:04 EST",
	"Tue, 1 Jul 2003 10:52:37 +0200", "Fri, 21 Nov 1997 09:55:06 -0600", "Thu, 13 Feb 1969 23:32:54 -0330", "Mon, 02 Jan 2006 15:04:05.123 -0700", "Mon, 02 Jan 2006 15:04:05 PDT",
	"Mon, 02 Jan 2006 15:04:05 UT", "Mon, 02 Jan 2006 15:04:05 Z", "Mon, 02 Jan 2006 15:04:05 +2500", "Mon, 32 Jan 2006 15:04:05 -0700", "Mon, 02 Foo 2006 15:04:05 -0700",
	"Mon, 02 jan 2006 15:04:05 -0700", "  Mon,  02  Jan  2006  15:04:05  -0700  ", "Mon, 02 Jan 2006", "Mon, 02 Jan 2006 25:04:05 -0700", "Mon 02 Jan 2006 15:04:05 -0700",
	"", "Mon, 02 Jan 2006 15:04:05 -07", "Mon, 02 Jan 2006 15:04:05 -0700 (comment (nested))", "Sat, 29 Feb 2020 00:00:00 +0000", "Sat, 29 Feb 2021 00:00:00 +0000",
	"Mon, 02 Jan 2006 15:04:05 +3", "Mon, 02 Jan 2006 15:04:05 -23", "Mon, 02 Jan 2006 15:04:05 -24", "Mon, 02 Jan 2006 15:04:05 GMT+3", "Mon, 02 Jan 2006 15:04:05 GMT-24", "Mon, 02 Jan 2006 15:04:05 CEST", "Mon, 02 Jan 2006 15:04:05 ChST",
}

// mlMutate cuts s short or changes a byte, keeping it UTF-8.
func mlMutate(r *rand.Rand, s string) string {
	b := []byte(s)
	switch r.Intn(8) {
	case 0:
		b = b[:r.Intn(len(b)+1)]
	case 1:
		if len(b) > 0 {
			b[r.Intn(len(b))] = "<>@,;:\"()\\. ="[r.Intn(13)]
		}
	}
	return string(b)
}

func mlAddrText(a *mail.Address) string {
	return fmt.Sprintf("%x|%x|%x", a.Name, a.Address, a.String())
}

func mlAnswer(c *mlCase) {
	switch c.kind {
	case "addr":
		a, err := mail.ParseAddress(c.text)
		if err != nil {
			c.want = "err " + err.Error()
			return
		}
		c.want = "ok " + mlAddrText(a)
	case "list":
		as, err := mail.ParseAddressList(c.text)
		if err != nil {
			c.want = "err " + err.Error()
			return
		}
		var parts []string
		for _, a := range as {
			parts = append(parts, mlAddrText(a))
		}
		c.want = "ok " + strings.Join(parts, ";")
	case "msg":
		m, err := mail.ReadMessage(strings.NewReader(c.text))
		if err != nil {
			c.want = "err " + err.Error()
			return
		}
		body, _ := io.ReadAll(m.Body)
		keys := make([]string, 0, len(m.Header))
		for k := range m.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var o strings.Builder
		o.WriteString("ok ")
		for _, k := range keys {
			fmt.Fprintf(&o, "%x=", k)
			for _, v := range m.Header[k] {
				fmt.Fprintf(&o, "%x,", v)
			}
			o.WriteString(";")
		}
		fmt.Fprintf(&o, "|%x", body)
		c.want = o.String()
	default:
		t, err := mail.ParseDate(c.text)
		if err != nil {
			c.want = "err " + err.Error()
			return
		}
		c.want = fmt.Sprintf("ok %d", t.UnixMilli())
	}
}

func mlCases(r *rand.Rand, n int) []mlCase {
	// time.Parse gives a zone abbreviation the offset it has in
	// time.Local (EST is -0500 on a New York machine) and zero otherwise;
	// std/mail has no local zone, so Go answers as on a UTC machine
	local := time.Local
	time.Local = time.UTC
	defer func() { time.Local = local }()
	cases := make([]mlCase, 0, n)
	for len(cases) < n {
		var c mlCase
		switch k := r.Intn(10); {
		case k < 4:
			c.kind, c.text = "addr", mlMutate(r, mlAddr(r))
		case k < 6:
			c.kind, c.text = "list", mlList(r)
			if r.Intn(3) == 0 {
				c.text = mlMutate(r, c.text)
			}
		case k < 8:
			c.kind, c.text = "msg", mlMessage(r)
		default:
			c.kind, c.text = "date", mlMutate(r, mlPick(r, mlDates))
		}
		if !utf8.ValidString(c.text) {
			continue
		}
		mlAnswer(&c)
		if !utf8.ValidString(c.want) {
			continue // an error quoting a cut rune: Xo's Str cannot hold it
		}
		cases = append(cases, c)
	}
	return cases
}

const mlRenderXo = `use std/mail

fn ml_addr_text(a: mail.Address) -> Str {
  a.name.bytes().to_hex() + "|" + a.address.bytes().to_hex() + "|" + a.display().bytes().to_hex()
}

fn ml_addr(s: Str) -> Str {
  match mail.parse_address(s) {
    Ok(a) => "ok " + ml_addr_text(a)
    Err(e) => "err " + e.display()
  }
}

fn ml_list(s: Str) -> Str {
  match mail.parse_address_list(s) {
    Ok(l) => {
      var parts: List[Str] = []
      for a in l {
        parts.push(ml_addr_text(a))
      }
      "ok " + parts.join(";")
    }
    Err(e) => "err " + e.display()
  }
}

fn ml_msg(s: Str) -> Str {
  match mail.read_message(s.bytes()) {
    Ok(m) => {
      var keys: List[Str] = []
      for (k, _) in m.headers {
        if !keys.contains(k) {
          keys.push(k)
        }
      }
      var out = "ok "
      for k in keys.sort() {
        out = out + k.bytes().to_hex() + "="
        for (k2, v) in m.headers {
          if k2 == k {
            out = out + v.bytes().to_hex() + ","
          }
        }
        out = out + ";"
      }
      out + "|" + m.body.to_hex()
    }
    Err(e) => "err " + e.display()
  }
}

fn ml_date(s: Str) -> Str {
  match mail.parse_date(s) {
    Ok(t) => "ok ${t.unix_millis()}"
    Err(e) => "err " + e.display()
  }
}

fn expect(got: Str, want: Str) {
  if got != want {
    fault("got " + got)
  }
}

`

// mlXoFile is the generated test file: the helpers and one test per case.
func mlXoFile(seed int64, cases []mlCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/mailfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// net/mail; do not edit: XO_MAILFUZZ_WRITE=1 go test ./internal/stdtest -run TestMailFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(mlRenderXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"mail fuzz %d %s\" {\n  expect(ml_%s(%s), %s)\n}\n\n", i, c.kind, c.kind, xoStr(c.text), xoStr(c.want))
	}
	return b.String()
}

// mlCoverage fails when the cases stop reaching the parsers' parts.
func mlCoverage(t *testing.T, cases []mlCase) {
	t.Helper()
	ok := map[string]int{}
	count := map[string]int{}
	msgs := map[string]int{}
	for _, c := range cases {
		count[c.kind]++
		if strings.HasPrefix(c.want, "ok ") {
			ok[c.kind]++
		} else {
			m := c.want
			if i := strings.IndexAny(m, ":"); i > 0 && strings.HasPrefix(m, "err mail: ") {
				if j := strings.Index(m[i+1:], ":"); j > 0 {
					m = m[:i+1+j]
				}
			}
			msgs[m]++
		}
	}
	t.Logf("cases %v, ok %v, %d distinct errors", count, ok, len(msgs))
	for m, n := range msgs {
		t.Logf("  %4d %.100s", n, m)
	}
	for _, k := range []string{"addr", "list", "msg", "date"} {
		if ok[k] < count[k]/5 || count[k]-ok[k] < count[k]/10 {
			t.Fatalf("the %s cases stopped covering std/mail: %d of %d ok", k, ok[k], count[k])
		}
	}
	if len(msgs) < 15 {
		t.Fatalf("only %d distinct errors", len(msgs))
	}
}

const mlCorpus = "testdata/corpus/mailfuzz/mailfuzz_test.xo"

// TestMailFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestMailFuzzCorpusCurrent(t *testing.T) {
	cases := mlCases(rand.New(rand.NewSource(1)), 2000)
	mlCoverage(t, cases)
	text := mlXoFile(1, cases)
	if os.Getenv("XO_MAILFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(mlCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mlCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(mlCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's net/mail or the generator changed): XO_MAILFUZZ_WRITE=1 go test ./internal/stdtest -run TestMailFuzzCorpusCurrent", mlCorpus)
	}
}

// TestMailFuzz runs a fresh seed's cases.
func TestMailFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the mail fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_MAILFUZZ_SEED", 2))
	n := envInt(t, "XO_MAILFUZZ_N", 2000)
	cases := mlCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d %s %q", i, c.kind, c.text)
	}
	runFuzzDir(t, "mailfuzz_test.xo", mlXoFile(seed, cases), envBackends("XO_MAILFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
