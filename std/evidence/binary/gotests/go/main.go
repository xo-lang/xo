// Command go is the Go side of std/binary's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package binary's result as text;
// std/evidence/binary/gotests/main.xo prints std/binary's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func bytesText(b []byte) string {
	s := make([]string, len(b))
	for i, c := range b {
		s[i] = strconv.Itoa(int(c))
	}
	return strings.Join(s, ",")
}

func bytesOf(s string) []byte {
	var b []byte
	if s != "" {
		for _, p := range strings.Split(s, ",") {
			n, _ := strconv.Atoi(p)
			b = append(b, byte(n))
		}
	}
	return b
}

// decText is Xo's view of a read: None for n <= 0.
func decText(v int64, n int) string {
	if n <= 0 {
		return "none"
	}
	return fmt.Sprintf("%d %d", v, n)
}

func varintCase(s string) string {
	x, _ := strconv.ParseInt(s, 10, 64)
	v := binary.AppendVarint(nil, x)
	u := binary.AppendUvarint(nil, uint64(x))
	vv, vn := binary.Varint(v)
	uv, un := binary.Uvarint(u)
	return bytesText(v) + " " + decText(vv, vn) + " " + bytesText(u) + " " + decText(int64(uv), un)
}

func run(kind string, in []string) string {
	switch kind {
	case "varint":
		return varintCase(in[0])
	case "uvarint":
		v, n := binary.Uvarint(bytesOf(in[0]))
		return decText(int64(v), n)
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
