// Command go is the Go side of std/httpproxy's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package httpproxy's result as text;
// std/evidence/httpproxy/gotests/main.xo prints std/httpproxy's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/net/http/httpproxy"
	"net/url"
	"os"
	"strings"
)

const noProxy = "foobar.com, .barbaz.net, *.wildcard.io, 192.168.1.1, 192.168.1.2:81, 192.168.1.3:80, 10.0.0.0/30, 2001:db8::52:0:1, [2001:db8::52:0:2]:443, [2001:db8::52:0:3]:80, 2002:db8:a::45/64"

func useProxy(no, host string) string {
	cfg := &httpproxy.Config{HTTPProxy: "http://proxy.example.org", HTTPSProxy: "https://proxy.example.org", NoProxy: no}
	u, err := url.Parse("https://" + host + ":80")
	if err != nil {
		return "bad url"
	}
	p, err := cfg.ProxyFunc()(u)
	if err != nil {
		return "error"
	}
	return fmt.Sprint(p != nil)
}

// forURL is the proxy for a request URL under a Config given as JSON (Go's
// TestProxyForURL), "" to connect directly. A setting std/httpproxy
// documents as ignored is blanked first: one ProxyFunc fails on or gives
// a scheme other than http or https for (socks5://, a value that does not
// parse), and HTTPProxy under CGI, where Go's ProxyFunc returns an error
// for http requests and Xo connects directly.
func forURL(cfgJSON, req string) string {
	var cfg httpproxy.Config
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return "bad cfg " + err.Error()
	}
	cfg.HTTPProxy = usable(cfg.HTTPProxy, "http")
	cfg.HTTPSProxy = usable(cfg.HTTPSProxy, "https")
	if cfg.CGI {
		cfg.HTTPProxy = ""
	}
	u, err := url.Parse(req)
	if err != nil {
		return "bad url"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		// http.Url holds http and https URLs only; ProxyFunc gives nil
		return "not http"
	}
	p, err := cfg.ProxyFunc()(u)
	if err != nil {
		return "err " + err.Error()
	}
	if p == nil {
		return ""
	}
	if p.Path == "" {
		// http.Url's path is "/" for a URL with none
		p.Path = "/"
	}
	return p.String()
}

// usable is setting, or "" when ProxyFunc rejects it or gives a proxy that
// is not http or https for a request of scheme to a host no_proxy keeps.
func usable(setting, scheme string) string {
	if setting == "" {
		return ""
	}
	cfg := httpproxy.Config{HTTPProxy: setting, HTTPSProxy: setting}
	p, err := cfg.ProxyFunc()(&url.URL{Scheme: scheme, Host: "example.com"})
	if err != nil || p == nil || (p.Scheme != "http" && p.Scheme != "https") {
		return ""
	}
	return setting
}

func run(kind string, in []string) string {
	switch kind {
	case "use":
		return useProxy(noProxy, in[0])
	case "useall":
		return useProxy("*", in[0])
	case "forurl":
		return forURL(in[0], in[1])
	}
	return "unknown kind " + kind
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		in := make([]string, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, string(b))
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
