package rt

import (
	"os"
	"strings"
	"testing"
	"unsafe"
)

// TestGoBytesView: the view is b's own bytes, with b's length as its
// length and capacity, and an empty Bytes gives an empty slice.
func TestGoBytesView(t *testing.T) {
	b := Bytes("the quick brown fox")
	v := GoBytesView(b)
	if len(v) != len(b) || cap(v) != len(b) || string(v) != string(b) {
		t.Fatalf("view %q len %d cap %d of %q", v, len(v), cap(v), b)
	}
	if unsafe.SliceData(v) != unsafe.StringData(string(b)) {
		t.Fatal("the view is a copy")
	}
	if e := GoBytesView(Bytes("")); len(e) != 0 || cap(e) != 0 {
		t.Fatalf("empty view: len %d cap %d", len(e), cap(e))
	}
	if s := GoBytesView(b[4:9]); string(s) != "quick" || cap(s) != 5 {
		t.Fatalf("view of a slice: %q cap %d", s, cap(s))
	}
}

// TestGoBytesViewNoChecks (task #491): GoBytesView does not go through
// unsafe.Slice, whose three tests ran at every bridged call (6
// instructions a call in bench/perf/gobridge's bytes loop). The gates
// have no instruction counter for Go code, so this reads the source.
func TestGoBytesViewNoChecks(t *testing.T) {
	src, err := os.ReadFile("gointerop.go")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(src), "\nfunc GoBytesView(b Bytes) []byte {\n")
	if !ok {
		t.Fatal("no GoBytesView in gointerop.go")
	}
	body, _, _ = strings.Cut(body, "\n}\n")
	if strings.Contains(body, "unsafe.Slice(") {
		t.Fatalf("GoBytesView calls unsafe.Slice:\n%s", body)
	}
}
