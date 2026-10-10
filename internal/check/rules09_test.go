package check

import "testing"

// Decision 0111: one variable or overlapping places passed to two `var`
// parameters of one call is XO0503.
func TestVarArgsOverlap(t *testing.T) {
	const head = "struct P { x: List[Int], y: List[Int] }\nfn both(var a: List[Int], var b: List[Int]) {\n  a.push(1)\n  b.push(2)\n}\nfn one(var a: P, var b: List[Int]) {\n  b.push(1)\n}\nfn (var p: P) add(var b: List[Int]) {\n  b.push(1)\n}\n"
	runCases(t, []ruleCase{
		{name: "same binding", src: head + "fn f() -> Int {\n  var xs = [0]\n  both(xs, xs)\n  xs.len()\n}\n", want: []string{"XO0503"}},
		{name: "named arguments", src: head + "fn f() -> Int {\n  var xs = [0]\n  both(b: xs, a: xs)\n  xs.len()\n}\n", want: []string{"XO0503"}},
		{name: "struct and its field", src: head + "fn f() -> Int {\n  var p = P{x: [], y: []}\n  one(p, p.x)\n  p.x.len()\n}\n", want: []string{"XO0503"}},
		{name: "receiver and its field", src: head + "fn f() -> Int {\n  var p = P{x: [], y: []}\n  p.add(p.y)\n  p.y.len()\n}\n", want: []string{"XO0503"}},
		{name: "two fields are distinct", src: head + "fn f() -> Int {\n  var p = P{x: [], y: []}\n  both(p.x, p.y)\n  p.x.len()\n}\n"},
		{name: "unknown indexes may overlap", src: head + "fn f(i: Int, j: Int) -> Int {\n  var g = [[0], [1]]\n  both(g[i], g[j])\n  g.len()\n}\n", want: []string{"XO0503"}},
		{name: "same constant index", src: head + "fn f() -> Int {\n  var g = [[0], [1]]\n  both(g[0], g[0])\n  g.len()\n}\n", want: []string{"XO0503"}},
		{name: "different constant indexes are distinct", src: head + "fn f() -> Int {\n  var g = [[0], [1]]\n  both(g[0], g[1])\n  g.len()\n}\n"},
		{name: "two bindings", src: head + "fn f() -> Int {\n  var xs = [0]\n  var ys = xs\n  both(xs, ys)\n  xs.len() + ys.len()\n}\n"},
		{name: "by value argument of the same binding", src: "fn mixed(var a: List[Int], b: List[Int]) -> Int {\n  a.push(1)\n  b.len()\n}\nfn f() -> Int {\n  var xs = [0]\n  mixed(xs, xs)\n}\n"},
	})
}
