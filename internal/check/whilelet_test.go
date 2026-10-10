package check

import "testing"

// TestWhileLet covers `while let` (core 3.5, decision 0134).
func TestWhileLet(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "some", src: "fn f(var xs: List[Int]) -> Int {\n  var t = 0\n  while let Some(x) = xs.pop() {\n    t += x\n  }\n  t\n}\n"},
		{name: "shorthand", src: "fn f(o: Int?) -> Int {\n  var c = o\n  var n = 0\n  while let v = c {\n    n += v\n    c = None\n  }\n  n\n}\n"},
		{name: "refutable variant", src: "enum E {\n  A(n: Int)\n  B\n}\n\nfn f(es: List[E]) -> Int {\n  var i = 0\n  while let E.A(_) = es[i] {\n    i += 1\n  }\n  i\n}\n"},
		{name: "labeled with bound", src: "fn f(var xs: List[Int]) {\n  outer: while let Some(_) = xs.pop() bound 10 {\n    break outer\n  }\n}\n"},
		{name: "binding scoped to the body", src: "fn f(var xs: List[Int]) -> Int {\n  while let Some(x) = xs.pop() {\n  }\n  x\n}\n", want: []string{"XO0110", "XO0601"}},
		{name: "unused binding", src: "fn f(var xs: List[Int]) {\n  while let Some(x) = xs.pop() {\n  }\n}\n", want: []string{"XO0110"}},
		{name: "pattern of the wrong type", src: "fn f(xs: List[Int]) {\n  while let Ok(_) = xs.first() {\n  }\n}\n", want: []string{"XO0302"}},
		{name: "real-time needs a bound", src: "fn f(o: Int?) uses realtime {\n  var c = o\n  while let v = c {\n    c = if v > 0 { Some(v - 1) } else { None }\n  }\n}\n", want: []string{"XO1402"}},
		{name: "real-time with a bound", src: "fn f(o: Int?) uses realtime {\n  var c = o\n  while let v = c bound 4 {\n    c = if v > 0 { Some(v - 1) } else { None }\n  }\n}\n"},
		{name: "not a loop that never ends", src: "fn f(var xs: List[Int]) -> Int {\n  while let Some(_) = xs.pop() {\n  }\n}\n", want: []string{"XO0401"}},
	})
}
