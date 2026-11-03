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

func zigzag(d int) int {
	if d < 0 {
		return -2*d - 1
	}
	return 2 * d
}

// decomps packs the sorted (key, runes) as 7 byte records: the first rune (3
// bytes), a byte (bit 7 set: a run of consecutive runes whose decomposition
// is one rune each at the same delta from the rune, the low bits the length
// minus one; else the low bits the count of runes of the decomposition) and
// 3 bytes of data (the zigzag delta of a run, else the position of the varint
// coded runes in the pool).
func decomps(keys, offs, runes []int) (recs, pool []byte) {
	for i := 0; i < len(keys); {
		k := keys[i]
		n := offs[i+1] - offs[i]
		if n == 1 {
			d := runes[offs[i]] - k
			j := i
			for j+1 < len(keys) && j+1-i < 127 && keys[j+1] == keys[j]+1 && offs[j+2]-offs[j+1] == 1 && runes[offs[j+1]]-keys[j+1] == d {
				j++
			}
			if j > i {
				z := zigzag(d)
				recs = append(recs, byte(k), byte(k>>8), byte(k>>16), byte(0x80|(j-i)), byte(z), byte(z>>8), byte(z>>16))
				i = j + 1
				continue
			}
		}
		pos := len(pool)
		for _, r := range runes[offs[i]:offs[i+1]] {
			pool = varint(pool, r)
		}
		recs = append(recs, byte(k), byte(k>>8), byte(k>>16), byte(n), byte(pos), byte(pos>>8), byte(pos>>16))
		i++
	}
	return
}

func main() {
	path := os.Args[1]
	var ck, co, cr, kk, ko, kr, cc, pk, pv []int
	var uc, uk []int
	add := func(u []int, r int) []int {
		if n := len(u); n >= 2 && u[n-1] == r-1 {
			u[n-1] = r
			return u
		}
		return append(u, r, r)
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF || (r >= 0xAC00 && r <= 0xD7A3) {
			continue
		}
		s := string(r)
		if r >= 0xC0 {
			if norm.NFC.String(s) != s || !norm.NFC.PropertiesString(s).BoundaryBefore() {
				uc = add(uc, int(r))
			}
			if norm.NFKC.String(s) != s || !norm.NFKC.PropertiesString(s).BoundaryBefore() {
				uk = add(uk, int(r))
			}
		}
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
	w.WriteString("/// Runes with a canonical decomposition, ascending, 7 bytes each (the layout of `decomps` in scripts/gennorm): a rune or a run of runes with their full decompositions (NFD), the runes of the others in CANON_POOL. Hangul syllables are algorithmic.\nconst CANON: Bytes = " + lit(cr7) + "\n\n")
	w.WriteString("/// The runes of the decompositions of CANON, each a varint.\nconst CANON_POOL: Bytes = " + lit(cp) + "\n\n")
	w.WriteString("/// Runes whose full compatibility decomposition (NFKD) differs from their NFD, as CANON.\nconst COMPAT: Bytes = " + lit(kr7) + "\n\n")
	w.WriteString("/// The runes of the decompositions of COMPAT, each a varint.\nconst COMPAT_POOL: Bytes = " + lit(kp) + "\n\n")
	idx16 := func(v int) []byte { return []byte{byte(v), byte(v >> 8)} }
	rd3 := func(b []byte, p int) int { return int(b[p]) | int(b[p+1])<<8 | int(b[p+2])<<16 }
	// blockIdx[b]: the last record with its first rune at or before b*64 (0 if none), for b up to 0x3000/64
	blockIdx := func(recs []byte) []byte {
		var out []byte
		n := len(recs) / 7
		for b := 0; b <= 0x3000/64; b++ {
			last := 0
			for i := 0; i < n; i++ {
				if rd3(recs, 7*i) <= b*64 {
					last = i
				}
			}
			out = append(out, idx16(last)...)
		}
		return out
	}
	// compIdx[a]: the first composition pair whose first rune is at least a, for a up to 0x400
	var compIdx []byte
	for a := 0; a <= 0x400; a++ {
		first := len(pk)
		for i, k := range pk {
			if k>>21 >= a {
				first = i
				break
			}
		}
		compIdx = append(compIdx, idx16(first)...)
	}
	// ranges of runes from U+C0 whose NFC (NFKC) form is not themselves or that combine backward, as 4 byte records: first rune (3 bytes) and the length minus one
	unst := func(u []int) []byte {
		var out []byte
		for i := 0; i < len(u); i += 2 {
			for a := u[i]; a <= u[i+1]; {
				n := u[i+1] - a
				if n > 255 {
					n = 255
				}
				out = append(out, byte(a), byte(a>>8), byte(a>>16), byte(n))
				a += n + 1
			}
		}
		return out
	}
	w.WriteString("/// Runes from U+C0 on that NFC can change or reorder, or that combine with a rune before them, as 4 byte records ascending: the first rune (3 bytes) and the length minus one. Text of all other runes is its own NFC.\nconst NFC_UNSTABLE: Bytes = " + lit(unst(uc)) + "\n\n")
	w.WriteString("/// The same for NFKC.\nconst NFKC_UNSTABLE: Bytes = " + lit(unst(uk)) + "\n\n")
	w.WriteString("/// Per block of 64 runes up to U+3000 (and one more), 2 bytes: the last record of CANON that starts at or before the block's first rune, to bound the search.\nconst CANON_IDX: Bytes = " + lit(blockIdx(cr7)) + "\n\n")
	w.WriteString("/// The same for COMPAT.\nconst COMPAT_IDX: Bytes = " + lit(blockIdx(kr7)) + "\n\n")
	w.WriteString("/// Per first rune up to U+400 (and one more), 2 bytes: the first record of COMP whose first rune is at least that.\nconst COMP_IDX: Bytes = " + lit(compIdx) + "\n\n")
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
