// Go twin of bench/perf/httpproxy_rules: x/net/http/httpproxy's
// Config.ProxyFunc over the same URLs.
package main

import (
	"fmt"
	"net/url"
	"os"

	"golang.org/x/net/http/httpproxy"
)

var urls = []string{"http://example.com/a", "https://example.com/b", "http://api.internal.corp/x", "https://svc.internal.corp:8443/y", "http://10.1.2.3/z", "https://10.9.9.9:443/", "http://192.168.0.5:8080/", "https://192.168.0.5/", "http://localhost:3000/", "https://127.0.0.1/", "http://foo.bar.example.org/", "https://example.org/", "http://[::1]:80/", "https://[2001:db8::1]/", "http://cdn.static.net/i.png", "https://static.net/"}

func cfg() *httpproxy.Config {
	return &httpproxy.Config{HTTPProxy: "proxy.corp:3128", HTTPSProxy: "https://secure-proxy.corp:443", NoProxy: "internal.corp,.example.org,10.0.0.0/8,192.168.0.5:8080,static.net,*.cdn.static.net,2001:db8::/32,localhost,1.2.3.4"}
}

func host(p *url.URL, err error) int {
	if err != nil || p == nil {
		return 1
	}
	return len(p.Host)
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	us := make([]*url.URL, 0, len(urls))
	for _, s := range urls {
		u, err := url.Parse(s)
		if err != nil {
			panic(err)
		}
		us = append(us, u)
	}
	sum := 0
	switch w {
	case "route":
		f := cfg().ProxyFunc()
		for r := 0; r < 200000; r++ {
			for _, u := range us {
				sum += host(f(u))
			}
		}
	case "rules":
		for i := 0; i < 300000; i++ {
			f := cfg().ProxyFunc()
			sum += host(f(us[i%16]))
		}
	}
	fmt.Printf("%s %d\n", w, sum)
}
