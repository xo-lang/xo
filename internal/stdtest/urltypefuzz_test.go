// A fuzz harness for std/url's URL type against Go's net/url (decision
// 0642; decision 0461: a parser of untrusted input is fuzzed; decision
// 0616's fuzz kind). urlfuzz_test.go covers std/url's escaping and
// Values; this covers url.parse and what a Url answers.
//
// Each case is a URL built from httpurlfuzz_test.go's pieces (a scheme or
// none, an authority with userinfo, IPv4 and IPv6 hosts with ports and
// zones, escapes valid and not, paths with dot segments, queries and
// fragments) or from its alphabet at random, sometimes changed the way a
// hostile peer would (a byte replaced, deleted or inserted, or the text
// cut); a case that is not UTF-8 is drawn again, since a Str holds UTF-8
// (std/url's rule for decoded bytes is applied to Go's answers):
//
//   - url.parse and url.parse_request_uri give Go's error text, or a Url
//     whose display, hostname, port, request_uri, redacted,
//     escaped_path, escaped_fragment and query().encode() are Go's;
//   - the URL as a base, a second case resolved against it with
//     parse_ref is Go's URL.Parse of it;
//   - url.join_path of the case and up to three elements is Go's
//     JoinPath.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/urltypefuzz/urltypefuzz_test.xo, which TestStdCorpora
// runs on every corpus backend; TestUrlTypeFuzzCorpusCurrent checks it is
// what this generator writes (XO_URLTFUZZ_WRITE=1 rewrites it).
// TestUrlTypeFuzz runs a fresh seed (XO_URLTFUZZ_SEED, default 2;
// XO_URLTFUZZ_N cases, default 2000) on XO_URLTFUZZ_BACKENDS (default
// arm64 and llvm).
package stdtest

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// utCase is one URL and Go's answers about it.
type utCase struct {
	raw             string
	parse, parseReq string
	ref, resolved   string
	elems           []string
	joined          string
}

// utElems are join_path's elements: names, dot segments, slashes,
// escapes valid and not.
var utElems = []string{"a", "b/", "..", ".", "../..", "/x", "", "c%20d", "%zz", "é", "x/../y", "//", "?q"}

// utRaw draws one UTF-8 case: structured, random, or absolute, then
// perhaps changed.
func utRaw(r *rand.Rand) string {
	for {
		var s string
		switch r.Intn(3) {
		case 0:
			s = ufStructured(r)
		case 1:
			s = ufRandom(r)
		default:
			s = ufAbsolute(r)
		}
		if r.Intn(4) == 0 && s != "" {
			bs := []byte(s)
			i := r.Intn(len(bs))
			switch r.Intn(4) {
			case 0:
				bs[i] = uqMutBytes[r.Intn(len(uqMutBytes))]
			case 1:
				bs = append(bs[:i], bs[i+1:]...)
			case 2:
				bs = append(bs[:i], append([]byte{uqMutBytes[r.Intn(len(uqMutBytes))]}, bs[i:]...)...)
			default:
				bs = bs[:i]
			}
			s = string(bs)
		}
		if utf8.ValidString(s) {
			return s
		}
	}
}

// utRuled is u under std/url's rule for decoded bytes that are not
// UTF-8 (spec std 20.1): the host and the user's name and password hold
// each such byte as its %XX escape, so their escaped forms escape that
// `%` again. The path and fragment are exact (std/url keeps their
// escaped text), so they stay as Go has them.
func utRuled(u *url.URL) *url.URL {
	r := *u
	r.Host = uqTextOf(u.Host)
	if u.User != nil {
		if p, ok := u.User.Password(); ok {
			r.User = url.UserPassword(uqTextOf(u.User.Username()), uqTextOf(p))
		} else {
			r.User = url.User(uqTextOf(u.User.Username()))
		}
	}
	return &r
}

// utQuery is Query().Encode() with each key and value under the rule, as
// std/url's query decodes them.
func utQuery(u *url.URL) string {
	v := url.Values{}
	for k, xs := range u.Query() {
		for _, x := range xs {
			v[uqTextOf(k)] = append(v[uqTextOf(k)], uqTextOf(x))
		}
	}
	return v.Encode()
}

// utAnswer is a parse's answer as std/url prints it: `err TEXT`, or `ok`
// and the Url's derived texts.
func utAnswer(u *url.URL, err error) string {
	if err != nil {
		return "err " + uqTextOf(err.Error())
	}
	r := utRuled(u)
	return "ok " + strings.Join([]string{
		r.String(), r.Hostname(), r.Port(), u.RequestURI(),
		r.Redacted(), u.EscapedPath(), u.EscapedFragment(), utQuery(u),
	}, " | ")
}

func utCases(r *rand.Rand, n int) []utCase {
	var cs []utCase
	for len(cs) < n {
		var c utCase
		c.raw = utRaw(r)
		u, err := url.Parse(c.raw)
		c.parse = utAnswer(u, err)
		c.parseReq = utAnswer(url.ParseRequestURI(c.raw))
		c.ref = utRaw(r)
		if err == nil {
			ru, rerr := u.Parse(c.ref)
			if rerr != nil {
				c.resolved = "err " + uqTextOf(rerr.Error())
			} else {
				c.resolved = "ok " + utRuled(ru).String()
			}
		}
		for i, k := 0, r.Intn(4); i < k; i++ {
			c.elems = append(c.elems, utElems[r.Intn(len(utElems))])
		}
		if _, jerr := url.JoinPath(c.raw, c.elems...); jerr != nil {
			c.joined = "err " + uqTextOf(jerr.Error())
		} else {
			c.joined = "ok " + utRuled(u.JoinPath(c.elems...)).String()
		}
		// Go's answers are bytes; a case whose answer std/url's rule
		// cannot make UTF-8 never occurs (uqTextOf escapes every invalid
		// byte), so every case is kept
		cs = append(cs, c)
	}
	return cs
}

const utCheckXo = `use std/url

fn answer(r: Result[url.Url, url.UrlErr]) -> Str {
  match r {
    Ok(u) => "ok " + [u.display(), u.hostname(), u.port(), u.request_uri(), u.redacted(), u.escaped_path(), u.escaped_fragment(), u.query().encode()].join(" | ")
    Err(e) => "err ${e.display()}"
  }
}

test "std/url parse and parse_request_uri answer as Go's net/url" {
  var bad: List[Str] = []
  for (s, p, q) in PARSES {
    let gp = answer(url.parse(s))
    let gq = answer(url.parse_request_uri(s))
    if gp != p || gq != q {
      bad.push("${s.quote()}: parse ${gp.quote()} want ${p.quote()}; request ${gq.quote()} want ${q.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${PARSES.len()} parses differ, first: ${bad[0]}")
  }
}

test "std/url parse_ref resolves as Go's URL.Parse" {
  var bad: List[Str] = []
  for (b, r, want) in RESOLVES {
    let got = match url.parse(b) {
      Ok(u) => match u.parse_ref(r) {
        Ok(x) => "ok ${x.display()}"
        Err(e) => "err ${e.display()}"
      }
      Err(e) => "base err ${e.display()}"
    }
    if got != want {
      bad.push("${b.quote()} + ${r.quote()}: ${got.quote()} want ${want.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${RESOLVES.len()} resolves differ, first: ${bad[0]}")
  }
}

test "std/url join_path answers as Go's JoinPath" {
  var bad: List[Str] = []
  for (b, elems, want) in JOINS {
    let got = match url.join_path(b, elems) {
      Ok(s) => "ok ${s}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push("${b.quote()} ${elems.join(",")}: ${got.quote()} want ${want.quote()}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${JOINS.len()} joins differ, first: ${bad[0]}")
  }
}
`

func utXoFile(seed int64, cases []utCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/urltypefuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// net/url; do not edit: XO_URLTFUZZ_WRITE=1 go test ./internal/stdtest -run TestUrlTypeFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(utCheckXo)
	b.WriteString("\n/// (raw, Parse's answer, ParseRequestURI's answer)\nconst PARSES: List[(Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %s, %s),\n", xoStr(c.raw), xoStr(c.parse), xoStr(c.parseReq))
	}
	b.WriteString("]\n\n/// (base, ref, URL.Parse of ref against base)\nconst RESOLVES: List[(Str, Str, Str)] = [\n")
	for _, c := range cases {
		if c.resolved != "" {
			fmt.Fprintf(&b, "  (%s, %s, %s),\n", xoStr(c.raw), xoStr(c.ref), xoStr(c.resolved))
		}
	}
	b.WriteString("]\n\n/// (base, elements, JoinPath's answer)\nconst JOINS: List[(Str, List[Str], Str)] = [\n")
	for _, c := range cases {
		var es []string
		for _, e := range c.elems {
			es = append(es, xoStr(e))
		}
		fmt.Fprintf(&b, "  (%s, [%s], %s),\n", xoStr(c.raw), strings.Join(es, ", "), xoStr(c.joined))
	}
	b.WriteString("]\n")
	return b.String()
}

// utCoverage fails when the cases stop reaching the outcomes that matter.
func utCoverage(t *testing.T, cases []utCase) {
	t.Helper()
	ok, bad, req, host, port, user, frag, resolved, joinErr := 0, 0, 0, 0, 0, 0, 0, 0, 0
	for _, c := range cases {
		if strings.HasPrefix(c.parse, "ok ") {
			ok++
			f := strings.Split(c.parse[3:], " | ")
			if f[1] != "" {
				host++
			}
			if f[2] != "" {
				port++
			}
			if strings.Contains(f[0], "@") {
				user++
			}
			if strings.Contains(f[0], "#") {
				frag++
			}
		} else {
			bad++
		}
		if strings.HasPrefix(c.parseReq, "ok ") {
			req++
		}
		if strings.HasPrefix(c.resolved, "ok ") {
			resolved++
		}
		if strings.HasPrefix(c.joined, "err ") {
			joinErr++
		}
	}
	n := len(cases)
	if ok < n/3 || bad < n/10 || req < n/10 || host < n/5 || port < n/50 || user < n/50 || frag < n/20 || resolved < n/5 || joinErr < n/20 {
		t.Fatalf("coverage fell: parse %d ok, %d errors; request %d ok; %d hosts, %d ports, %d userinfo, %d fragments; %d resolved; %d join errors; of %d", ok, bad, req, host, port, user, frag, resolved, joinErr, n)
	}
}

const utCorpus = "testdata/corpus/urltypefuzz/urltypefuzz_test.xo"

// TestUrlTypeFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestUrlTypeFuzzCorpusCurrent(t *testing.T) {
	cases := utCases(rand.New(rand.NewSource(1)), 2000)
	utCoverage(t, cases)
	text := utXoFile(1, cases)
	if os.Getenv("XO_URLTFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(utCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(utCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(utCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's net/url or the generator changed): XO_URLTFUZZ_WRITE=1 go test ./internal/stdtest -run TestUrlTypeFuzzCorpusCurrent", utCorpus)
	}
}

// TestUrlTypeFuzz runs a fresh seed's cases.
func TestUrlTypeFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the url type fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_URLTFUZZ_SEED", 2))
	n := envInt(t, "XO_URLTFUZZ_N", 2000)
	cases := utCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "urltypefuzz_test.xo", utXoFile(seed, cases), envBackends("XO_URLTFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
