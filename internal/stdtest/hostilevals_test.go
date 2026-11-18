package stdtest

// The hostile values every std fuzz harness puts in a share of its cases
// (task #352; ~/xo-lead class guard 2, C3): the edges a random generator
// rarely reaches and that faulted std before. Int overflow: html's long
// numeric reference (f447c3f71), websocket's 64-bit length with its top
// bit set (80d586b09), png's dimensions, time's far fields,
// parse_duration, rate, pg's unsigned 16-bit counts (54e535150). A Str
// cut inside UTF-8: json, uuid, yaml, surrogates, term. Depth: asn1
// nesting, archive. A harness takes values from here instead of its own
// list, so the edges are a fixed share of every run; repotest
// TestFuzzHarnessUsesHostile checks each one calls the helper.

import (
	"math"
	"math/rand"
	"strings"
	"unicode/utf8"
)

// HostileInts are the integers at the edges of Int and of the field
// widths a wire format uses (8, 16, 32, 53, 63 and 64 bits), each with
// its neighbours.
func HostileInts() []int64 {
	var out []int64
	add := func(v int64) { out = append(out, v-1, v, v+1) }
	out = append(out, 0, 1, -1, math.MaxInt64, math.MaxInt64-1, math.MinInt64, math.MinInt64+1)
	for _, b := range []uint{7, 8, 15, 16, 31, 32, 53, 62} {
		add(1 << b)
		add(-(1 << b))
	}
	return out
}

// HostileUints are HostileInts' unsigned edges, past Int included: the
// top bit set (a length or count a peer sends as uint64) and all ones.
func HostileUints() []uint64 {
	out := []uint64{0, 1, math.MaxUint64, math.MaxUint64 - 1, 1 << 63, 1<<63 - 1, 1<<63 + 1, 1<<63 | 5}
	for _, b := range []uint{8, 16, 32} {
		out = append(out, 1<<b-1, 1<<b, 1<<(b-1), 1<<(b-1)|5)
	}
	return out
}

// HostileLengths are the values of a width-bit unsigned length or count
// field (8, 16, 32 or 64) that a decoder must refuse or bound without
// overflowing: all ones, the top bit alone, the top bit with a small
// value, one under the top bit, and zero.
func HostileLengths(width uint) []uint64 {
	if width == 0 || width > 64 {
		panic("HostileLengths: width is 1 to 64")
	}
	top := uint64(1) << (width - 1)
	ones := top | (top - 1)
	return []uint64{ones, top, top | 5, top - 1, ones - 1, 0}
}

// HostileDigits are decimal runs at and past Int's range: 19, 20 and 40
// digits, Int's bounds and one past them, signs, and leading zeros that
// keep a small value long.
func HostileDigits() []string {
	return []string{
		strings.Repeat("9", 19), strings.Repeat("9", 20), strings.Repeat("9", 40),
		"1" + strings.Repeat("0", 19), "1" + strings.Repeat("0", 40),
		"9223372036854775807", "9223372036854775808", "-9223372036854775808",
		"-9223372036854775809", "18446744073709551615", "18446744073709551616",
		"-" + strings.Repeat("9", 20), "+" + strings.Repeat("9", 20),
		strings.Repeat("0", 40) + "1", strings.Repeat("0", 400) + "65",
	}
}

// HostileHex are hexadecimal runs at and past 64 bits: 16, 17 and 32
// digits, the top bit set, and leading zeros.
func HostileHex() []string {
	return []string{
		strings.Repeat("F", 16), strings.Repeat("f", 17), strings.Repeat("F", 32),
		"7FFFFFFFFFFFFFFF", "8000000000000000", "10000000000000000",
		strings.Repeat("0", 40) + "41", "1" + strings.Repeat("0", 16),
	}
}

// HostileUTF8 are byte sequences that are not UTF-8 in the ways decoders
// get wrong: a lone lead byte, a continuation byte alone, a sequence cut
// short, overlong forms, a UTF-16 surrogate encoded (CESU-8) alone and as
// a pair, past U+10FFFF, and the bytes C0, C1, F5 and FF.
func HostileUTF8() []string {
	return []string{
		"\xc3", "\x80", "\xbf", "\xe6\x97", "\xf0\x9f\x98",
		"\xc0\x80", "\xc0\xaf", "\xe0\x80\xaf", "\xf0\x80\x80\xaf",
		"\xed\xa0\x80", "\xed\xbf\xbf", "\xed\xa0\xbd\xed\xb8\x80",
		"\xf4\x90\x80\x80", "\xf8\x88\x80\x80\x80",
		"\xc1", "\xf5", "\xfe", "\xff",
	}
}

// CutUTF8 is s cut at every byte inside a multibyte sequence: each prefix
// that ends part way through a rune, and each suffix that starts part way
// through one. Every result is not UTF-8.
func CutUTF8(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		_, n := utf8.DecodeRuneInString(s[i:])
		for k := 1; k < n; k++ {
			out = append(out, s[:i+k], s[i+k:])
		}
		i += n
	}
	return out
}

// HostileDepths are the nesting depths a recursive decoder must refuse or
// handle without exhausting its stack.
func HostileDepths() []int { return []int{1000, 10000, 100000} }

// Nest is open n times, then close n times: "[[[...]]]".
func Nest(open, close string, n int) string {
	return strings.Repeat(open, n) + strings.Repeat(close, n)
}

// HostilePick is one of xs, for a generator to put in a share of its
// cases.
func HostilePick[T any](r *rand.Rand, xs []T) T { return xs[r.Intn(len(xs))] }
