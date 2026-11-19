// Command go is the Go side of std/mail's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package mail's result as text;
// std/evidence/mail/gotests/main.xo prints std/mail's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/mail"
	"os"
	"strings"
)

func date(s string) string {
	t, err := mail.ParseDate(s)
	if err != nil {
		return "err " + err.Error()
	}
	_, off := t.Zone()
	return fmt.Sprintf("off=%d unix=%d %d", off, t.Unix(), t.UnixNano())
}

func list(s string) string {
	as, err := mail.ParseAddressList(s)
	if err != nil {
		return "err " + err.Error()
	}
	var out []string
	for _, a := range as {
		out = append(out, a.Name+"|"+a.Address+"|"+a.String())
	}
	return strings.Join(out, "; ")
}

func addr(s string) string {
	a, err := mail.ParseAddress(s)
	if err != nil {
		return "err " + err.Error()
	}
	return a.Name + "|" + a.Address + "|" + a.String()
}

func run(kind string, in []string) string {
	switch kind {
	case "date":
		return date(in[0])
	case "list":
		return list(in[0])
	case "addr":
		return addr(in[0])
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
