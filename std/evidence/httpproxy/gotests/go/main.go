// Command go is the Go side of std/httpproxy's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package httpproxy's result as text;
// std/evidence/httpproxy/gotests/main.xo prints std/httpproxy's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
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

func run(kind string, in []string) string {
	switch kind {
	case "use":
		return useProxy(noProxy, in[0])
	case "useall":
		return useProxy("*", in[0])
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
