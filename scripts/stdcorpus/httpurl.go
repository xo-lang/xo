package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// goURLInputs are the input strings of Go's net/url tests (url_test.go:
// urltests, parseRequestURLTests, stringURLTests and TestParseErrors),
// read from the Go tree the corpus is generated with.
func goURLInputs() []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(runtime.GOROOT(), "src", "net", "url", "url_test.go"), nil, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		os.Exit(1)
	}
	want := map[string]bool{"urltests": true, "parseRequestURLTests": true, "stringURLTests": true, "TestParseErrors": true}
	seen := map[string]bool{}
	var out []string
	for _, d := range f.Decls {
		name := ""
		switch d := d.(type) {
		case *ast.GenDecl:
			for _, s := range d.Specs {
				if v, ok := s.(*ast.ValueSpec); ok && len(v.Names) == 1 {
					name = v.Names[0].Name
				}
			}
		case *ast.FuncDecl:
			name = d.Name.Name
		}
		if !want[name] {
			continue
		}
		ast.Inspect(d, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok || len(cl.Elts) == 0 {
				return true
			}
			first := cl.Elts[0]
			if kv, ok := first.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); !ok || (id.Name != "in" && id.Name != "url") {
					return true
				}
				first = kv.Value
			}
			if bl, ok := first.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				s, err := strconv.Unquote(bl.Value)
				if err == nil && !seen[s] {
					seen[s] = true
					out = append(out, s)
				}
			}
			return true
		})
	}
	return out
}

func hurlRandom(r *rand.Rand) string {
	pick := func(xs ...string) string { return xs[r.Intn(len(xs))] }
	var b strings.Builder
	switch r.Intn(8) {
	case 0:
		b.WriteString(pick("http", "https", "HTTP", "Https", "ftp", "mailto", "h t", "1http", "ht+tp", "x.y-z"))
		b.WriteString(pick(":", ":", ":"))
		b.WriteString(pick("//", "//", "//", "/", ""))
	case 1:
		b.WriteString("//")
	default:
		b.WriteString(pick("http", "https", "http", "https", "HTTP"))
		b.WriteString("://")
	}
	if r.Intn(5) == 0 {
		b.WriteString(pick("u", "user:pw", "u%40x", "a b", ":"))
		b.WriteString("@")
	}
	b.WriteString(pick("example.com", "a.b", "localhost", "EXAMPLE.com", "[::1]", "[fe80::1%25en0]", "127.0.0.1", "h%C3%A9.com", "hé.com", "a_b", "", "[::1", "x y", "%41", "日本.jp"))
	if r.Intn(3) == 0 {
		b.WriteString(pick(":80", ":8080", ":", ":0080", ":99999999999999999999", ":http", ":65535", ":%38"))
	}
	for i, n := 0, r.Intn(4); i < n; i++ {
		b.WriteString("/" + pick("a", "b", "a%20b", "%2F", "%zz", "é", "%C3%A9", "..", ".", "a+b", ";x", ":", "@", "%", "%2", "日", "%FF", "a b", ""))
	}
	if r.Intn(2) == 0 {
		b.WriteString("?" + hurlQuery(r))
	}
	if r.Intn(4) == 0 {
		b.WriteString("#" + pick("f", "", "a b", "%zz", "é", "#"))
	}
	if r.Intn(30) == 0 {
		b.WriteString(pick("\x00", "\t", "\x7f", "\n"))
	}
	return textOf(b.String())
}

func hurlQuery(r *rand.Rand) string {
	var b strings.Builder
	for j, n := 0, r.Intn(6); j < n; j++ {
		if j > 0 {
			b.WriteString([]string{"&", "&", "&", "&", "&&", ";", "&;&"}[r.Intn(7)])
		}
		b.WriteString([]string{"a", "b", "key", "a+b", "%61", "x%zz", "", "é", "k%20", "a", "b", "日", "%", "k;"}[r.Intn(14)])
		if r.Intn(5) > 0 {
			b.WriteString("=" + urlText(r))
		}
	}
	return b.String()
}

// hurlGoQuery is Go's first value of each key of raw, as k=v,k=v with
// the keys sorted (pairs with a bad escape or a semicolon are dropped).
func hurlGoQuery(raw string) string {
	vals, _ := url.ParseQuery(raw)
	var ks []string
	for k := range vals {
		ks = append(ks, urlKeep(k))
	}
	sort.Strings(ks)
	m := map[string]string{}
	for k, v := range vals {
		m[urlKeep(k)] = urlKeep(v[0])
	}
	var parts []string
	for _, k := range ks {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}

func hurlPort(u *url.URL) string {
	p := u.Port()
	if p == "" {
		return ""
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return ""
	}
	return strconv.Itoa(n)
}

// hurlParse is Go's side of http.parse_url: ok SCHEME|HOST|PORT|PATH|QUERY
// or err TEXT.
func hurlParse(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "err " + textOf(err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "err scheme must be http or https: " + s
	}
	if u.Host == "" {
		return "err missing host: " + s
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	return "ok " + u.Scheme + "|" + urlKeep(u.Hostname()) + "|" + hurlPort(u) + "|" + urlKeep(p) + "|" + hurlGoQuery(u.RawQuery)
}

// hurlRequest is Go's side of http.new_request(GET, s): path, the url's
// path and query (the text as the path when it does not parse).
func hurlRequest(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "path=" + s + " upath=" + s + " query="
	}
	p := u.Path
	up := p
	if up == "" {
		up = "/"
	}
	return "path=" + urlKeep(p) + " upath=" + urlKeep(up) + " query=" + hurlGoQuery(u.RawQuery)
}

// hurlTargetQuery is the query Go reads from the request target /p?q: a
// '#' ends it, and a target that does not parse has none.
func hurlTargetQuery(q string) string {
	u, err := url.Parse("/p?" + q)
	if err != nil {
		return ""
	}
	return hurlGoQuery(u.RawQuery)
}

func httpurlCorpus() string {
	r := rand.New(rand.NewSource(23))
	var o strings.Builder
	o.WriteString(header("net/url's Parse on the input strings of Go's url_test.go (urltests, parseRequestURLTests, stringURLTests, TestParseErrors) and on seeded random URLs, with ParseQuery's first value of each key, against std/http's parse_url and new_request (Go's net/url tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("use std/http\n\n")
	in := goURLInputs()
	for i := 0; i < 4000; i++ {
		in = append(in, hurlRandom(r))
	}
	o.WriteString("/// (url, Go's http.parse_url result: ok SCHEME|HOST|PORT|PATH|K=V,K=V or err TEXT, and new_request's path=PATH upath=PATH query=K=V,K=V)\nconst URLS: List[(Str, Str, Str)] = [\n")
	for _, s := range in {
		s = textOf(s)
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(s), xq(hurlParse(s)), xq(hurlRequest(s)))
	}
	o.WriteString("]\n\n/// (query, Go's first value of each key: K=V,K=V)\nconst QUERIES: List[(Str, Str)] = [\n")
	for i := 0; i < 3000; i++ {
		q := textOf(hurlQuery(r))
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(q), xq(hurlTargetQuery(q)))
	}
	o.WriteString("]\n")
	o.WriteString(httpurlTests)
	return o.String()
}

const httpurlTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn show(q: Map[Str, Str]) -> Str { q.keys().sort().map(fn(k) { "${k}=${q.get(k) ?? ""}" }).join(",") }

test "parse_url is Go's url.Parse for an http URL" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want, _) in URLS {
    let got = match http.parse_url(s) {
      Ok(u) => "ok ${u.scheme}|${u.host}|${u.port.map(fn(p) { "${p}" }) ?? ""}|${u.path}|${show(u.query)}"
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "new_request reads the path and the first value of each query key as Go does" {
  var bad: List[(Str, Str, Str)] = []
  for (s, _, want) in URLS {
    let r = http.new_request("GET", s)
    let got = "path=${r.path} upath=${r.url.path} query=${show(r.url.query)}"
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "a request's query is Go's ParseQuery, first value of each key" {
  var bad: List[(Str, Str, Str)] = []
  for (q, want) in QUERIES {
    let r = http.new_request("GET", "/p?" + q)
    let got = show(r.url.query)
    if got != want {
      bad.push((q, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
