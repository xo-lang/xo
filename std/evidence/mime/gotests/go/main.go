// Command go is the Go side of std/mime's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package mime's result as text;
// std/evidence/mime/gotests/main.xo prints std/mime's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"sort"
	"strings"
)

var decoder = new(mime.WordDecoder)

func media(s string) string {
	mt, params, err := mime.ParseMediaType(s)
	if err != nil {
		return "err " + err.Error()
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := mt
	for _, k := range keys {
		out += ";" + k + "=" + params[k]
	}
	return out
}

func or(s string, err error) string {
	if err != nil {
		return "err " + err.Error()
	}
	return s
}

func run(kind string, in []string) string {
	switch kind {
	case "word":
		return or(decoder.Decode(in[0]))
	case "header":
		return or(decoder.DecodeHeader(in[0]))
	case "media":
		return media(in[0])
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
