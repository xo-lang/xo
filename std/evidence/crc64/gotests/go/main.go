// Command go is the Go side of std/crc64's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package crc64's result as text;
// std/evidence/crc64/gotests/main.xo prints std/crc64's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"hash/crc64"
	"os"
	"strings"
)

func sums(d []byte) string {
	h := len(d) / 2
	var out []string
	t0 := crc64.MakeTable(crc64.ISO)
	out = append(out, fmt.Sprintf("%d %d", int64(crc64.Checksum(d, t0)), int64(crc64.Update(crc64.Checksum(d[:h], t0), t0, d[h:]))))
	t1 := crc64.MakeTable(crc64.ECMA)
	out = append(out, fmt.Sprintf("%d %d", int64(crc64.Checksum(d, t1)), int64(crc64.Update(crc64.Checksum(d[:h], t1), t1, d[h:]))))
	return strings.Join(out, "; ")
}

func run(kind string, in [][]byte) string {
	switch kind {
	case "sum":
		return sums(in[0])
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
		in := make([][]byte, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, b)
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
