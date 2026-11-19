// Command go is the Go side of std/norm's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package norm's result as text;
// std/evidence/norm/gotests/main.xo prints std/norm's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"os"
	"strings"
)

func run(kind string, in []string) string {
	switch kind {
	case "isnfc":
		return fmt.Sprint(norm.NFC.IsNormalString(in[0]))
	case "nfc":
		return norm.NFC.String(in[0] + in[1])
	case "isnfd":
		return fmt.Sprint(norm.NFD.IsNormalString(in[0]))
	case "nfd":
		return norm.NFD.String(in[0] + in[1])
	case "isnfkc":
		return fmt.Sprint(norm.NFKC.IsNormalString(in[0]))
	case "nfkc":
		return norm.NFKC.String(in[0] + in[1])
	case "isnfkd":
		return fmt.Sprint(norm.NFKD.IsNormalString(in[0]))
	case "nfkd":
		return norm.NFKD.String(in[0] + in[1])
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
