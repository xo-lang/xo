// Command go is the Go side of std/adler32's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package adler32's result as text;
// std/evidence/adler32/gotests/main.xo prints std/adler32's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"hash/adler32"
	"os"
	"strings"
)

// adler32Split is the checksum of d[:h] continued over d[h:] through the
// hash.Hash32 (package adler32 has no Update function).
func adler32Split(d []byte, h int) uint32 {
	w := adler32.New()
	w.Write(d[:h])
	w.Write(d[h:])
	return w.Sum32()
}

func sums(d []byte) string {
	h := len(d) / 2
	var out []string
	out = append(out, fmt.Sprintf("%d %d", adler32.Checksum(d), adler32Split(d, h)))
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
