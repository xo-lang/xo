// Command go is the Go side of std/cookiejar's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package cookiejar's result as text;
// std/evidence/cookiejar/gotests/main.xo prints std/cookiejar's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
)

// jrun: a fresh jar with no public suffix list (cookiejar.New(nil); the
// tables' testPSL is the package's own), the Set-Cookie lines from
// fromURL stored, then each query's cookies as Go's test writes them.
func jrun(from, sets, qs string) string {
	var lines []string
	var queries [][]string
	if json.Unmarshal([]byte(sets), &lines) != nil || json.Unmarshal([]byte(qs), &queries) != nil {
		return "err json"
	}
	jar, _ := cookiejar.New(nil)
	u, err := url.Parse(from)
	if err != nil {
		return "err url"
	}
	var cs []*http.Cookie
	for _, l := range lines {
		c, err := http.ParseSetCookie(l)
		if err == nil {
			cs = append(cs, c)
		}
	}
	jar.SetCookies(u, cs)
	var out []string
	for _, q := range queries {
		qu, err := url.Parse(q[0])
		if err != nil {
			out = append(out, "err url")
			continue
		}
		var s []string
		for _, c := range jar.Cookies(qu) {
			v := c.Value
			if c.Quoted || strings.ContainsAny(v, " ,") {
				v = `"` + v + `"`
			}
			s = append(s, c.Name+"="+v)
		}
		out = append(out, strings.Join(s, " "))
	}
	return strings.Join(out, "|")
}

func run(kind string, in []string) string {
	switch kind {
	case "jar":
		return jrun(in[0], in[1], in[2])
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
