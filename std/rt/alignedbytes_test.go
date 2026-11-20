package rt

import (
	"strings"
	"testing"
	"unsafe"
)

func TestAlignedBytes(t *testing.T) {
	for _, n := range []int{1, 31, 32, 33, 64, 100} {
		s := strings.Repeat("ab", n)[1 : n+1]
		b := AlignedBytes(s)
		if string(b) != s {
			t.Fatalf("n=%d: got %q, want %q", n, b, s)
		}
		if p := uintptr(unsafe.Pointer(unsafe.StringData(string(b)))); p%32 != 0 {
			t.Errorf("n=%d: data at %#x, not 32-byte aligned", n, p)
		}
	}
}
