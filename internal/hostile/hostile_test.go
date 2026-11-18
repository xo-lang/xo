package hostile

import (
	"math"
	"math/big"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestValues: the dictionary holds the edges the C3 fixes needed
// (task #352).
func TestValues(t *testing.T) {
	is := Ints()
	for _, v := range []int64{0, -1, math.MaxInt64, math.MinInt64, 1 << 31, 1 << 32, 1 << 53, 1<<16 - 1} {
		if !slices.Contains(is, v) {
			t.Errorf("HostileInts lacks %d", v)
		}
	}
	us := Uints()
	for _, v := range []uint64{1 << 63, math.MaxUint64, 1<<63 | 5, 1<<16 - 1} {
		if !slices.Contains(us, v) {
			t.Errorf("HostileUints lacks %d", v)
		}
	}
	if got := Lengths(64); !slices.Contains(got, 1<<63|5) || !slices.Contains(got, math.MaxUint64) {
		t.Errorf("Lengths(64) = %v", got)
	}
	if got := Lengths(16); !slices.Contains(got, 0xffff) || !slices.Contains(got, 0x8000) {
		t.Errorf("Lengths(16) = %v", got)
	}
	// every digit run but the padded ones is past Int
	past := 0
	for _, d := range Digits() {
		n, ok := new(big.Int).SetString(strings.TrimPrefix(d, "+"), 10)
		if !ok {
			t.Errorf("HostileDigits %q is not decimal", d)
			continue
		}
		if !n.IsInt64() {
			past++
		}
	}
	if past < 8 {
		t.Errorf("only %d HostileDigits are past Int", past)
	}
	for _, h := range Hex() {
		if _, ok := new(big.Int).SetString(h, 16); !ok {
			t.Errorf("HostileHex %q is not hexadecimal", h)
		}
	}
	for _, s := range UTF8() {
		if utf8.ValidString(s) {
			t.Errorf("HostileUTF8 %q is UTF-8", s)
		}
	}
	cuts := CutUTF8("aé日😀")
	if len(cuts) != 2*(1+2+3) {
		t.Errorf("CutUTF8 gave %d cuts, want 12", len(cuts))
	}
	for _, s := range cuts {
		if utf8.ValidString(s) {
			t.Errorf("CutUTF8 gave %q, which is UTF-8", s)
		}
	}
	if len(CutUTF8("ascii")) != 0 {
		t.Error("CutUTF8 of ASCII is not empty")
	}
	if n := Nest("[", "]", 3); n != "[[[]]]" {
		t.Errorf("Nest = %q", n)
	}
	if slices.Max(Depths()) < 100000 {
		t.Error("HostileDepths stops short of 10^5")
	}
}
