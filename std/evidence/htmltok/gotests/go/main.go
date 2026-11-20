// Command go is the Go side of std/htmltok's gotests evidence (decision
// 0720, task #403): main CASES prints, for each case line `KIND<TAB>B64...`,
// the base64 of golang.org/x/net/html's result as text;
// std/evidence/htmltok/gotests/main.xo prints std/htmltok's the same way.
// See scripts/stdproven/gotests.go.
//
// Kind tokens: TestTokenizer's loop with AllowCDATA on, every Token's
// String joined by "$", then "|" and the error that ended it. Kind raw: each
// token's type and Raw text. Kind unescape: UnescapeString.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"golang.org/x/net/html"
)

func tokens(s string, raw bool) string {
	z := html.NewTokenizer(strings.NewReader(s))
	z.AllowCDATA(true)
	var out []string
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if raw {
			out = append(out, tt.String()+":"+string(z.Raw()))
		} else {
			out = append(out, z.Token().String())
		}
	}
	return strings.Join(out, "$") + "|" + z.Err().Error()
}

func run(kind string, in []string) string {
	switch kind {
	case "tokens":
		return tokens(in[0], false)
	case "raw":
		return tokens(in[0], true)
	case "unescape":
		return html.UnescapeString(in[0])
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
