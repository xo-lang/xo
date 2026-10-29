// The Go twin of bench/perf/httpproxy's proxy: httputil.ReverseProxy
// with Rewrite (SetURL, SetXForwarded) from -addr to -upstream.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18099", "listen address")
	up := flag.String("upstream", "127.0.0.1:18097", "upstream address")
	flag.Parse()
	target, _ := url.Parse("http://" + *up + "/")
	p := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.SetXForwarded()
	}}
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		panic(err)
	}
	fmt.Println("ready")
	http.Serve(l, p)
}
