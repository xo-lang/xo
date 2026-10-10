package check

import "testing"

// Habits from the Codex r4 first runs (bench/results/codex-xo-r4-2026-10-10.md):
// each is reported once with its own help, and the close Xo forms are not.

func TestR4HabitsReported(t *testing.T) {
	runCases(t, []ruleCase{
		// decision 0136: one XO0178, no XO0160, XO0175, or XO0177 after it
		{name: "leading or in a condition", src: "fn f(a: Int, b: Int) -> Int {\n  if a > b\n    || (a == b && b > 0) {\n    return 1\n  }\n  0\n}\n", want: []string{"XO0178"}},
		{name: "leading and", src: "fn f(a: Int, b: Int) -> Bool {\n  let ok = a > 0\n    && b > 0\n  ok\n}\n", want: []string{"XO0178"}},
		{name: "leading plus", src: "fn f(a: Int, b: Int) -> Int {\n  a\n    + b\n}\n", want: []string{"XO0178"}},
		{name: "leading comparison", src: "fn f(a: Int, b: Int) -> Bool {\n  a + 1\n    == b\n}\n", want: []string{"XO0178"}},
		{name: "leading coalesce", src: "fn f(a: Int?) -> Int {\n  a\n    ?? 0\n}\n", want: []string{"XO0178"}},
		{name: "two leading operators", src: "fn f(a: Bool, b: Bool, c: Bool) -> Bool {\n  a\n    || b\n    || c\n}\n", want: []string{"XO0178", "XO0178"}},
		{name: "leading spaced star", src: "fn f(a: Int, b: Int) -> Int {\n  a\n    * b\n}\n", want: []string{"XO0178"}},
		// an `if` without `else` whose branch has a value: one XO0401 with a
		// note (and, from a closure tail, a fix; internal/fix goldens)
		{name: "open else if in a closure", src: "fn f(a: Atomic[Int], n: Int) {\n  let g = fn() {\n    if n == 1 {\n      a.add(1)\n    } else if n == 2 {\n      a.add(2)\n    }\n  }\n  g()\n}\n", want: []string{"XO0401"}},
		{name: "if without else as a result", src: "fn f(c: Bool) -> Int {\n  if c { 1 }\n}\n", want: []string{"XO0401"}},
		// a Rust closure where an expression is expected stays XO0177
		{name: "closure argument", src: "fn f(xs: List[Int]) -> List[Int] {\n  xs.map(|x| x + 1)\n}\n", want: []string{"XO0177"}},
	})
}

func TestR4HabitsNotReported(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "operator at the end of the line", src: "fn f(a: Int, b: Int) -> Bool {\n  if a > b ||\n    (a == b && b > 0) {\n    return true\n  }\n  a +\n    b > 0\n}\n"},
		{name: "minus and bang start statements", src: "fn f(var a: Int, b: Bool) -> Int {\n  a = 1\n  -a\n  !b\n  a\n}\n"},
		{name: "method chain continues", src: "fn f(s: Str) -> Int {\n  s\n    .trim()\n    .len()\n}\n"},
		{name: "or patterns on one line", src: "fn f(x: Int) -> Int {\n  match x {\n    1 | 2 => 1\n    _ => 0\n  }\n}\n"},
		{name: "if without else as a statement", src: "fn f(a: Atomic[Int], c: Bool) {\n  if c {\n    let _ = a.add(1)\n  }\n  let g = fn() {\n    if c {\n      a.add(1)\n    }\n  }\n  g()\n}\n"},
		// decision 0134 and 0135: the r4 forms are now Xo
		{name: "while let", src: "fn f(var xs: List[Int]) -> Int {\n  var t = 0\n  while let Some(v) = xs.pop() {\n    t += v\n  }\n  t\n}\n"},
		{name: "for over bytes", src: "fn f(s: Str) -> Bool {\n  for c in s.bytes() {\n    if !(c >= 'a' && c <= 'z') && c != '-' {\n      return false\n    }\n  }\n  true\n}\n"},
	})
}
