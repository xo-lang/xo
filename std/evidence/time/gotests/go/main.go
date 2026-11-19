// Command go is the Go side of std/time's gotests evidence (task #357):
// main CASES prints, for each case line `KIND<TAB>B64...`, the base64 of
// package time's result as text; std/evidence/time/gotests/main.xo prints
// std/time's the same way. See scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// pacific is Local in Go's time tests (ForceUSPacificForTesting).
var pacific *time.Location

func zone(name string) *time.Location {
	if name == "UTC" {
		return time.UTC
	}
	return pacific
}

func run(kind string, in []string) string {
	switch kind {
	case "date":
		// time.Date, which normalizes every field and takes the earlier
		// instant of a civil time a daylight saving change skips or repeats
		var n [7]int
		for i := range n {
			v, err := strconv.Atoi(in[i])
			if err != nil {
				return "bad input " + in[i]
			}
			n[i] = v
		}
		t := time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], n[6], zone(in[7]))
		return strconv.FormatInt(t.UnixNano(), 10)
	}
	return "unknown kind " + kind
}

func main() {
	var err error
	if pacific, err = time.LoadLocation("America/Los_Angeles"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
