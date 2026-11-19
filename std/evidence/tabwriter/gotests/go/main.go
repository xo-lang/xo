// Command go is the Go side of std/tabwriter's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package tabwriter's result as text;
// std/evidence/tabwriter/gotests/main.xo prints std/tabwriter's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

var flagNames = map[string]uint{"FilterHTML": tabwriter.FilterHTML, "StripEscape": tabwriter.StripEscape, "AlignRight": tabwriter.AlignRight,
	"DiscardEmptyColumns": tabwriter.DiscardEmptyColumns, "TabIndent": tabwriter.TabIndent, "Debug": tabwriter.Debug}

func flags(s string) uint {
	var n uint
	for _, f := range strings.Split(s, "|") {
		if v, ok := flagNames[f]; ok {
			n |= v
		} else if v, err := strconv.Atoi(f); err == nil {
			n |= uint(v)
		}
	}
	return n
}

func toStr(b []byte) string {
	if !utf8.Valid(b) {
		return "not utf-8"
	}
	return string(b)
}

// format is the output of src written whole, then written a byte at a
// time, joined by a line "--".
func format(in []string) string {
	var n [4]int
	for i := range n {
		v, err := strconv.Atoi(in[i])
		if err != nil {
			return "bad input " + in[i]
		}
		n[i] = v
	}
	src := []byte(in[5])
	var whole, bytewise bytes.Buffer
	w := tabwriter.NewWriter(&whole, n[0], n[1], n[2], byte(n[3]), flags(in[4]))
	w.Write(src)
	w.Flush()
	w1 := tabwriter.NewWriter(&bytewise, n[0], n[1], n[2], byte(n[3]), flags(in[4]))
	for i := range src {
		w1.Write(src[i : i+1])
	}
	w1.Flush()
	return toStr(whole.Bytes()) + "\n--\n" + toStr(bytewise.Bytes())
}

func run(kind string, in []string) string {
	switch kind {
	case "format":
		return format(in)
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
