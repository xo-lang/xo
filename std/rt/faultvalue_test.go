package rt

import (
	"fmt"
	"testing"
)

// The raw panic values of the checked operations render as Xo's fault
// text when something outside Xo's runtime prints them (net/http's per
// connection recover logs the value with %v; it showed
// `&{7 3 105 main.xo:20:13}`, 2026-10-30).
func TestFaultValueText(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{&arithFault{7, 3, 'i', "main.xo:20:13"}, "index 7 out of range for List of length 3 at main.xo:20:13"},
		{&arithFault{1, 0, '/', "main.xo:3:4"}, "division by zero: 1 / 0 at main.xo:3:4"},
		{&sizedFault{127, 1, '+', 1, "main.xo:5:6"}, "integer overflow: 127 + 1 does not fit Int8 at main.xo:5:6"},
	}
	for _, c := range cases {
		if got := fmt.Sprintf("%v", c.v); got != c.want {
			t.Errorf("%%v of %#v: %q, want %q", c.v, got, c.want)
		}
		if got := fmt.Sprint(c.v); got != c.want {
			t.Errorf("Sprint of %#v: %q, want %q", c.v, got, c.want)
		}
		// the recovered fault keeps the same text
		if f := AsFault(c.v); f.Error() != c.want {
			t.Errorf("AsFault(%#v).Error(): %q, want %q", c.v, f.Error(), c.want)
		}
	}
}
