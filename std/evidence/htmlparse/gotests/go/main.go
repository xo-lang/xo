// Command go is the Go side of std/htmlparse's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package html's result as text;
// std/evidence/htmlparse/gotests/main.xo prints std/htmlparse's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"golang.org/x/net/html"
	"os"
	"strings"
)

func run(kind string, in []string) string {
	switch kind {
	case "render":
		return render(in[0], true)
	case "noscript":
		return render(in[0], false)
	}
	return "unknown kind " + kind
}

// render is the rendered tree of the parsed text, or the error.
func render(s string, scripting bool) string {
	n, err := html.ParseWithOptions(strings.NewReader(s), html.ParseOptionEnableScripting(scripting))
	if err != nil {
		return "parse: " + err.Error()
	}
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		return b.String() + "|render: " + err.Error()
	}
	return b.String()
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
