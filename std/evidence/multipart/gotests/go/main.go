// Command go is the Go side of std/multipart's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package multipart's result as text;
// std/evidence/multipart/gotests/main.xo prints std/multipart's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"
)

func boundary(b string) string {
	w := multipart.NewWriter(io.Discard)
	if err := w.SetBoundary(b); err != nil {
		return "err " + err.Error()
	}
	return "ok " + w.Boundary()
}

func run(kind string, in []string) string {
	switch kind {
	case "boundary":
		return boundary(in[0])
	case "disp":
		return multipart.FileContentDisposition(in[0], in[1])
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
