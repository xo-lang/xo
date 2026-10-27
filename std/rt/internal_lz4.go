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

// uuidText is std/uuid's canonical text
// of 16 bytes (google/uuid's encodeHex), "" when b is not 16 bytes.
func uuidText(b Bytes) string {
	if len(b) != 16 {
		return ""
	}
	const hexd = "0123456789abcdef"
	var out [36]byte
	o := 0
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[o] = '-'
			o++
		}
		out[o] = hexd[b[i]>>4]
		out[o+1] = hexd[b[i]&15]
		o += 2
	}
	return string(out[:])
}

func unhexAny(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// uuidDecode is the 16
// bytes of a UUID's text from off, None when it is malformed or short.
func uuidDecode(s Bytes, off int64, dashed bool) Option[Bytes] {
	n := int64(32)
	if dashed {
		n = 36
	}
	if off < 0 || int64(len(s))-off < n {
		return None[Bytes]()
	}
	t := s[off:]
	if dashed && (t[8] != '-' || t[13] != '-' || t[18] != '-' || t[23] != '-') {
		return None[Bytes]()
	}
	var out [16]byte
	k := 0
	for i := 0; i < 16; i++ {
		if dashed && (i == 4 || i == 6 || i == 8 || i == 10) {
			k++
		}
		hi, lo := unhexAny(t[k]), unhexAny(t[k+1])
		if hi < 0 || lo < 0 {
			return None[Bytes]()
		}
		out[i] = byte(hi<<4 | lo)
		k += 2
	}
	return Some(Bytes(out[:]))
}

// Internal_list_swap_at is `internal.list_swap_at(xs, i, j)`, the native
// backends' codegen probe (test/codegen/intrinsics): the swap when i is in
// range; an index out of range faults as a list index does. The Go backend
// has it since std/uuid imports std/internal, which puts every internal
// function in the Go-compiled set (scripts/stdgen's std_sigs).
func Internal_list_swap_at(_ *Task, xs List[int64], i, j int64) {
	if i < int64(len(xs.s)) {
		xs.s[i], xs.s[j] = xs.s[j], xs.s[i]
	}
}

// Internal_list_swap_both is `internal.list_swap_both(xs, ys, zs)`: the
// swap at ys's and zs's lengths when both are in range.
func Internal_list_swap_both(_ *Task, xs, ys, zs List[int64]) {
	i, j := len(ys.s), len(zs.s)
	if i < len(xs.s) && j < len(xs.s) {
		xs.s[i], xs.s[j] = xs.s[j], xs.s[i]
	}
}

// Internal_uuid_half is `internal.uuid_half(s, off, dashed, half)`: the
// first (half 0) or last (half 1) 8 bytes of a UUID's text as a word, the
// first byte highest; None as uuidDecode.
func Internal_uuid_half(t *Task, s Bytes, off int64, dashed bool, half int64) Option[int64] {
	b := uuidDecode(s, off, dashed)
	if !b.Some {
		return None[int64]()
	}
	var w uint64
	for i := half * 8; i < half*8+8; i++ {
		w = w<<8 | uint64(b.V[i])
	}
	return Some(int64(w))
}

// Internal_uuid_text_words is `internal.uuid_text_words(hi, lo)`: the
// canonical text of the UUID whose bytes are hi then lo, big endian.
func Internal_uuid_text_words(t *Task, hi, lo int64) string {
	return uuidText(Internal_uuid_bytes_words(t, hi, lo))
}

// Internal_uuid_word is `internal.uuid_word(b, i)`: b[i:i+8] big endian.
func Internal_uuid_word(_ *Task, b Bytes, i int64) int64 {
	return int64(binary.BigEndian.Uint64([]byte(b[i : i+8])))
}

// Internal_uuid_bytes_words is `internal.uuid_bytes_words(hi, lo)`.
func Internal_uuid_bytes_words(_ *Task, hi, lo int64) Bytes {
	var b [16]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(uint64(hi) >> (56 - 8*i))
		b[8+i] = byte(uint64(lo) >> (56 - 8*i))
	}
	return Bytes(b[:])
}
