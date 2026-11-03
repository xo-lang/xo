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

func lit(b []byte) string {
	var w strings.Builder
	w.WriteString(`b"`)
	for _, c := range b {
		fmt.Fprintf(&w, `\x%02x`, c)
	}
	w.WriteString(`"`)
	return w.String()
}

func varint(b []byte, v int) []byte {
	for v >= 0x80 {
		b = append(b, byte(v&0x7f|0x80))
		v >>= 7
	}
	return append(b, byte(v))
}

// decomps packs sorted (key, runes) as 7 byte records (the key in 3 bytes, the
// count in 1, the position of the varint coded runes in the pool in 3) and the pool.
func decomps(keys, offs, runes []int) (recs, pool []byte) {
	for i, k := range keys {
		pos := len(pool)
		n := offs[i+1] - offs[i]
		for _, r := range runes[offs[i]:offs[i+1]] {
			pool = varint(pool, r)
		}
		recs = append(recs, byte(k), byte(k>>8), byte(k>>16), byte(n), byte(pos), byte(pos>>8), byte(pos>>16))
	}
	return
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
	cr7, cp := decomps(ck, co, cr)
	kr7, kp := decomps(kk, ko, kr)
	var ccb []byte
	for i := 0; i < len(cc); i += 3 {
		ccb = append(ccb, byte(cc[i]), byte(cc[i]>>8), byte(cc[i]>>16), byte(cc[i+1]), byte(cc[i+1]>>8), byte(cc[i+1]>>16), byte(cc[i+2]))
	}
	var cmb []byte
	for i, k := range pk {
		v := pv[i]
		cmb = append(cmb, byte(k), byte(k>>8), byte(k>>16), byte(k>>24), byte(k>>32), byte(k>>40), byte(v), byte(v>>8), byte(v>>16))
	}
	w.WriteString("/// Runes with a canonical decomposition, ascending, 7 bytes each: the rune (3 bytes), the count of runes of its full decomposition (NFD), and where they start in CANON_POOL (3 bytes). Hangul syllables are algorithmic.\nconst CANON: Bytes = " + lit(cr7) + "\n\n")
	w.WriteString("/// The runes of the decompositions of CANON, each a varint.\nconst CANON_POOL: Bytes = " + lit(cp) + "\n\n")
	w.WriteString("/// Runes whose full compatibility decomposition (NFKD) differs from their NFD, as CANON.\nconst COMPAT: Bytes = " + lit(kr7) + "\n\n")
	w.WriteString("/// The runes of the decompositions of COMPAT, each a varint.\nconst COMPAT_POOL: Bytes = " + lit(kp) + "\n\n")
	w.WriteString("/// Canonical combining classes as 7 byte records, ascending: the first rune (3 bytes), the last rune (3 bytes), the class.\nconst CCC: Bytes = " + lit(ccb) + "\n\n")
	w.WriteString("/// Primary composition pairs, ascending by first * 2**21 + second, 9 bytes each: that key (6 bytes) and the rune it composes to (3 bytes).\nconst COMP: Bytes = " + lit(cmb) + "\n\n")
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
