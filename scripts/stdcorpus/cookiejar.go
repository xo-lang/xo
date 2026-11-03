package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
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
	o.WriteString(cookiejarTests)
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
