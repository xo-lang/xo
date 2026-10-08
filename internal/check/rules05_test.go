package check

import "testing"

// Checker fixes from the stage 2 benchmark (spec draft 0.5 candidates).

func TestLostUpdateReadAfter(t *testing.T) {
	const head = "struct U { pub name: Str }\nenum E { Missing }\n"
	runCases(t, []ruleCase{
		{name: "copy out mutated then only read and returned", src: head + "fn f(var m: Map[Int, U]) -> U ! E {\n  var u = m.get(1).or_err(E.Missing)?\n  u.name = \"b\"\n  let n = u.name\n  u\n}\n", want: []string{"XO0520", "XO0110"}},
		{name: "copy out mutated then read in a condition", src: head + "fn f(var m: Map[Int, U]) -> Bool {\n  var u = m.get(1) ?? U{name: \"a\"}\n  u.name = \"b\"\n  u.name == \"b\"\n}\n", want: []string{"XO0520"}},
		{name: "copy out mutated then stored", src: head + "fn f(var m: Map[Int, U]) -> U ! E {\n  var u = m.get(1).or_err(E.Missing)?\n  u.name = \"b\"\n  m.put(1, u)\n  u\n}\n"},
		{name: "copy out mutated then assigned by index", src: head + "fn f(var m: Map[Int, U]) {\n  var u = m.get(1) ?? U{name: \"a\"}\n  u.name = \"b\"\n  m[1] = u\n}\n"},
		{name: "copy out mutated then passed to a function", src: head + "fn save(u: U) {}\nfn f(m: Map[Int, U]) {\n  var u = m.get(1) ?? U{name: \"a\"}\n  u.name = \"b\"\n  save(u)\n}\n"},
		{name: "scratch copy through a local is allowed", src: head + "fn f(m: Map[Int, U]) -> Str {\n  let base = m.get(1) ?? U{name: \"a\"}\n  var scratch = base\n  scratch.name = \"b\"\n  scratch.name\n}\n"},
	})
}

func TestTestingPredeclaredInTestFileSignatures(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "helper signature in a test file", files: map[string]string{
			"a.xo":      "pub fn double(x: Int) -> Int { x * 2 }\n",
			"a_test.xo": "fn seed(os: testing.TestOs) -> Int { 0 }\ntest \"t\" {\n  expect(seed(os)) == double(0)\n}\n",
		}},
		{name: "not predeclared outside test files", files: map[string]string{
			"a.xo": "fn seed(os: testing.TestOs) -> Int { 0 }\npub fn g() -> Int { 1 }\n",
		}, want: []string{"XO0601"}},
	})
}

func TestNoInferCascadeOnBadCallee(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "closure argument to a method on a bad receiver", src: "fn load(x: nosuch.T) -> nosuch.L { x }\npub fn f() -> Int {\n  let xs = load(1)\n  let ys = xs.map(fn(p) { p })\n  ys\n}\n", want: []string{"XO0601", "XO0601"}},
		{name: "closure with no expected type still needs an annotation", src: "pub fn f() -> Int {\n  let g = fn(p) { p }\n  g(1)\n}\n", want: []string{"XO0405"}},
	})
}
