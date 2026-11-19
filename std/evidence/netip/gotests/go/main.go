// Command go is the Go side of std/netip's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package netip's result as text;
// std/evidence/netip/gotests/main.xo prints std/netip's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

func addr(s string) string {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "err " + err.Error()
	}
	return a.String()
}

func addrPort(s string) string {
	a, err := netip.ParseAddrPort(s)
	if err != nil {
		return "err " + err.Error()
	}
	return a.String()
}

func prefix(s string) string {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return "err " + err.Error()
	}
	return p.String() + " masked " + p.Masked().String() + fmt.Sprintf(" single %v", p.IsSingleIP())
}

func orNone(a netip.Addr) string {
	if !a.IsValid() {
		return "none"
	}
	return a.String()
}

func props(s string) string {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "err " + err.Error()
	}
	return fmt.Sprintf("lb=%v pv=%v un=%v ll=%v mc=%v 4=%v 6=%v 4in6=%v unmap=%s next=%s prev=%s",
		a.IsLoopback(), a.IsPrivate(), a.IsUnspecified(), a.IsLinkLocalUnicast(), a.IsMulticast(),
		a.Is4(), a.Is6(), a.Is4In6(), a.Unmap(), orNone(a.Next()), orNone(a.Prev()))
}

func contains(p, a string) string {
	pp, err := netip.ParsePrefix(p)
	if err != nil {
		return "err " + err.Error()
	}
	aa, err := netip.ParseAddr(a)
	if err != nil {
		return "err " + err.Error()
	}
	return fmt.Sprint(pp.Contains(aa))
}

func run(kind string, in []string) string {
	switch kind {
	case "addr":
		return addr(in[0])
	case "addrport":
		return addrPort(in[0])
	case "prefix":
		return prefix(in[0])
	case "corpus":
		return addr(in[0]) + " / " + addrPort(in[0]) + " / " + prefix(in[0])
	case "props":
		return props(in[0])
	case "contains":
		return contains(in[0], in[1])
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
