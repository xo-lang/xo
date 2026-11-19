// Command go is the Go side of std/xml's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package xml's result as text;
// std/evidence/xml/gotests/main.xo prints std/xml's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// xrun reads every token of in[0] (Decoder.Token, or RawToken for raw)
// until EOF: the count, or the error.
func xrun(kind string, in []string) string {
	d := xml.NewDecoder(strings.NewReader(in[0]))
	n := 0
	for {
		var err error
		if kind == "raw" {
			_, err = d.RawToken()
		} else {
			_, err = d.Token()
		}
		if err == io.EOF {
			return fmt.Sprintf("ok %d", n)
		}
		if err != nil {
			return "err " + err.Error()
		}
		n++
	}
}

func run(kind string, in []string) string {
	switch kind {
	case "tok", "raw":
		return xrun(kind, in)
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
