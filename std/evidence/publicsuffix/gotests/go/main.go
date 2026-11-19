// Command go is the Go side of std/publicsuffix's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package publicsuffix's result as text;
// std/evidence/publicsuffix/gotests/main.xo prints std/publicsuffix's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"golang.org/x/net/publicsuffix"
	"os"
	"strings"
)

func run(kind string, in []string) string {
	switch kind {
	case "ps":
		s, icann := publicsuffix.PublicSuffix(in[0])
		return fmt.Sprintf("%s %v", s, icann)
	case "etld1":
		s, _ := publicsuffix.EffectiveTLDPlusOne(in[0])
		return s
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
