// Command gennorm prints the Unicode normalization tables of std/norm from
// golang.org/x/text/unicode/norm (Unicode 15.0.0 for the go1.26 build): the
// full canonical and compatibility decompositions, the combining classes and
// the primary composition pairs, derived through the package's public API.
// Usage: GOFLAGS=-mod=mod GOPROXY=off go run . [stats]
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func list(name string, doc string, xs []int, w *strings.Builder) {
	fmt.Fprintf(w, "%s\nconst %s: List[Int] = [", doc, name)
	for i, x := range xs {
		if i%12 == 0 {
			w.WriteString("\n  ")
		} else {
			w.WriteString(" ")
		}
		fmt.Fprintf(w, "%d,", x)
	}
	w.WriteString("\n]\n\n")
}

func main() {
	path := os.Args[1]
	var ck, co, cr, kk, ko, kr, cc, pk, pv []int
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF || (r >= 0xAC00 && r <= 0xD7A3) {
			continue
		}
		s := string(r)
		d := norm.NFD.String(s)
		k := norm.NFKD.String(s)
		if d != s {
			ck = append(ck, int(r))
			co = append(co, len(cr))
			for _, x := range d {
				cr = append(cr, int(x))
			}
			rs := []rune(d)
			if len(rs) >= 2 {
				a := norm.NFC.String(string(rs[:len(rs)-1]))
				b := rs[len(rs)-1]
				if utf8.RuneCountInString(a) == 1 && norm.NFC.String(a+string(b)) == s {
					pk = append(pk, int([]rune(a)[0])<<21|int(b))
					pv = append(pv, int(r))
				}
			}
		}
		if k != d {
			kk = append(kk, int(r))
			ko = append(ko, len(kr))
			for _, x := range k {
				kr = append(kr, int(x))
			}
		}
		if c := int(norm.NFD.PropertiesString(s).CCC()); c != 0 {
			n := len(cc)
			if n >= 3 && cc[n-1] == c && cc[n-2] == int(r)-1 {
				cc[n-2] = int(r)
			} else {
				cc = append(cc, int(r), int(r), c)
			}
		}
	}
	co = append(co, len(cr))
	ko = append(ko, len(kr))
	idx := make([]int, len(pk))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return pk[idx[a]] < pk[idx[b]] })
	pk2, pv2 := make([]int, len(pk)), make([]int, len(pk))
	for n, i := range idx {
		pk2[n], pv2[n] = pk[i], pv[i]
	}
	pk, pv = pk2, pv2
	var w strings.Builder
	fmt.Fprintf(&w, "// Unicode %s, from golang.org/x/text/unicode/norm (scripts/gennorm): %d canonical and %d compatibility decompositions, %d combining class ranges, %d composition pairs.\n", norm.Version, len(ck), len(kk), len(cc)/3, len(pk))
	w.WriteString("const UNICODE_VERSION: Str = \"" + norm.Version + "\"\n\n")
	list("CANON_KEYS", "/// Runes with a canonical decomposition, ascending (Hangul syllables are algorithmic).", ck, &w)
	list("CANON_OFFS", "/// Where the decomposition of CANON_KEYS[i] starts in CANON_RUNES; one past the last at the end.", co, &w)
	list("CANON_RUNES", "/// The full canonical decompositions (NFD) of CANON_KEYS, one after the other.", cr, &w)
	list("COMPAT_KEYS", "/// Runes whose full compatibility decomposition (NFKD) differs from their NFD, ascending.", kk, &w)
	list("COMPAT_OFFS", "/// Where the NFKD of COMPAT_KEYS[i] starts in COMPAT_RUNES; one past the last at the end.", ko, &w)
	list("COMPAT_RUNES", "/// The full compatibility decompositions of COMPAT_KEYS, one after the other.", kr, &w)
	list("CCC", "/// Canonical combining classes as (first rune, last rune, class) triples, ascending.", cc, &w)
	list("COMP_KEYS", "/// Primary composition pairs, first * 2**21 + second, ascending.", pk, &w)
	list("COMP_VALS", "/// The rune each COMP_KEYS pair composes to.", pv, &w)
	// sort pairs by key
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	src := string(b)
	i := strings.Index(src, "// BEGIN GENERATED\n")
	j := strings.Index(src, "// END GENERATED\n")
	if i < 0 || j < 0 {
		panic("markers missing")
	}
	out := src[:i] + "// BEGIN GENERATED\n" + w.String() + src[j:]
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		panic(err)
	}
}
