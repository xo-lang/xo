package gen_test

import (
	"regexp"
	"strings"
	"testing"
)

// funcBody returns the generated Go of function name, without //line
// directives.
func funcBody(t *testing.T, src, name string) string {
	t.Helper()
	i := strings.Index(src, "func "+name+"(")
	if i < 0 {
		t.Fatalf("no func %s in:\n%s", name, src)
	}
	j := strings.Index(src[i:], "\n}\n")
	var b strings.Builder
	for _, l := range strings.Split(src[i:i+j], "\n") {
		if !strings.HasPrefix(l, "//line ") {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// checked matches a checked Int operation.
const checked = `rt\.(Add|Sub|Mul|Div|Rem)`

// TestRangeElision checks which Int operations the range analysis
// (ranges.go) emits as plain Go and which stay checked. want are regular
// expressions the body of F_f must match, not those it must not.
func TestRangeElision(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
		not  []string
	}{
		{
			name: "constant operands",
			src:  `fn f(x: Int) -> Int { x % 7 + x / 7 + (x * 3) - 1 }`,
			want: []string{`\(v_x ?% ?int64\(7\)\)`, `\(v_x ?/ ?int64\(7\)\)`, `rt\.MulIK\(v_x, int64\(3\), -3074457345618258602, 3074457345618258602, `, `rt\.SubIDown\(`, `rt\.AddI\(`},
		},
		{
			name: "division by -1 and 0 stay checked",
			src:  `fn f(x: Int) -> Int { x / -1 + x % 0 }`,
			want: []string{`rt\.DivI\(v_x, int64\(-1\)`, `rt\.RemI\(v_x, int64\(0\)`},
		},
		{
			name: "remainder and mask results",
			src:  `fn f(x: Int) -> Int { (x % 1000) * 1000 + (x & 255) + 1 }`,
			want: []string{`\(\(v_x % int64\(1000\)\) \* int64\(1000\)\)`},
			not:  []string{checked},
		},
		{
			name: "loop induction variable bounded by a range",
			src: `fn f() -> Int {
  var s = 0
  for i in 0..10 {
    s += i * 922337203685477580
  }
  s
}`,
			want: []string{`\(v_i \* int64\(922337203685477580\)\)`, `rt\.AddI\(v_s,`},
		},
		{
			name: "near the bound: ..=10 fits, ..=11 does not",
			src: `fn f() -> Int {
  var s = 0
  for i in 0..=10 {
    s = i + 9223372036854775797
  }
  for j in 0..=11 {
    s = j + 9223372036854775797
  }
  s
}`,
			want: []string{`\(v_i \+ int64\(9223372036854775797\)\)`, `rt\.AddIUp\(v_j, int64\(9223372036854775797\)`},
		},
		{
			name: "comparison before use",
			src: `fn f(x: Int) -> Int {
  if x < 3037000499 && x > -3037000499 {
    return x * x
  }
  x * x
}`,
			want: []string{`return \(v_x \* v_x\)`, `rt\.MulI\(v_x, v_x`},
		},
		{
			name: "sized integers widen",
			src:  `fn f(a: Int32, b: Int32) -> Int { Int(a) * Int(b) + 1 }`,
			not:  []string{checked},
		},
		{
			name: "index proven in range by a loop bound",
			src: `fn f(xs: List[Int]) -> Int {
  var s = 0
  for i in 0..xs.len() {
    s += xs[i]
  }
  s
}`,
			want: []string{`v_xs\.AtU\(v_i\)`},
			not:  []string{`AtM`},
		},
		{
			name: "binary search",
			src: `fn f(xs: List[Int], x: Int) -> Int {
  var lo = 0
  var hi = xs.len()
  while lo < hi {
    let mid = (lo + hi) / 2
    if xs[mid] < x {
      lo = mid + 1
    } else {
      hi = mid
    }
  }
  lo
}`,
			want: []string{`\(\(v_lo \+ v_hi\) / int64\(2\)\)`, `v_xs\.AtU\(v_mid\)`, `\(v_mid \+ int64\(1\)\)`},
			not:  []string{checked, `AtM`},
		},
		{
			name: "index of a var list or past the length stays checked",
			src: `fn f(xs: List[Int]) -> Int {
  var ys = xs
  var s = 0
  for i in 0..ys.len() {
    s += ys[i]
  }
  for j in 0..=xs.len() {
    s += xs[j]
  }
  s
}`,
			want: []string{`v_ys\.AtM\(v_i,`, `v_xs\.AtM\(v_j,`},
			not:  []string{`AtU`},
		},
		{
			name: "a var assigned in a defer is not tracked",
			src: `fn f() -> Int {
  var n = 1
  {
    defer { n = 9223372036854775807 }
    n = 2
  }
  n + 1
}`,
			want: []string{`rt\.AddIUp\(v_n, int64\(1\)`},
		},
		{
			name: "a var passed as a var argument is not tracked",
			src: `fn set_max(var n: Int) { n = 9223372036854775807 }

fn f() -> Int {
  var n = 1
  set_max(n)
  n + 1
}`,
			want: []string{`rt\.AddIUp\(v_n, int64\(1\)`},
		},
		{
			name: "loop widening keeps growing counters checked",
			src: `fn f(n: Int) -> Int {
  var c = 0
  var i = 0
  while i < n {
    c = c + 2
    i += 1
  }
  c
}`,
			// c grows without bound; i < n keeps i + 1 in range
			want: []string{`rt\.AddIUp\(v_c, int64\(2\)`, `v_i = \(v_i \+ int64\(1\)\)`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "fn main() {}\n\n" + c.src + "\n"
			body := funcBody(t, goSource(generateMono(t, src)), "F_f")
			for _, w := range c.want {
				if !regexp.MustCompile(w).MatchString(body) {
					t.Errorf("want %s in:\n%s", w, body)
				}
			}
			for _, w := range c.not {
				if regexp.MustCompile(w).MatchString(body) {
					t.Errorf("do not want %s in:\n%s", w, body)
				}
			}
		})
	}
}
