package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// std/httpproxy (decision 0617 B): net/http's ProxyFromEnvironment, which
// is golang.org/x/net/http/httpproxy over the process environment, read
// once a process. Each seeded configuration runs in a child process of
// this generator with only that configuration's variables set; the child
// answers the proxy for each request URL. Proxies are http or https and
// no REQUEST_METHOD is set: Xo's differences (other schemes, CGI) are in
// std/httpproxy_test.xo.

const httpproxyChild = "XO_STDCORPUS_HTTPPROXY_CHILD"

func init() {
	if os.Getenv(httpproxyChild) == "" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		u, err := url.Parse(in.Text())
		if err != nil {
			fmt.Println("<bad url>")
			continue
		}
		p, err := http.ProxyFromEnvironment(&http.Request{URL: u})
		switch {
		case err != nil:
			fmt.Println("<error>")
		case p == nil:
			fmt.Println("<nil>")
		default:
			fmt.Println(p.String())
		}
	}
	os.Exit(0)
}

var httpproxyProxies = []string{
	"", "proxy.test:3128", "http://proxy.test", "https://sproxy.test:8443",
	"10.0.0.9:8080", "user:pw@proxy.test:3128", "http://[2001:db8::9]:3128", "localhost:3128",
}

var httpproxyNoProxy = []string{
	"example.com", ".example.org", "*.wild.test", "10.0.0.0/8", "192.168.1.5",
	"192.168.1.6:8080", "[2001:db8::1]:443", "2001:db8::/32", "fe80::1",
	"localhost", "a.test:443", "示例.com", " Example.NET ", ":1", "", "[::1]",
	"b.test:80", "172.16.0.0/12", "*", "xn--bcher-kva.example",
}

var httpproxyHosts = []string{
	"example.com", "www.example.com", "example.org", "a.example.org", "wild.test",
	"x.wild.test", "10.1.2.3", "11.0.0.1", "192.168.1.5", "192.168.1.6",
	"[2001:db8::1]", "[2001:db8:5::7]", "[fe80::1]", "[::1]", "127.0.0.1",
	"localhost", "a.test", "b.test", "www.示例.com", "xn--fsq092h.com",
	"EXAMPLE.NET", "example.net", "172.20.1.1", "bücher.example", "notexample.com",
}

var httpproxyPorts = []string{"", ":80", ":443", ":8080"}

func httpproxyCorpus() string {
	r := rand.New(rand.NewSource(617))
	var o strings.Builder
	o.WriteString(header("net/http's ProxyFromEnvironment (golang.org/x/net/http/httpproxy) on seeded HTTP_PROXY, HTTPS_PROXY and NO_PROXY settings"))
	o.WriteString("use std/http\nuse std/httpproxy\n\n")
	type cfg struct{ hp, hsp, no string }
	var cfgs []cfg
	type row struct {
		c         int
		u, answer string
	}
	var rows []row
	for k := 0; k < 48; k++ {
		c := cfg{hp: httpproxyProxies[r.Intn(len(httpproxyProxies))], hsp: httpproxyProxies[r.Intn(len(httpproxyProxies))]}
		var no []string
		for i, n := 0, r.Intn(6); i < n; i++ {
			e := httpproxyNoProxy[r.Intn(len(httpproxyNoProxy))]
			if e == "*" && r.Intn(3) != 0 {
				continue
			}
			no = append(no, e)
		}
		c.no = strings.Join(no, ",")
		var urls []string
		for i := 0; i < 40; i++ {
			s := []string{"http", "https"}[r.Intn(2)]
			urls = append(urls, s+"://"+httpproxyHosts[r.Intn(len(httpproxyHosts))]+httpproxyPorts[r.Intn(len(httpproxyPorts))]+"/p")
		}
		cmd := exec.Command(os.Args[0])
		cmd.Env = []string{httpproxyChild + "=1", "HTTP_PROXY=" + c.hp, "HTTPS_PROXY=" + c.hsp, "NO_PROXY=" + c.no}
		cmd.Stdin = strings.NewReader(strings.Join(urls, "\n") + "\n")
		out, err := cmd.Output()
		if err != nil {
			panic(fmt.Sprintf("httpproxy child: %v", err))
		}
		got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(got) != len(urls) {
			panic("httpproxy child: short output")
		}
		cfgs = append(cfgs, c)
		for i, u := range urls {
			rows = append(rows, row{len(cfgs) - 1, u, got[i]})
		}
	}
	o.WriteString("/// (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)\nconst CONFIGS: List[(Str, Str, Str)] = [\n")
	for _, c := range cfgs {
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xq(c.hp), xq(c.hsp), xq(c.no))
	}
	o.WriteString("]\n\n/// (config, request URL, Go's proxy URL or <nil>)\nconst CASES: List[(Int, Str, Str)] = [\n")
	for _, w := range rows {
		fmt.Fprintf(&o, "  (%d, %s, %s),\n", w.c, xq(w.u), xq(w.answer))
	}
	o.WriteString(`]

fn shown(u: http.Url?) -> Str {
  match u {
    Some(p) => p.display().trim_suffix("/")
    None => "<nil>"
  }
}

test "proxy_func answers as net/http's ProxyFromEnvironment" {
  var funcs: List[fn(http.Url) -> http.Url?] = []
  for (hp, hsp, no) in CONFIGS {
    funcs.push(httpproxy.Config{http_proxy: hp, https_proxy: hsp, no_proxy: no}.proxy_func())
  }
  var bad: List[Str] = []
  for (c, u, want) in CASES {
    let got = shown(funcs[c](http.parse_url(u).ok() ?? fault("url ${u}")))
    if got != want {
      bad.push("${CONFIGS[c]:?} ${u}: got ${got}, want ${want}")
    }
  }
  expect(bad.join("\n")) == ""
}
`)
	return o.String()
}
