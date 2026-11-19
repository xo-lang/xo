// Command go is the Go side of std/asn1's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package asn1's result as text;
// std/evidence/asn1/gotests/main.xo prints std/asn1's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

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

func bytesText(b []byte) string {
	s := make([]string, len(b))
	for i, c := range b {
		s[i] = strconv.Itoa(int(c))
	}
	return strings.Join(s, ",")
}

// tlv is DER for tag (universal, primitive) and content c.
func tlv(tag int, c []byte) []byte {
	out := []byte{byte(tag)}
	if n := len(c); n < 128 {
		out = append(out, byte(n))
	} else {
		var l []byte
		for ; n > 0; n >>= 8 {
			l = append([]byte{byte(n)}, l...)
		}
		out = append(out, 0x80|byte(len(l)))
		out = append(out, l...)
	}
	return append(out, c...)
}

func parse(kind string, in []string) string {
	var err error
	var s string
	switch kind {
	case "bool":
		var v bool
		_, err = asn1.Unmarshal(tlv(1, bytesOf(in[0])), &v)
		s = fmt.Sprint(v)
	case "int64":
		var v int64
		_, err = asn1.Unmarshal(tlv(2, bytesOf(in[0])), &v)
		s = fmt.Sprint(v)
	case "int32":
		var v int32
		_, err = asn1.Unmarshal(tlv(2, bytesOf(in[0])), &v)
		s = fmt.Sprint(v)
	case "bigint":
		var v *big.Int
		_, err = asn1.Unmarshal(tlv(2, bytesOf(in[0])), &v)
		if err == nil {
			s = v.String()
		}
	case "bitstring":
		var v asn1.BitString
		_, err = asn1.Unmarshal(tlv(3, bytesOf(in[0])), &v)
		s = fmt.Sprintf("%s/%d", bytesText(v.Bytes), v.BitLength)
	case "oid":
		var v asn1.ObjectIdentifier
		_, err = asn1.Unmarshal(tlv(6, bytesOf(in[0])), &v)
		s = v.String()
	case "utc", "gen":
		tag := 23
		if kind == "gen" {
			tag = 24
		}
		var v time.Time
		_, err = asn1.Unmarshal(tlv(tag, []byte(in[0])), &v)
		if err != nil {
			// Xo's time errors have its own texts (std/asn1 time_err).
			return "err"
		}
		s = fmt.Sprint(v.UnixNano())
	case "bmp":
		c, _ := hex.DecodeString(in[0])
		var v string
		_, err = asn1.Unmarshal(tlv(30, c), &v)
		s = fmt.Sprintf("%q", v)
	}
	if err != nil {
		return "err " + err.Error()
	}
	return "ok " + s
}

func rightAlign(in []string) string {
	n, _ := strconv.Atoi(in[1])
	return bytesText(asn1.BitString{Bytes: bytesOf(in[0]), BitLength: n}.RightAlign())
}

func run(kind string, in []string) string {
	switch kind {
	case "rightalign":
		return rightAlign(in)
	case "bool", "int64", "int32", "bigint", "bitstring", "oid", "utc", "gen", "bmp":
		return parse(kind, in)
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
