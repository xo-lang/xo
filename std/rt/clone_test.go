package rt

import (
	"strings"
	"testing"
	"unsafe"
)

// TestCloneOwnBuffer: Str.clone and Bytes.clone are "an explicit copy in a
// buffer of its own" (spec/std.md), so a clone of a slice of a large value
// does not keep the large value alive. Both returned their argument on the
// Go backend before task #429.
func TestCloneOwnBuffer(t *testing.T) {
	big := strings.Repeat("abcdefgh", 1<<12)
	piece := big[100:132]
	if c := Str_clone(nil, piece); unsafe.StringData(c) == unsafe.StringData(piece) || c != piece {
		t.Errorf("Str_clone shares its argument's buffer (or changed it): %q", c)
	}
	b := Bytes(piece)
	if c := Bytes_clone(nil, b); unsafe.StringData(string(c)) == unsafe.StringData(string(b)) || c != b {
		t.Errorf("Bytes_clone shares its argument's buffer (or changed it): %q", c)
	}
	if Str_clone(nil, "") != "" || Bytes_clone(nil, "") != "" {
		t.Error("clone of an empty value is not empty")
	}
}
