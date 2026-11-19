// The oracle of TestHttpproxyFuzz (internal/stdtest/httpproxyfuzz_test.go,
// task #115): seeded proxy settings (HTTP_PROXY, HTTPS_PROXY, NO_PROXY,
// CGI) and request URLs, answered by golang.org/x/net/http/httpproxy
// v0.62.0's Config.ProxyFunc. Each case prints
//
//	== i
//	c HTTPPROXY<TAB>HTTPSPROXY<TAB>NOPROXY<TAB>CGI<TAB>URL   (hex fields)
//	w nil | p SCHEME|HOST|PORT|USER|PWSET PASSWORD | skip
//
// skip marks std/httpproxy's documented departures: a proxy setting that
// gives an error, a scheme other than http and https, or no host (Go's
// http:// retry of a setting with "://" that did not parse; Xo ignores
// each), and a request URL net/url cannot parse.
//
//	go run . SEED N
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"stdcorpus/httpproxyoracle/hostile"
)

var hosts = []string{
	"example.com", "sub.example.com", "a.b.example.com", "EXAMPLE.com", "example.com.",
	"notexample.com", "example.org", "foo.example.org", "localhost", "LOCALHOST",
	"127.0.0.1", "127.1.2.3", "10.1.2.3", "10.255.0.1", "192.168.1.1", "192.168.2.1",
	"8.8.8.8", "[::1]", "[2001:db8::1]", "[2001:db8:1::5]", "[fe80::1]", "[::ffff:10.1.2.3]",
	"bücher.example", "xn--bcher-kva.example", "münchen.de", "x", "1.2.3", "host-1.internal",
}

var ports = []string{"", "", "", ":80", ":443", ":8080", ":1234", ":0", ":65535"}

var paths = []string{"", "/", "/a/b", "/?q=1", "/x#frag"}

var noProxyItems = []string{
	"example.com", ".example.com", "*.example.com", "example.com:8080", ".example.com:443",
	"EXAMPLE.COM", "sub.example.com", "example.org", "*.org", ".org", "org",
	"localhost", "127.0.0.1", "10.0.0.0/8", "10.1.2.3", "10.1.2.3:8080", "192.168.1.0/24",
	"192.168.1.1/24", "192.168.0.0/16:80", "::1", "[::1]", "[::1]:80", "2001:db8::/32",
	"2001:db8::1", "[2001:db8::1]:443", "fe80::/10", "*", " example.com ", "", ":80",
	"bücher.example", "xn--bcher-kva.example", "münchen.de", "1.2.3", "host-1.internal",
	"*.", ".", "[", "]", "a:b:c", "10.0.0.0/33", "example.com:", "[::ffff:10.1.2.3]",
	"8.8.8.8/32", "0.0.0.0/0", "::/0", "%", "x",
}

var proxies = []string{
	"", "", "proxy", "proxy:3128", "proxy.corp:8080", "http://proxy:8080", "https://proxy",
	"https://secure.proxy:443", "http://user:pw@proxy:1", "user@proxy", "user:@proxy:2",
	"http://u%40x:p%3Ay@proxy", "127.0.0.1:8080", "http://127.0.0.1", "[::1]:3128",
	"http://[2001:db8::1]:8080", "socks5://127.0.0.1:1080", "foo://host", "http://proxy/path",
	"http://proxy:8080/", "%zz", "http://pro xy", "proxy:port", "HTTP://PROXY:9", "localhost",
	"cache.corp.example.com:1234", "http://", "://x", "proxy:08080",
}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func mutate(r *rand.Rand, s string) string {
	if s == "" || r.Intn(6) != 0 {
		return s
	}
	// by rune: an Xo Str (an environment value) is UTF-8
	b := []rune(s)
	i := r.Intn(len(b))
	switch r.Intn(4) {
	case 0:
		return string(b[:i]) + string(b[i+1:])
	case 1:
		return string(b[:i]) + strings.ToUpper(string(b[i:i+1])) + string(b[i+1:])
	case 2:
		return string(b[:i]) + pick(r, []string{".", ":", "/", "[", "]", "*", " ", ",", "0"}) + string(b[i:])
	default:
		return s + string(b[i:])
	}
}

func noProxy(r *rand.Rand) string {
	n := r.Intn(5)
	var items []string
	for i := 0; i < n; i++ {
		items = append(items, mutate(r, pick(r, noProxyItems)))
	}
	if n == 4 {
		// a CIDR whose prefix length is a hostile digit run (task #352),
		// chosen by the first item's length so the stream stays as it was
		ds := hostile.Digits()
		items = append(items, "10.0.0.0/"+ds[len(items[0])%len(ds)])
	}
	return strings.Join(items, pick(r, []string{",", ",", ", ", " ,"}))
}

func reqURL(r *rand.Rand) string {
	scheme := "http"
	if r.Intn(2) == 0 {
		scheme = "https"
	}
	h, p := pick(r, hosts), pick(r, ports)
	if len(h)%7 == 3 {
		// a port that is a hostile digit run (task #352): past 65535 and
		// past Int, chosen by the host's length
		ds := hostile.Digits()
		p = ":" + strings.TrimPrefix(ds[len(h)%len(ds)], "+")
	}
	return scheme + "://" + h + p + pick(r, paths)
}

func hx(s string) string { return hex.EncodeToString([]byte(s)) }

func answer(c httpproxy.Config, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "skip"
	}
	setting := c.HTTPProxy
	if u.Scheme == "https" {
		setting = c.HTTPSProxy
	}
	if strings.Contains(setting, "://") {
		// Go retries a setting without a scheme and host as http:// plus
		// the setting (http://://x, http://http://pro xy): a proxy with no
		// host or the wrong one, which std/httpproxy ignores
		if q, err := url.Parse(setting); err != nil || q.Scheme == "" || q.Host == "" {
			return "skip"
		}
	}
	p, err := c.ProxyFunc()(u)
	if err != nil {
		return "skip"
	}
	if p == nil {
		return "nil"
	}
	if p.Scheme != "http" && p.Scheme != "https" || p.Hostname() == "" {
		return "skip"
	}
	port := p.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil {
			return "skip"
		}
		port = strconv.Itoa(n)
	}
	user, pw, pwset := "", "", "0"
	if p.User != nil {
		user = p.User.Username()
		if q, ok := p.User.Password(); ok {
			pw, pwset = q, "1"
		}
	}
	return "p " + hx(p.Scheme) + "|" + hx(strings.ToLower(p.Hostname())) + "|" + port + "|" + hx(user) + "|" + pwset + hx(pw)
}

func main() {
	seed, _ := strconv.ParseInt(os.Args[1], 10, 64)
	n, _ := strconv.Atoi(os.Args[2])
	r := rand.New(rand.NewSource(seed))
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for i := 0; i < n; i++ {
		c := httpproxy.Config{
			HTTPProxy:  mutate(r, pick(r, proxies)),
			HTTPSProxy: mutate(r, pick(r, proxies)),
			NoProxy:    noProxy(r),
			CGI:        r.Intn(10) == 0,
		}
		raw := reqURL(r)
		cgi := "0"
		if c.CGI {
			cgi = "1"
		}
		fmt.Fprintf(w, "== %d\nc %s\t%s\t%s\t%s\t%s\nw %s\n", i, hx(c.HTTPProxy), hx(c.HTTPSProxy), hx(c.NoProxy), cgi, hx(raw), answer(c, raw))
	}
}
