// Command go is the Go side of std/gif's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package gif's result as text;
// std/evidence/gif/gotests/main.xo prints std/gif's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"image/gif"
	"os"
	"strings"
)

func loopCount(data string) string {
	g, err := gif.DecodeAll(bytes.NewReader([]byte(data)))
	if err != nil {
		return "err " + err.Error()
	}
	var w bytes.Buffer
	if err := gif.EncodeAll(&w, g); err != nil {
		return "err " + err.Error()
	}
	g1, err := gif.DecodeAll(&w)
	if err != nil {
		return "err " + err.Error()
	}
	return fmt.Sprintf("%d %d", g.LoopCount, g1.LoopCount)
}

func run(kind string, in []string) string {
	switch kind {
	case "loop":
		return loopCount(in[0])
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
