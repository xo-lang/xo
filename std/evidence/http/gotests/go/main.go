// Command go is the Go side of std/http's gotests evidence (task #357):
// main CASES prints, for each case line `KIND<TAB>B64...`, the base64 of
// net/url's result as text; std/evidence/http/gotests/main.xo prints
// http.parse_url's the same way. See scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const sep = "\x1f"

func urlText(u *url.URL) string {
	user := "-"
	if u.User != nil {
		user = "u=" + u.User.Username()
		if p, ok := u.User.Password(); ok {
			user += " p=" + p
		}
	}
	return strings.Join([]string{"ok", u.Scheme, u.Opaque, user, u.Host, u.Path, u.RawPath,
		fmt.Sprint(u.OmitHost), fmt.Sprint(u.ForceQuery), u.RawQuery, u.Fragment, u.RawFragment, u.String()}, sep)
}

func errText(err error) string { return "err " + err.Error() }

func parsed(u *url.URL, err error) string {
	if err != nil {
		return errText(err)
	}
	return urlText(u)
}

func run(kind string, in []string) string {
	switch kind {
	case "parse_url":
		// http.parse_url's projection of url.Parse: Go's error text, or
		// "rejected" where http.Url has no form for the URL (a scheme
		// other than http and https, no host), else the fields http.Url
		// keeps (an empty path is "/")
		u, err := url.Parse(in[0])
		if err != nil {
			return errText(err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "rejected"
		}
		user, pw := "", "-"
		if u.User != nil {
			user = u.User.Username()
			if p, ok := u.User.Password(); ok {
				pw = "p=" + p
			}
		}
		path := u.Path
		if path == "" {
			path = "/"
		}
		return strings.Join([]string{"ok", u.Scheme, u.Hostname(), u.Port(), user, pw, path}, sep)
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
		in := make([]string, len(f)-1)
		for i, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in[i] = string(b)
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
