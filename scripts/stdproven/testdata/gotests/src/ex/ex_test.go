package ex

type pair struct {
	in, out string
}

var keyed = []pair{
	{in: "a", out: "1"},
	{out: "2", in: "b" + "c"},
	{in: someVar, out: "3"},
	{in: "\xff", out: "4"},
}

var unkeyed = []*pair{
	{"x", "y"},
	&pair{"z", "w"},
}

var nums = []struct {
	year, month int
	z           *int
	f           float64
}{
	{2011, -3, Local, 1e9},
	{0x10, 1_000, UTC, 2},
	{1, int(January - 2), Local, 0},
	{1, 2, Other, 0},
	{1, 2, Local, 1.5},
}

func TestLocal(t *testing.T) {
	tests := []struct{ in string }{{"p"}, {"q"}}
	_ = tests
}

func TestRanges(t *testing.T) {
	for _, n := range []int{1} {
		_ = n
	}
	for _, test := range []struct{ in string }{{"r"}, {"s"}} {
		_ = test
	}
	for range []struct{ in string }{{"u"}} {
	}
}

var flags = []struct {
	pad   byte
	on    bool
	flags uint
}{
	{'.', true, Debug},
	{'\t', false, AlignRight | Debug},
	{'\'', true, 0},
	{'x', true, local},
	{'y', true, Debug &^ 1},
}

var slices = []struct {
	in []rune
}{
	{[]rune{'a', 0x10000, -1}},
	{[]rune{}},
	{[]rune{n}},
	{nil},
}

var opt = []pair{
	{in: "a"},
	{out: "2"},
	{in: someVar},
	{in: nil, out: "3"},
}

var vals = []int64{
	-1 << 63,
	-1<<63 + 1,
	1<<63 - 1,
	(2 + 3) * 4,
	'a' + 1,
	x,
	7 / 2,
	^0,
}

var sums = []struct{ n int }{
	{1 + 2},
	{"a" + "b"},
}

var calls = []struct{ in string }{
	{strings.Repeat("ab", 3)},
	{string([]byte("x" + "y"))},
	{strings.Repeat("x", n)},
	{strings.Repeat("x", 1<<30)},
	{strings.ToUpper("x")},
	{mustParse("p")},
	{netip.MustParseAddr("q")},
	{mustTwo("a", "b")},
}

func mustParse(s string) string { return s }

func mustTwo(a, b string) string { return a + b }

var files = []string{"a", "missing"}

type q struct{ to, want string }

// nested: a :json field, a slice of strings or of string structs
var nested = []struct {
	set []string
	qs  []q
}{
	{[]string{"A=a", "B=b"}, []q{{"http://h", "A=a"}, {to: "ftp://h"}}},
	{nil, []q{}},
	{[]string{name}, nil},
}

const name = "n"

type config struct {
	A, B string
	On   bool
}

// cfgs: a :json field holding a named struct literal, every field keyed
var cfgs = []struct {
	cfg config
}{
	{cfg: config{A: "x", On: true}},
	{cfg: config{B: "y"}},
	{cfg: config{"x", "y", false}},
	{cfg: config{A: someVar}},
}

func TestLocalVar(t *testing.T) {
	var tests = []struct{ in string }{
		{"v"},
		{"w"},
	}
	_ = tests
}
