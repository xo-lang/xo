package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"unicode/utf8"
)

func urlText(r *rand.Rand) string {
	alpha := []string{"a", "Z", "0", " ", "+", "%", "%2", "%20", "%2B", "%zz", "%C3%A9", "%c3%a9", "%FF", "%", "é", "&", "=", ";", "/", "?", "#", "~", "-", "_", ".", "!", "*", "'", "(", ")", ":", "@", "$", ",", "[", "]", "\x00", "\t", "\n", "\x7f", "\u0080", "�", "日", "%00"}
	var b strings.Builder
	for i, n := 0, r.Intn(12); i < n; i++ {
		b.WriteString(alpha[r.Intn(len(alpha))])
	}
	return textOf(b.String())
}

// urlKeep is std/url's text of outside bytes: each invalid byte as its
// %XX escape (std/rt/url.go urlText, decision 0259).
func urlKeep(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	const hx = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteByte('%')
			b.WriteByte(hx[s[i]>>4])
			b.WriteByte(hx[s[i]&15])
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

func urlUnesc(s string, err error) string {
	if err != nil {
		return "err " + textOf(err.Error())
	}
	return "ok " + urlKeep(s)
}

// urlQuery is std/rt's parse_query: keys in first appearance order with
// their values, and Values.Encode; or err TEXT.
func urlQuery(s string) string {
	if _, err := url.ParseQuery(s); err != nil {
		return "err " + textOf(err.Error())
	}
	var ks []string
	vs := map[string][]string{}
	for _, pair := range strings.Split(s, "&") {
		if pair == "" {
			continue
		}
		k, x, _ := strings.Cut(pair, "=")
		k, _ = url.QueryUnescape(k)
		x, _ = url.QueryUnescape(x)
		k, x = urlKeep(k), urlKeep(x)
		if _, ok := vs[k]; !ok {
			ks = append(ks, k)
		}
		vs[k] = append(vs[k], x)
	}
	var parts []string
	m := url.Values{}
	for _, k := range ks {
		parts = append(parts, k+"="+strings.Join(vs[k], ","))
		m[k] = vs[k]
	}
	return "ok " + strings.Join(parts, "|") + " " + m.Encode()
}

func urlCorpus() string {
	r := rand.New(rand.NewSource(11))
	var o strings.Builder
	o.WriteString(header("net/url's QueryEscape, PathEscape, QueryUnescape, PathUnescape, ParseQuery, and Values.Encode as std/rt/url.go calls them, on seeded random texts of escapes, reserved bytes, and bad percent codes"))
	o.WriteString("use std/url\n\n/// (text, Go's QueryEscape, PathEscape, QueryUnescape, PathUnescape: ok TEXT or err TEXT)\nconst ESCAPES: List[(Str, Str, Str, Str, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		s := urlText(r)
		qu, qe := url.QueryUnescape(s)
		pu, pe := url.PathUnescape(s)
		fmt.Fprintf(&o, "  (%s, %s, %s, %s, %s),\n", xq(s), xq(url.QueryEscape(s)), xq(url.PathEscape(s)), xq(urlUnesc(qu, qe)), xq(urlUnesc(pu, pe)))
	}
	o.WriteString("]\n\n/// (query, Go's result: ok KEY=V,V|... ENCODED or err TEXT)\nconst QUERIES: List[(Str, Str)] = [\n")
	for i := 0; i < 3000; i++ {
		var b strings.Builder
		for j, n := 0, r.Intn(5); j < n; j++ {
			if j > 0 {
				b.WriteString([]string{"&", "&", "&", "&&", ";", ";;", "&&&"}[r.Intn(7)])
			}
			b.WriteString([]string{"a", "b", "key", "a+b", "%61", "x%zz", "", "é", "k%20", "%2F", "%", "%c3%a9", "日本", "�", "\x7f", "\u0080"}[r.Intn(16)])
			if r.Intn(5) > 0 {
				b.WriteString("=" + urlText(r))
			}
			if r.Intn(12) == 0 {
				b.WriteString("=" + urlText(r))
			}
		}
		s := textOf(b.String())
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(urlQuery(s)))
	}
	o.WriteString("]\n")
	o.WriteString(urlTests)
	return o.String()
}

const urlTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn unesc(r: Result[Str, url.EscapeErr]) -> Str {
  match r {
    Ok(s) => "ok ${s}"
    Err(e) => "err ${e.display()}"
  }
}

test "the escapes and unescapes are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (s, qe, pe, qu, pu) in ESCAPES {
    let got = "${url.query_escape(s)} ${url.path_escape(s)} ${unesc(url.query_unescape(s))} ${unesc(url.path_unescape(s))}"
    let want = "${qe} ${pe} ${qu} ${pu}"
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse_query and encode are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in QUERIES {
    let got = match url.parse_query(s) {
      Ok(v) => "ok ${v.keys().map(fn(k) { "${k}=${v.all(k).join(",")}" }).join("|")} ${v.encode()}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
