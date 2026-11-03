// A fuzz harness for std/csv against Go's encoding/csv (2026-11-02,
// decision 0461: a parser of untrusted input is Early on testing until it
// is fuzzed).
//
// std/csv is Go's csv.Reader and csv.Writer (decision 0298): the same
// records, the same errors with the same lines and columns. Each case is a
// text built from fields a hostile upload carries (quoted and bare, a
// quote inside a bare field, a quote run on after a closing quote, line
// breaks inside quotes, \r alone, \r\n, a separator inside quotes, empty
// fields and lines, leading space, comment lines, a multi-byte
// separator), cut short or with a byte changed, under Reader options
// drawn at random: Comma (',', ';', '\t', '|', 'é'), Comment (none or
// '#'), FieldsPerRecord (-1, 0, 2), LazyQuotes and TrimLeadingSpace.
//
// Each case is compared two ways, as Go answers them: csv.parse against
// ReadAll ("err" and the error's text exactly, or "ok" and every field as
// hex), and, when the read succeeds, csv.format of those records against
// csv.Writer (with UseCRLF drawn at random) byte for byte, except for a
// record that is one empty field, which std/csv quotes on purpose (its
// doc: so it reads back as a record).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/csvfuzz/csvfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestCsvFuzzCorpusCurrent checks it is what this
// generator writes (XO_CSVFUZZ_WRITE=1 rewrites it). TestCsvFuzz runs a
// fresh seed (XO_CSVFUZZ_SEED, default 2; XO_CSVFUZZ_N cases, default
// 2000) on XO_CSVFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// cfCase is one text, the options it is read with, and Go's answers.
type cfCase struct {
	text             string
	sep, comment     rune
	fields           int
	lazy, trim, crlf bool
	read, format     string
}

var cfSeps = []rune{',', ',', ',', ';', '\t', '|', 'é'}

var cfFields = []string{"a", "b", "", "", "1", " a", "  x", "a b", "é", `"q"`, `""`, `"a""b"`, `"a,b"`, `"a;b"`, "\"a\nb\"", "\"a\r\nb\"", `"`}
var cfBadFields = []string{`a"b`, `"a"b`, `"a" `, `"a`, "a\rb", `""x`, ` "a"`, "\"a\r\"", "\x00"}

func cfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// cfText is 0 to 5 records of 0 to 4 fields, with comment lines, empty
// lines and line ends drawn at random.
func cfText(r *rand.Rand, sep, comment rune) string {
	var b strings.Builder
	for i, n := 0, r.Intn(6); i < n; i++ {
		switch r.Intn(12) {
		case 0:
			if comment != 0 {
				b.WriteString(string(comment) + " note")
			}
		case 1: // an empty line
		default:
			for j, m := 0, r.Intn(5); j < m; j++ {
				if j > 0 {
					b.WriteRune(sep)
				}
				f := cfPick(r, cfFields)
				if r.Intn(10) == 0 {
					f = cfPick(r, cfBadFields)
				}
				f = strings.ReplaceAll(f, ",", string(sep))
				b.WriteString(f)
			}
		}
		if i < n-1 || r.Intn(2) == 0 {
			b.WriteString(cfPick(r, []string{"\n", "\n", "\r\n", "\r"}))
		}
	}
	return b.String()
}

// cfCases builds the seeded cases; one in ten is cut short and one in ten
// has a byte changed (a case that stops being UTF-8 is drawn again).
func cfCases(r *rand.Rand, n int) []cfCase {
	cases := make([]cfCase, 0, n)
	for len(cases) < n {
		c := cfCase{sep: cfSeps[r.Intn(len(cfSeps))], fields: []int{-1, -1, 0, 2}[r.Intn(4)]}
		if r.Intn(3) == 0 {
			c.comment = '#'
		}
		c.lazy, c.trim, c.crlf = r.Intn(4) == 0, r.Intn(4) == 0, r.Intn(2) == 0
		raw := []byte(cfText(r, c.sep, c.comment))
		switch r.Intn(10) {
		case 0:
			if len(raw) > 0 {
				raw = raw[:r.Intn(len(raw))]
			}
		case 1:
			if len(raw) > 0 {
				mutate := ",;\"\n\r #a"
				raw[r.Intn(len(raw))] = mutate[r.Intn(len(mutate))]
			}
		}
		if !utf8.Valid(raw) {
			continue
		}
		c.text = string(raw)
		c.read, c.format = cfGo(c)
		cases = append(cases, c)
	}
	return cases
}

// cfGo is Go's answer for c: the read, and the format of what was read
// ("" when the read failed or a record is one empty field).
func cfGo(c cfCase) (string, string) {
	rd := csv.NewReader(strings.NewReader(c.text))
	rd.Comma, rd.Comment, rd.FieldsPerRecord = c.sep, c.comment, c.fields
	rd.LazyQuotes, rd.TrimLeadingSpace = c.lazy, c.trim
	recs, err := rd.ReadAll()
	if err != nil {
		return "err " + err.Error(), ""
	}
	var parts []string
	for _, rec := range recs {
		var fs []string
		for _, f := range rec {
			fs = append(fs, fmt.Sprintf("%x", f))
		}
		parts = append(parts, "["+strings.Join(fs, ",")+"]")
	}
	read := "ok " + strings.Join(parts, "")
	for _, rec := range recs {
		if len(rec) == 1 && rec[0] == "" {
			return read, ""
		}
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma, w.UseCRLF = c.sep, c.crlf
	if err := w.WriteAll(recs); err != nil {
		panic(err)
	}
	return read, fmt.Sprintf("%x", b.Bytes())
}

const cfRenderXo = `use std/csv

fn cf_read(text: Str, sep: Rune, comment: Rune, fields: Int, lazy: Bool, trim: Bool) -> Str {
  match csv.parse(text, sep: sep, comment: comment, fields: fields, lazy_quotes: lazy, trim_leading_space: trim) {
    Ok(recs) => {
      var out = "ok "
      for rec in recs {
        var fs: List[Str] = []
        for f in rec {
          fs.push(f.bytes().to_hex())
        }
        out = out + "[" + fs.join(",") + "]"
      }
      out
    }
    Err(e) => "err " + e.display()
  }
}

fn cf_format(text: Str, sep: Rune, comment: Rune, fields: Int, lazy: Bool, trim: Bool, crlf: Bool) -> Str {
  match csv.parse(text, sep: sep, comment: comment, fields: fields, lazy_quotes: lazy, trim_leading_space: trim) {
    Ok(recs) => csv.format(recs, sep: sep, crlf: crlf).bytes().to_hex()
    Err(e) => "err " + e.display()
  }
}

`

func xoRune(r rune) string {
	switch r {
	case 0:
		return `'\0'`
	case '\t':
		return `'\t'`
	}
	return "'" + string(r) + "'"
}

// cfXoFile is the generated test file: the renderer and one test per case.
func cfXoFile(seed int64, cases []cfCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/csvfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// encoding/csv; do not edit: XO_CSVFUZZ_WRITE=1 go test ./internal/stdtest -run TestCsvFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(cfRenderXo)
	for i, c := range cases {
		args := fmt.Sprintf("%s, %s, %s, %d, %t, %t", xoStr(c.text), xoRune(c.sep), xoRune(c.comment), c.fields, c.lazy, c.trim)
		fmt.Fprintf(&b, "test \"csv fuzz %d\" {\n  let got = cf_read(%s)\n  if got != %s {\n    fault(\"parse: got \" + got)\n  }\n", i, args, xoStr(c.read))
		if c.format != "" {
			fmt.Fprintf(&b, "  let f = cf_format(%s, %t)\n  if f != %s {\n    fault(\"format: got \" + f)\n  }\n", args, c.crlf, xoStr(c.format))
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// cfCoverage fails when the cases stop reaching the reader's parts.
func cfCoverage(t *testing.T, cases []cfCase) {
	t.Helper()
	oks, errs, formats, quoted := 0, 0, 0, 0
	msgs := map[string]bool{}
	for _, c := range cases {
		if strings.HasPrefix(c.read, "err ") {
			errs++
			msgs[c.read[strings.LastIndex(c.read, ":")+1:]] = true
			continue
		}
		oks++
		if c.format != "" {
			formats++
		}
		if strings.Contains(c.text, `"`) {
			quoted++
		}
	}
	t.Logf("answers: %d ok, %d err (%d distinct messages); format cases %d; ok with a quote %d", oks, errs, len(msgs), formats, quoted)
	if oks < len(cases)/4 || errs < len(cases)/10 || len(msgs) < 3 || formats < len(cases)/5 || quoted < 200 {
		t.Fatalf("the cases stopped covering the reader: %d ok, %d err, %d messages, %d format, %d quoted", oks, errs, len(msgs), formats, quoted)
	}
}

const cfCorpus = "testdata/corpus/csvfuzz/csvfuzz_test.xo"

// TestCsvFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestCsvFuzzCorpusCurrent(t *testing.T) {
	cases := cfCases(rand.New(rand.NewSource(1)), 2000)
	cfCoverage(t, cases)
	text := cfXoFile(1, cases)
	if os.Getenv("XO_CSVFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(cfCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(cfCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's encoding/csv or the generator changed): XO_CSVFUZZ_WRITE=1 go test ./internal/stdtest -run TestCsvFuzzCorpusCurrent", cfCorpus)
	}
}

// TestCsvFuzz runs a fresh seed's cases.
func TestCsvFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the csv fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_CSVFUZZ_SEED", 2))
	n := envInt(t, "XO_CSVFUZZ_N", 2000)
	cases := cfCases(rand.New(rand.NewSource(seed)), n)
	for i, c := range cases {
		t.Logf("case %d %q", i, c.text)
	}
	runFuzzDir(t, "csvfuzz_test.xo", cfXoFile(seed, cases), envBackends("XO_CSVFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
