// Command go is the Go side of std/textproto's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package textproto's result as text;
// std/evidence/textproto/gotests/main.xo prints std/textproto's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/textproto"
	"os"
	"strconv"
	"strings"
)

func response(s, code string) string {
	r := textproto.NewReader(bufio.NewReader(strings.NewReader(s + "\nFOLLOWING DATA")))
	n, _ := strconv.Atoi(code)
	c, msg, err := r.ReadResponse(n)
	if err != nil {
		return "err " + err.Error()
	}
	rest, err := r.ReadLine()
	if err != nil {
		rest = "-"
	}
	return fmt.Sprintf("%d|%s|%s", c, msg, rest)
}

func run(kind string, in []string) string {
	switch kind {
	case "canonical":
		return textproto.CanonicalMIMEHeaderKey(in[0])
	case "response":
		return response(in[0], in[1])
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
