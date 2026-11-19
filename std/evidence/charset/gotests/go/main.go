// Command go is the Go side of std/charset's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package utf16's result as text;
// std/evidence/charset/gotests/main.xo prints std/charset's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

func ints(s string) []int {
	var out []int
	if s == "" {
		return out
	}
	for _, f := range strings.Split(s, ",") {
		n, _ := strconv.Atoi(f)
		out = append(out, n)
	}
	return out
}

func join[T int | uint16 | rune](xs []T) string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strconv.Itoa(int(x))
	}
	return strings.Join(out, ",")
}

func encode(s string) string {
	var rs []rune
	var lens, pairs []int
	for _, n := range ints(s) {
		r := rune(n)
		rs = append(rs, r)
		lens = append(lens, utf16.RuneLen(r))
		a, b := utf16.EncodeRune(r)
		pairs = append(pairs, int(a), int(b))
	}
	return join(utf16.Encode(rs)) + "|" + join(lens) + "|" + join(pairs)
}

func decode(s string) string {
	var us []uint16
	for _, n := range ints(s) {
		us = append(us, uint16(n))
	}
	var pairs []rune
	var surr []string
	for i := range us {
		if i+1 < len(us) {
			pairs = append(pairs, utf16.DecodeRune(rune(us[i]), rune(us[i+1])))
		}
		surr = append(surr, strconv.FormatBool(utf16.IsSurrogate(rune(us[i]))))
	}
	return join(utf16.Decode(us)) + "|" + join(pairs) + "|" + strings.Join(surr, ",")
}

func run(kind string, in []string) string {
	switch kind {
	case "encode":
		return encode(in[0])
	case "decode":
		return decode(in[0])
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
