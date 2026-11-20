// Command go is the Go side of std/cbor's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of fxamacker/cbor v2's result as text;
// std/evidence/cbor/gotests/main.xo prints std/cbor's the same way. See
// scripts/stdproven/gotests.go.
//
// The input is the case's CBOR in hex (the tables' mustHexDecode). Kind
// diag: Diagnose's text with the default options, or "err". Kind decode:
// Unmarshal into an any, then "ok" and the item's diagnostic notation, or
// "err" (the error texts are each library's own).
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

func diag(data []byte) string {
	s, err := cbor.Diagnose(data)
	if err != nil {
		return "err"
	}
	return s
}

func decode(data []byte) string {
	var v any
	if err := cbor.Unmarshal(data, &v); err != nil {
		return "err"
	}
	return "ok " + diag(data)
}

func run(kind string, in []string) string {
	data, err := hex.DecodeString(in[0])
	if err != nil {
		return "bad hex"
	}
	switch kind {
	case "diag":
		return diag(data)
	case "decode":
		return decode(data)
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
