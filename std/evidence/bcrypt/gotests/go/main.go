// Command go is the Go side of std/bcrypt's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package bcrypt's result as text;
// std/evidence/bcrypt/gotests/main.xo prints std/bcrypt's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"os"
	"strings"
)

func run(kind string, in []string) string {
	switch kind {
	case "invalid":
		return errText(bcrypt.CompareHashAndPassword([]byte(in[0]), []byte("anything"))) + "\n" + costText(in[0])
	}
	return "unknown kind " + kind
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

func costText(h string) string {
	c, err := bcrypt.Cost([]byte(h))
	if err != nil {
		return err.Error()
	}
	return fmt.Sprint(c)
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
