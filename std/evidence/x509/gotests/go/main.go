// Command go is the Go side of std/x509's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package x509's result as text;
// std/evidence/x509/gotests/main.xo prints std/x509's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func u64s(s string) []uint64 {
	var v []uint64
	if s != "" {
		for _, p := range strings.Split(s, ",") {
			n, _ := strconv.ParseUint(p, 10, 64)
			v = append(v, n)
		}
	}
	return v
}

func oidOut(o x509.OID, err error) string {
	if err != nil {
		return "err " + err.Error()
	}
	return o.String()
}

func oidBin(s string) string {
	var b []byte
	if s != "" {
		for _, p := range strings.Split(s, ",") {
			n, _ := strconv.Atoi(p)
			b = append(b, byte(n))
		}
	}
	var o x509.OID
	return oidOut(o, o.UnmarshalBinary(b))
}

func oidStr(s string) string {
	return oidOut(x509.ParseOID(s))
}

func oidInts(s string) string {
	return oidOut(x509.OIDFromInts(u64s(s)))
}

// orun: oid (raw, str, ints of oidTests) builds each, inv (str, ints)
// parses and builds, parse parses.
func orun(kind string, in []string) string {
	switch kind {
	case "oid":
		return "bin " + oidBin(in[0]) + " | str " + oidStr(in[1]) + " | ints " + oidInts(in[2])
	case "inv":
		return "str " + oidStr(in[0]) + " | ints " + oidInts(in[1])
	}
	return oidStr(in[0])
}

func run(kind string, in []string) string {
	switch kind {
	case "oid", "inv", "parse":
		return orun(kind, in)
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
