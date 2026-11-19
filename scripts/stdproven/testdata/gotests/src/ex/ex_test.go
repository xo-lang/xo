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
}
