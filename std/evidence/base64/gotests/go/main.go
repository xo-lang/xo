// Command go is the Go side of std/base64's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package base64's result as text;
// std/evidence/base64/gotests/main.xo prints std/base64's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func nums(b []byte) string {
	out := make([]string, len(b))
	for i, x := range b {
		out[i] = strconv.Itoa(int(x))
	}
	return "[" + strings.Join(out, ",") + "]"
}

func decodeAll(s string) string {
	var out []string
	if b, err := base64.StdEncoding.DecodeString(s); err != nil {
		out = append(out, "err "+err.Error())
	} else {
		out = append(out, nums(b))
	}
	if b, err := base64.URLEncoding.DecodeString(s); err != nil {
		out = append(out, "err "+err.Error())
	} else {
		out = append(out, nums(b))
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err != nil {
		out = append(out, "err "+err.Error())
	} else {
		out = append(out, nums(b))
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err != nil {
		out = append(out, "err "+err.Error())
	} else {
		out = append(out, nums(b))
	}
	if b, err := base64.StdEncoding.Strict().DecodeString(s); err != nil {
		out = append(out, "err "+err.Error())
	} else {
		out = append(out, nums(b))
	}
	return strings.Join(out, "\n")
}

func encodeAll(s string) string {
	var out []string
	out = append(out, base64.StdEncoding.EncodeToString([]byte(s)))
	out = append(out, base64.URLEncoding.EncodeToString([]byte(s)))
	out = append(out, base64.RawStdEncoding.EncodeToString([]byte(s)))
	out = append(out, base64.RawURLEncoding.EncodeToString([]byte(s)))
	return strings.Join(out, "\n")
}

func run(kind string, in []string) string {
	switch kind {
	case "decode":
		return decodeAll(in[0])
	case "encode":
		return encodeAll(in[0])
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
