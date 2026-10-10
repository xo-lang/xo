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

// Decision 0112: values that hold handles (or function values) are not
// comparable: `==`, Map keys, Set elements, and derive Eq/Hash/Ord are
// XO0404.
func TestHandlesNotComparable(t *testing.T) {
	const head = "struct Counter {\n  id: Int\n  n: Atomic[Int]\n}\nenum Slot {\n  Empty\n  Busy(lock: Mutex[Int])\n}\n"
	runCases(t, []ruleCase{
		{name: "== on a struct with a handle field", src: head + "fn f(a: Counter, b: Counter) -> Bool { a == b }\n", want: []string{"XO0404"}},
		{name: "!= on an enum with a handle payload", src: head + "fn f(a: Slot, b: Slot) -> Bool { a != b }\n", want: []string{"XO0404"}},
		{name: "== on a tuple with a channel", src: "fn f(a: (Int, Chan[Int]), b: (Int, Chan[Int])) -> Bool { a == b }\n", want: []string{"XO0404"}},
		{name: "== on a list of handles", src: "fn f(a: List[Mutex[Int]], b: List[Mutex[Int]]) -> Bool { a == b }\n", want: []string{"XO0404"}},
		{name: "== on a struct with a function field", src: "struct H { f: fn(Int) -> Int }\nfn f(a: H, b: H) -> Bool { a == b }\n", want: []string{"XO0404"}},
		{name: "comparing the identifying field is fine", src: head + "fn f(a: Counter, b: Counter) -> Bool { a.id == b.id }\n"},
		{name: "Map key annotation", src: head + "fn f() -> Int {\n  let m: Map[Counter, Int] = {}\n  m.len()\n}\n", want: []string{"XO0404"}},
		{name: "Set element in a field", src: "struct S { conns: Set[Conn] }\n", want: []string{"XO0404"}},
		{name: "Map key inferred from a literal", src: head + "fn f(c: Counter) -> Int {\n  let m = {c: 1}\n  m.len()\n}\n", want: []string{"XO0404"}},
		{name: "Set from to_set", src: "fn f(xs: List[Chan[Int]]) -> Int {\n  xs.to_set().len()\n}\n", want: []string{"XO0404"}},
		{name: "handle as a Map value is fine", src: head + "fn f(c: Counter) -> Int {\n  let m = {1: c}\n  m.len()\n}\n"},
		{name: "derive Eq", src: "struct S {\n  m: Mutex[Int]\n} derive Eq\n", want: []string{"XO0404"}},
		{name: "derive Hash and Ord on an enum", src: "enum E {\n  A(ch: Chan[Int])\n  B\n} derive Hash, Ord\n", want: []string{"XO0404", "XO0404"}},
		{name: "derive Ord on a generic struct is fine", src: "struct Box2[T] {\n  v: T\n} derive Ord\n"},
		{name: "Eq constraint", src: "fn same[T: Eq](a: T, b: T) -> Bool { a == b }\nfn f(a: Mutex[Int]) -> Bool { same(a, a) }\n", want: []string{"XO0404"}},
		{name: "derive Display is fine", src: "struct S {\n  m: Mutex[Int]\n} derive Display\n"},
	})
}
