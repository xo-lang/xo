// Command go is the Go side of std/crypto's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package sha256's result as text;
// std/evidence/crypto/gotests/main.xo prints std/crypto's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"strings"
)

// hsum: the hex digest of in, then of in written as two halves.
func hsum(kind string, in []byte) string {
	var h hash.Hash
	switch kind {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha224":
		h = sha256.New224()
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	case "sha512_224":
		h = sha512.New512_224()
	default:
		h = sha512.New512_256()
	}
	h.Write(in)
	whole := hex.EncodeToString(h.Sum(nil))
	h.Reset()
	h.Write(in[:len(in)/2])
	h.Write(in[len(in)/2:])
	return whole + " " + hex.EncodeToString(h.Sum(nil))
}

func run(kind string, in []string) string {
	switch kind {
	case "md5", "sha1", "sha224", "sha256", "sha384", "sha512", "sha512_224", "sha512_256":
		return hsum(kind, []byte(in[0]))
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
