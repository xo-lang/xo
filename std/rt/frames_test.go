package rt

import (
	"strings"
	"sync"
	"testing"
)

// eagerFrameText is how frames showed a value before decision 0092: the
// full Debug text, then cut.
func eagerFrameText(v any) string {
	s := Debug(v)
	if len(s) > 64 {
		s = s[:61] + "..."
	}
	return s
}

func frameValues() []any {
	long := strings.Repeat("ab", 100)
	return []any{
		int64(42), true, 1.5, "", "ada",
		strings.Repeat("x", 62), strings.Repeat("x", 63), strings.Repeat("x", 64), strings.Repeat("x", 65),
		long,
		strings.Repeat("é", 40), strings.Repeat("日本", 30), strings.Repeat("\n\t\"\\", 30),
		"abc\xff\xfe\x80\x80\x80\x80\x80" + strings.Repeat("\x80", 70),
		strings.Repeat("a", 60) + "\xe2\x82" + strings.Repeat("b", 10),
		strings.Repeat("\x01", 30),
		Bytes(strings.Repeat("\x00\x01z", 40)),
		ListOf("a", "b"),
		ListOf(long, long),
		ListOf(ListOf(int64(1), int64(2)), ListOf[int64]()),
		ListOf(Secret[string]{v: "pw"}, Secret[string]{v: long}),
		Some(long), Option[int64]{},
		Tuple2[string, int64]{F0: long, F1: 7},
		StrBuilder{b: []byte(long)},
		BytesBuilder{b: []byte(long)},
		FrameText(long),
	}
}

// TestFrameDebugBounded checks that bounded formatting (decision 0092)
// shows exactly what full formatting cut to 64 bytes showed, for values
// at, below, and above the limit, multibyte and invalid UTF-8 cut at the
// limit, escapes, nesting, and secrets.
func TestFrameDebugBounded(t *testing.T) {
	m := mapCap[string, string](0)
	for i := 0; i < 20; i++ {
		m.M_put(nil, strings.Repeat("k", i), strings.Repeat("v", i))
	}
	vals := append(frameValues(), m)
	for i, v := range vals {
		if got, want := frameDebug(v), eagerFrameText(v); got != want {
			t.Errorf("value %d: bounded %q, eager %q", i, got, want)
		}
	}
}

// TestFrameDebugStopsEarly checks that a large value is not formatted in
// full: the bounded text of a 1 MB string and a 100,000 element list stays
// near the limit.
func TestFrameDebugStopsEarly(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	xs := make([]int64, 100000)
	for _, v := range []any{big, Bytes(big), ListOf(xs...), ListOf(big)} {
		if s := debugBounded(v, frameArgMax+1); len(s) > 2*frameArgMax {
			t.Errorf("bounded text of %T is %d bytes", v, len(s))
		}
	}
	if boundedActive.Load() != nil {
		t.Error("a bounded format is still registered")
	}
}

// TestFrameDebugConcurrent formats bounded and unbounded at once on many
// goroutines (run with -race): an unbounded Debug must never be cut.
func TestFrameDebugConcurrent(t *testing.T) {
	long := strings.Repeat("z", 500)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				if g%2 == 0 {
					if s := frameDebug(long); len(s) != 64 {
						t.Errorf("bounded: %d bytes", len(s))
						return
					}
				} else if s := Debug(long); len(s) != 502 {
					t.Errorf("unbounded Debug cut to %d bytes", len(s))
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestTraceLabel checks the frame text: values, no parameters, and
// parameters not recorded (`--error-args=false`).
func TestTraceLabel(t *testing.T) {
	tr := Frame2(nil, "users.fetch_user", "users/fetch.xo:18", "id name", int64(42), "ada")
	tr = Frame0(tr, "api.main", "api/main.xo:3", "")
	tr = Frame0(tr, "api.serve", "api/main.xo:9", "port")
	tr = FrameN(tr, "api.many", "api/main.xo:12", "a b c d e f g", int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), Secret[string]{v: "pw"})
	want := []string{
		`  at users.fetch_user(id=42, name="ada") users/fetch.xo:18`,
		`  at api.main()                         api/main.xo:3`,
		`  at api.serve(...)                     api/main.xo:9`,
		`  at api.many(a=1, b=2, c=3, d=4, e=5, f=6, g=<redacted>) api/main.xo:12`,
	}
	got := tr.Lines()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
