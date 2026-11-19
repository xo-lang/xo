// Command go is the Go side of std/idna's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package idna's result as text;
// std/evidence/idna/gotests/main.xo prints std/idna's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"golang.org/x/net/idna"
	"os"
	"strings"
)

func show(s string, err error) string {
	if err != nil {
		return "error " + err.Error()
	}
	return "ok " + s
}

func run(kind string, in []string) string {
	switch kind {
	case "alookup":
		return show(idna.Lookup.ToASCII(in[0]))
	case "ulookup":
		return show(idna.Lookup.ToUnicode(in[0]))
	case "aregistration":
		return show(idna.Registration.ToASCII(in[0]))
	case "uregistration":
		return show(idna.Registration.ToUnicode(in[0]))
	case "adisplay":
		return show(idna.Display.ToASCII(in[0]))
	case "udisplay":
		return show(idna.Display.ToUnicode(in[0]))
	case "apunycode":
		return show(idna.Punycode.ToASCII(in[0]))
	case "upunycode":
		return show(idna.Punycode.ToUnicode(in[0]))
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
