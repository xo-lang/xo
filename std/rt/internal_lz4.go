package rt

import "encoding/binary"

// LZ4 block format (decision 0365) for `internal.lz4_compress` and
// `internal.lz4_decompress`: the same greedy matcher as the native
// std/native/llrt/lz4.xo (github.com/pierrec/lz4/v4's CompressBlock and
// lz4.c's LZ4_compress_fast at acceleration 1, BSD 3-Clause), so both
// backends write the same bytes; the differential tests
// (internal/lz4test) check them against each other and the lz4 CLI.

const (
	lz4MinMatch     = 4
	lz4LastLiterals = 5
	lz4MFLimit      = 12
	lz4MaxOffset    = 65535
)

func lz4Hash(v uint32, hlog uint) uint32 { return (v * 2654435761) >> (32 - hlog) }

func lz4PutLen(d []byte, n int) []byte {
	for n >= 255 {
		d = append(d, 255)
		n -= 255
	}
	return append(d, byte(n))
}

func lz4PutSeq(d, s []byte, anchor, litEnd, off, ml int) []byte {
	ll := litEnd - anchor
	tokLit := min(ll, 15)
	mx := 0
	if ml > 0 {
		mx = ml - lz4MinMatch
	}
	d = append(d, byte(tokLit<<4|min(mx, 15)))
	if ll >= 15 {
		d = lz4PutLen(d, ll-15)
	}
	d = append(d, s[anchor:litEnd]...)
	if ml > 0 {
		d = append(d, byte(off), byte(off>>8))
		if mx >= 15 {
			d = lz4PutLen(d, mx-15)
		}
	}
	return d
}

// LZ4CompressBlock is src as one LZ4 block.
func LZ4CompressBlock(s []byte) []byte {
	n := len(s)
	d := make([]byte, 0, n+n/255+16)
	anchor := 0
	if n >= lz4MFLimit+1 {
		hlog := uint(10)
		for hlog < 16 && 1<<hlog < n {
			hlog++
		}
		tab := make([]int32, 1<<hlog)
		mflimit := n - lz4MFLimit
		matchlimit := n - lz4LastLiterals
		u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(s[i:]) }
		tab[lz4Hash(u32(0), hlog)] = 1
		ip := 1
	outer:
		for {
			fwd := ip
			attempts := 64
			ref := 0
			for {
				ip = fwd
				step := attempts >> 6
				attempts++
				fwd = ip + step
				if fwd > mflimit {
					break outer
				}
				v := u32(ip)
				h := lz4Hash(v, hlog)
				r := int(tab[h]) - 1
				tab[h] = int32(ip + 1)
				if r >= 0 && ip-r <= lz4MaxOffset && u32(r) == v {
					ref = r
					break
				}
			}
			for ip > anchor && ref > 0 && s[ip-1] == s[ref-1] {
				ip--
				ref--
			}
			i, j := ip+lz4MinMatch, ref+lz4MinMatch
			for i < matchlimit && s[i] == s[j] {
				i++
				j++
			}
			ml := i - ip
			d = lz4PutSeq(d, s, anchor, ip, ip-ref, ml)
			ip += ml
			anchor = ip
			if ip > mflimit {
				break
			}
			tab[lz4Hash(u32(ip-2), hlog)] = int32(ip - 1)
		}
	}
	return lz4PutSeq(d, s, anchor, n, 0, 0)
}

// LZ4DecompressBlock is the n bytes of the LZ4 block s; false when s is
// not a block of exactly n bytes.
func LZ4DecompressBlock(s []byte, n int) ([]byte, bool) {
	if n < 0 {
		return nil, false
	}
	d := make([]byte, 0, n)
	ip := 0
	readLen := func(l int) (int, bool) {
		for {
			if ip >= len(s) {
				return 0, false
			}
			b := int(s[ip])
			ip++
			l += b
			if b != 255 {
				return l, true
			}
		}
	}
	for {
		if ip >= len(s) {
			return nil, false
		}
		tok := int(s[ip])
		ip++
		ll := tok >> 4
		if ll == 15 {
			var ok bool
			if ll, ok = readLen(ll); !ok {
				return nil, false
			}
		}
		if ll > len(s)-ip || ll > n-len(d) {
			return nil, false
		}
		d = append(d, s[ip:ip+ll]...)
		ip += ll
		if ip == len(s) {
			break
		}
		if ip+2 > len(s) {
			return nil, false
		}
		off := int(s[ip]) | int(s[ip+1])<<8
		ip += 2
		if off == 0 || off > len(d) {
			return nil, false
		}
		ml := tok & 15
		if ml == 15 {
			var ok bool
			if ml, ok = readLen(ml); !ok {
				return nil, false
			}
		}
		ml += lz4MinMatch
		if ml > n-len(d) {
			return nil, false
		}
		from := len(d) - off
		for k := 0; k < ml; k++ {
			d = append(d, d[from+k])
		}
	}
	if len(d) != n {
		return nil, false
	}
	return d, true
}

// Internal_lz4_compress is `internal.lz4_compress(src)`.
func Internal_lz4_compress(_ *Task, src Bytes) Bytes {
	return Bytes(LZ4CompressBlock([]byte(src)))
}

// Internal_lz4_decompress is `internal.lz4_decompress(src, start, n)`.
func Internal_lz4_decompress(_ *Task, src Bytes, start, n int64) Option[Bytes] {
	if n < 0 || n > 1<<40 || start < 0 || start > int64(len(src)) {
		return None[Bytes]()
	}
	d, ok := LZ4DecompressBlock([]byte(src[start:]), int(n))
	if !ok {
		return None[Bytes]()
	}
	return Some(Bytes(d))
}
