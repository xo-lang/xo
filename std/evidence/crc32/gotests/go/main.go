// Command go is the Go side of std/crc32's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package crc32's result as text;
// std/evidence/crc32/gotests/main.xo prints std/crc32's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"os"
	"strings"
)

func sums(d []byte) string {
	h := len(d) / 2
	var out []string
	t0 := crc32.IEEETable
	out = append(out, fmt.Sprintf("%d %d", (crc32.Checksum(d, t0)), (crc32.Update(crc32.Checksum(d[:h], t0), t0, d[h:]))))
	t1 := crc32.MakeTable(crc32.Castagnoli)
	out = append(out, fmt.Sprintf("%d %d", (crc32.Checksum(d, t1)), (crc32.Update(crc32.Checksum(d[:h], t1), t1, d[h:]))))
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
