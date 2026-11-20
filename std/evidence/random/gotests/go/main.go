// Command go is the Go side of std/random's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package math/rand/v2's result as text;
// std/evidence/random/gotests/main.xo prints std/random's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
)

// seed is chacha8_test.go's chacha8seed.
var seed = [32]byte([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"))

func run(kind string, in []string) string {
	switch kind {
	case "chacha8":
		// where the word is in the first 4096 of NewChaCha8(seed)
		v, err := strconv.ParseUint(in[0], 10, 64)
		if err != nil {
			return "bad input"
		}
		r := rand.NewChaCha8(seed)
		for i := 0; i < 4096; i++ {
			if r.Uint64() == v {
				return fmt.Sprint(i)
			}
		}
		return "absent"
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
