// Command go is the Go side of std/url's gotests evidence (task #357):
// main CASES prints, for each case line `KIND<TAB>B64...`, the base64 of
// net/url's result as text; std/evidence/url/gotests/main.xo prints
// std/url's the same way. See scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"sort"
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

func unescaped(s string, err error) string {
	if err != nil {
		return errText(err)
	}
	return "ok " + s
}

func valuesText(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + strings.Join(v[k], ",") + sep)
	}
	return b.String() + "encode " + v.Encode()
}

func run(kind string, in []string) string {
	switch kind {
	case "parse":
		return parsed(url.Parse(in[0]))
	case "request":
		return parsed(url.ParseRequestURI(in[0]))
	case "unescape":
		return unescaped(url.QueryUnescape(in[0])) + sep + unescaped(url.PathUnescape(in[0]))
	case "query_escape":
		return url.QueryEscape(in[0])
	case "path_escape":
		return url.PathEscape(in[0])
	case "resolve":
		base, err := url.Parse(in[0])
		if err != nil {
			return "base " + errText(err)
		}
		ref, err := url.Parse(in[1])
		if err != nil {
			return "ref " + errText(err)
		}
		return urlText(base.ResolveReference(ref))
	case "parse_query":
		// std/url's parse_query returns no pairs with its error, so a
		// failing query compares the error alone
		v, err := url.ParseQuery(in[0])
		if err != nil {
			return "partial" + sep + err.Error()
		}
		return valuesText(v) + sep + "nil"
	case "hostport":
		u := &url.URL{Host: in[0]}
		return u.Hostname() + sep + u.Port()
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
