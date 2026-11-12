package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The std/cookiejar corpus: seeded sequences of SetCookies and Cookies calls
// on real net/http/cookiejar (go1.26.1) and on std/cookiejar, the same
// cookies per call and the cookies to send compared name by name in order.
// Go's jar reads the clock itself, so the corpus keeps to what the clock
// cannot change: Max-Age 0 and below, Expires in 2000 (deleted) or 2100,
// Max-Age of an hour in scripts of under 3000 virtual seconds. The time
// passing cases are written by hand in the tests.
func cookiejarCorpus() string {
	rg := rand.New(rand.NewSource(58))
	var o strings.Builder
	o.WriteString(header("net/http/cookiejar (go1.26.1) on seeded SetCookies and Cookies sequences against std/cookiejar"))
	o.WriteString("use std/cookiejar\nuse std/http\n\n")
	hosts := []string{"www.example.com", "example.com", "a.b.example.com", "b.example.com", "sub.example.org", "example.org.", "WWW.Example.COM", "127.0.0.1", "[::1]", "localhost", "foo.co.uk", "xn--bcher-kva.example"}
	paths := []string{"", "/", "/a", "/a/", "/a/b", "/a/b/c", "/ab", "a/b", "/a/b/", "/x/y"}
	domains := []string{"", "", "", ".example.com", "example.com", "b.example.com", "other.com", "com", ".", "..x", "Example.COM", "www.example.com.", "a.b.example.com", "example.org", "127.0.0.1", "uk", "co.uk"}
	cpaths := []string{"", "", "/", "/a", "/a/b", "x", "/ab", "/a/"}
	names := []string{"n1", "n2", "n3", "n4", "id"}
	type step struct {
		set    bool
		scheme string
		host   string
		path   string
		cs     string
		want   string
	}
	var scripts []string
	o.WriteString("/// (kind 0 set 1 get, scheme, host, path, cookies `name|value|domain|path|secure|maxage|expires`, joined by `;;`, the cookies to send as `name=value,...`)\nconst STEPS: List[List[(Int, Str, Str, Str, Str, Str)]] = [\n")
	for sc := 0; sc < 100; sc++ {
		j, _ := cookiejar.New(nil)
		o.WriteString("  [\n")
		n := 8 + rg.Intn(40)
		for i := 0; i < n; i++ {
			scheme := "http"
			if rg.Intn(2) == 0 {
				scheme = "https"
			}
			if rg.Intn(40) == 0 {
				scheme = "ftp"
			}
			host := hosts[rg.Intn(len(hosts))]
			path := paths[rg.Intn(len(paths))]
			hostURL := host
			xoHost := hostURL
			u := &url.URL{Scheme: scheme, Host: hostURL, Path: path}
			if rg.Intn(4) == 0 {
				u.Host = hostURL + ":8080"
				xoHost = strings.Trim(hostURL, "[]") + "#8080"
			}
			if rg.Intn(3) != 0 {
				var cs []*http.Cookie
				var enc []string
				for k := rg.Intn(3) + 1; k > 0; k-- {
					c := &http.Cookie{Name: names[rg.Intn(len(names))], Value: fmt.Sprintf("v%d", rg.Intn(100))}
					c.Domain = domains[rg.Intn(len(domains))]
					c.Path = cpaths[rg.Intn(len(cpaths))]
					c.Secure = rg.Intn(4) == 0
					exp := "-"
					switch rg.Intn(8) {
					case 0:
						c.MaxAge = -1
					case 1:
						c.MaxAge = 3600
					case 2:
						c.Expires = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
						exp = "past"
					case 3:
						c.Expires = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
						exp = "future"
					}
					cs = append(cs, c)
					enc = append(enc, fmt.Sprintf("%s|%s|%s|%s|%v|%d|%s", c.Name, c.Value, c.Domain, c.Path, c.Secure, c.MaxAge, exp))
				}
				j.SetCookies(u, cs)
				fmt.Fprintf(&o, "    (0, %q, %q, %q, %q, \"\"),\n", scheme, xoHost, path, strings.Join(enc, ";;"))
			} else {
				var got []string
				for _, c := range j.Cookies(u) {
					got = append(got, c.Name+"="+c.Value)
				}
				fmt.Fprintf(&o, "    (1, %q, %q, %q, \"\", %q),\n", scheme, xoHost, path, strings.Join(got, ","))
			}
		}
		o.WriteString("  ],\n")
	}
	_ = scripts
	o.WriteString("]\n\n")
	o.WriteString(cookiejarPslSteps(rand.New(rand.NewSource(617))))
	o.WriteString(cookiejarTests)
	return o.String()
}

// cookiejarPslSteps: seeded scripts on hosts and Domain attributes around
// public suffixes (wildcards, exceptions, private suffixes), replayed by
// psloracle's jar mode on net/http/cookiejar with x/net's
// publicsuffix.List (decision 0617 B), as STEPS_PSL for cookiejar.new_with.
func cookiejarPslSteps(rg *rand.Rand) string {
	hosts := []string{"example.co.uk", "www.example.co.uk", "foo.co.uk", "co.uk", "uk", "user.github.io", "a.user.github.io", "github.io", "example.com", "www.example.com", "com", "city.kawasaki.jp", "www.city.kawasaki.jp", "a.foo.kawasaki.jp", "foo.kawasaki.jp", "kawasaki.jp", "x.s3.amazonaws.com", "s3.amazonaws.com", "a.nosuchtld", "b.a.nosuchtld", "127.0.0.1"}
	domains := []string{"", "", "co.uk", ".co.uk", "example.co.uk", "uk", "github.io", "user.github.io", ".github.io", "com", "example.com", "kawasaki.jp", "city.kawasaki.jp", "foo.kawasaki.jp", "a.foo.kawasaki.jp", "jp", "s3.amazonaws.com", "amazonaws.com", "nosuchtld", "a.nosuchtld"}
	paths := []string{"/", "/a", "/a/b"}
	names := []string{"n1", "n2", "id"}
	type step struct {
		set                bool
		scheme, host, path string
		enc                string
	}
	var scripts [][]step
	var in strings.Builder
	for sc := 0; sc < 80; sc++ {
		in.WriteString("N\n")
		var st []step
		for i, n := 0, 6+rg.Intn(24); i < n; i++ {
			x := step{scheme: []string{"http", "https"}[rg.Intn(2)], host: hosts[rg.Intn(len(hosts))], path: paths[rg.Intn(len(paths))]}
			if rg.Intn(3) != 0 {
				x.set = true
				var enc []string
				for k := rg.Intn(3) + 1; k > 0; k-- {
					enc = append(enc, fmt.Sprintf("%s|v%d|%s|/|false|0|-", names[rg.Intn(len(names))], rg.Intn(100), domains[rg.Intn(len(domains))]))
				}
				x.enc = strings.Join(enc, ";;")
				fmt.Fprintf(&in, "S\t%s\t%s\t%s\t%s\n", x.scheme, x.host, x.path, x.enc)
			} else {
				fmt.Fprintf(&in, "G\t%s\t%s\t%s\n", x.scheme, x.host, x.path)
			}
			st = append(st, x)
		}
		scripts = append(scripts, st)
	}
	cmd := exec.Command("go", "run", ".", "jar")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "psloracle")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	cmd.Stdin = strings.NewReader(in.String())
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("psloracle jar: " + err.Error())
	}
	answers := strings.Split(string(out), "\n")
	var o strings.Builder
	o.WriteString("/// The same as STEPS for cookiejar.new_with(): net/http/cookiejar with\n/// golang.org/x/net/publicsuffix.List (psloracle jar)\nconst STEPS_PSL: List[List[(Int, Str, Str, Str, Str, Str)]] = [\n")
	ai := 0
	for _, st := range scripts {
		o.WriteString("  [\n")
		for _, x := range st {
			if x.set {
				fmt.Fprintf(&o, "    (0, %q, %q, %q, %q, \"\"),\n", x.scheme, x.host, x.path, x.enc)
			} else {
				fmt.Fprintf(&o, "    (1, %q, %q, %q, \"\", %q),\n", x.scheme, x.host, x.path, answers[ai])
				ai++
			}
		}
		o.WriteString("  ],\n")
	}
	o.WriteString("]\n\n")
	return o.String()
}

const cookiejarTests = `fn now(i: Int) -> Time { Time.from_unix(1700000000 + 10 * i) }

/// host, or host#port for a URL with a port (the Url holds them apart)
fn url_of(scheme: Str, host: Str, path: Str) -> http.Url {
  match host.find("#") {
    Some(i) => http.Url{scheme: scheme, host: host[0..i], port: Int.parse(host[i + 1..host.len()]), path: path, query: {}}
    None => http.Url{scheme: scheme, host: host, port: None, path: path, query: {}}
  }
}

fn cookie_of(enc: Str) -> http.Cookie {
  let f = enc.split("|")
  var c = http.Cookie{name: f[0], value: f[1], domain: f[2], path: f[3], secure: f[4] == "true", max_age: Int.parse(f[5]) ?? 0}
  if f[6] == "past" {
    c.expires = Some(Time.from_unix(946684800))
  } else if f[6] == "future" {
    c.expires = Some(Time.from_unix(4102444800))
  }
  c
}

fn show(cs: List[http.Cookie]) -> Str {
  var parts: List[Str] = []
  for c in cs {
    parts.push("${c.name}=${c.value}")
  }
  parts.join(",")
}

test "net/http/cookiejar's SetCookies and Cookies on seeded sequences" {
  var bad: List[Str] = []
  var si = 0
  for script in STEPS {
    let j = cookiejar.new()
    var i = 0
    for s in script {
      let u = url_of(s.1, s.2, s.3)
      if s.0 == 0 {
        var cs: List[http.Cookie] = []
        for e in s.4.split(";;") {
          cs.push(cookie_of(e))
        }
        j.set_cookies(u, cs, now(i))
      } else {
        let got = show(j.cookies(u, now(i)))
        if got != s.5 {
          bad.push("script ${si} step ${i} ${s.1}://${s.2}${s.3}: ${got} want ${s.5}")
        }
      }
      i += 1
    }
    si += 1
  }
  expect(bad[0..(if bad.len() < 8 { bad.len() } else { 8 })]) == []
}

fn replay(steps: List[List[(Int, Str, Str, Str, Str, Str)]], mk: fn() -> cookiejar.Jar) -> List[Str] {
  var bad: List[Str] = []
  var si = 0
  for script in steps {
    let j = mk()
    var i = 0
    for s in script {
      let u = url_of(s.1, s.2, s.3)
      if s.0 == 0 {
        var cs: List[http.Cookie] = []
        for e in s.4.split(";;") {
          cs.push(cookie_of(e))
        }
        j.set_cookies(u, cs, now(i))
      } else {
        let got = show(j.cookies(u, now(i)))
        if got != s.5 {
          bad.push("script ${si} step ${i} ${s.1}://${s.2}${s.3}: ${got} want ${s.5}")
        }
      }
      i += 1
    }
    si += 1
  }
  bad[0..(if bad.len() < 8 { bad.len() } else { 8 })]
}

test "new_with() answers as net/http/cookiejar with x/net's publicsuffix.List" {
  expect(replay(STEPS_PSL, fn() { cookiejar.new_with() })) == []
}

test "new_with(): a public suffix is no cookie domain, and the key is the registrable domain" {
  let j = cookiejar.new_with()
  // co.uk from example.co.uk: refused (new() would keep it for every .co.uk site)
  j.set_cookies(url_of("http", "example.co.uk", "/"), [http.Cookie{name: "bad", value: "1", domain: "co.uk"}, http.Cookie{name: "ok", value: "2", domain: "example.co.uk"}], now(0))
  expect(show(j.cookies(url_of("http", "other.co.uk", "/"), now(1)))) == ""
  expect(show(j.cookies(url_of("http", "www.example.co.uk", "/"), now(1)))) == "ok=2"
  // the host itself naming its public suffix: a host cookie
  j.set_cookies(url_of("http", "github.io", "/"), [http.Cookie{name: "h", value: "3", domain: "github.io"}], now(2))
  expect(show(j.cookies(url_of("http", "github.io", "/"), now(3)))) == "h=3"
  expect(show(j.cookies(url_of("http", "user.github.io", "/"), now(3)))) == ""
  let k = cookiejar.new()
  k.set_cookies(url_of("http", "example.co.uk", "/"), [http.Cookie{name: "bad", value: "1", domain: "co.uk"}], now(0))
  expect(show(k.cookies(url_of("http", "other.co.uk", "/"), now(1)))) == "bad=1"
  expect(show(cookiejar.new_with(psl: false).cookies(url_of("http", "x.co.uk", "/"), now(1)))) == ""
}

test "a host cookie is for its host only, a domain cookie for its subdomains" {
  let j = cookiejar.new()
  j.set_cookies(url_of("http", "www.example.com", "/"), [http.Cookie{name: "h", value: "1"}, http.Cookie{name: "d", value: "2", domain: "example.com"}], now(0))
  expect(show(j.cookies(url_of("http", "www.example.com", "/"), now(1)))) == "h=1,d=2"
  expect(show(j.cookies(url_of("http", "a.example.com", "/"), now(1)))) == "d=2"
  expect(show(j.cookies(url_of("http", "example.com", "/"), now(1)))) == "d=2"
  expect(show(j.cookies(url_of("http", "example.org", "/"), now(1)))) == ""
}

test "a cookie for a domain that is not a parent of the host is ignored" {
  let j = cookiejar.new()
  j.set_cookies(url_of("http", "www.example.com", "/"), [http.Cookie{name: "x", value: "1", domain: "other.com"}], now(0))
  expect(show(j.cookies(url_of("http", "other.com", "/"), now(1)))) == ""
  expect(show(j.cookies(url_of("http", "www.example.com", "/"), now(1)))) == ""
}

test "Secure cookies go to https only, and the path decides, longest first" {
  let j = cookiejar.new()
  j.set_cookies(url_of("https", "example.com", "/a/b"), [http.Cookie{name: "s", value: "1", path: "/", secure: true}, http.Cookie{name: "p", value: "2", path: "/a/b"}, http.Cookie{name: "q", value: "3", path: "/"}], now(0))
  expect(show(j.cookies(url_of("https", "example.com", "/a/b/c"), now(1)))) == "p=2,s=1,q=3"
  expect(show(j.cookies(url_of("http", "example.com", "/a/b/c"), now(1)))) == "p=2,q=3"
  expect(show(j.cookies(url_of("http", "example.com", "/a"), now(1)))) == "q=3"
}

test "Max-Age and Expires pass with the time given" {
  let j = cookiejar.new()
  let u = url_of("http", "example.com", "/")
  j.set_cookies(u, [http.Cookie{name: "m", value: "1", max_age: 30}, http.Cookie{name: "e", value: "2", expires: Some(Time.from_unix(1700000100))}, http.Cookie{name: "s", value: "3"}], now(0))
  expect(show(j.cookies(u, Time.from_unix(1700000010)))) == "m=1,e=2,s=3"
  expect(show(j.cookies(u, Time.from_unix(1700000040)))) == "e=2,s=3"
  expect(show(j.cookies(u, Time.from_unix(1700000200)))) == "s=3"
  j.set_cookies(u, [http.Cookie{name: "s", value: "x", max_age: -1}], Time.from_unix(1700000201))
  expect(show(j.cookies(u, Time.from_unix(1700000202)))) == ""
}

test "clear forgets every cookie, and a copy of a jar shares its cookies" {
  let j = cookiejar.new()
  let k = j
  let u = url_of("http", "example.com", "/")
  j.set_cookies(u, [http.Cookie{name: "a", value: "1"}], now(0))
  expect(show(k.cookies(u, now(1)))) == "a=1"
  k.clear()
  expect(show(j.cookies(u, now(2)))) == ""
}
`
