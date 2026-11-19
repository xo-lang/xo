// Command go is the Go side of std/fnv's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package fnv's result as text;
// std/evidence/fnv/gotests/main.xo prints std/fnv's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"hash"
	"hash/fnv"
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

// sum is the hash of s, and for 32 and 64 bits also of its first half
// continued over the rest (std/fnv's update), as Int text (int64 bits).
func sum(kind, s string) string {
	d := []byte(s)
	h := len(d) / 2
	pair := func(f func() hash.Hash) string {
		w := f()
		w.Write(d)
		whole := w.Sum(nil)
		w2 := f()
		w2.Write(d[:h])
		w2.Write(d[h:])
		split := w2.Sum(nil)
		if len(whole) == 16 {
			return nums(whole)
		}
		var a, b uint64
		for i := range whole {
			a = a<<8 | uint64(whole[i])
			b = b<<8 | uint64(split[i])
		}
		return fmt.Sprintf("%d %d", int64(a), int64(b))
	}
	switch kind {
	case "32":
		return pair(func() hash.Hash { return fnv.New32() })
	case "32a":
		return pair(func() hash.Hash { return fnv.New32a() })
	case "64":
		return pair(func() hash.Hash { return fnv.New64() })
	case "64a":
		return pair(func() hash.Hash { return fnv.New64a() })
	case "128":
		return pair(fnv.New128)
	case "128a":
		return pair(fnv.New128a)
	}
	return "unknown kind " + kind
}

func run(kind string, in []string) string {
	switch kind {
	case "32", "32a", "64", "64a", "128", "128a":
		return sum(kind, in[0])
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
