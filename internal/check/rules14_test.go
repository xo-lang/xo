package check

import "testing"

// Decision 0133: real-time functions (core 6.5) and loop bounds.
func TestRealtimeRules(t *testing.T) {
	const head = "const N = 8\nstruct S { xs: List[Int], name: Str }\n"
	runCases(t, []ruleCase{
		{name: "scalar code is fine", src: head + "fn f(x: Int) -> Int uses realtime {\n  (x * 3).abs() % 7\n}\n"},
		{name: "index read and in place write are fine", src: head + "fn f(var s: S, i: Int) -> Int uses realtime {\n  s.xs[i] = s.xs[i] + 1\n  s.xs.len()\n}\n"},
		{name: "push allocates", src: head + "fn f(var s: S) uses realtime {\n  s.xs.push(1)\n}\n", want: []string{"XO1401"}},
		{name: "interpolation allocates", src: head + "fn f(x: Int) -> Str uses realtime {\n  \"${x}\"\n}\n", want: []string{"XO1401"}},
		{name: "fault message may allocate", src: head + "fn f(x: Int) -> Int uses realtime {\n  if x < 0 { fault(\"negative ${x}\") }\n  x\n}\n"},
		{name: "list concatenation allocates", src: head + "fn f(s: S) -> Int uses realtime {\n  (s.xs + s.xs).len()\n}\n", want: []string{"XO1401"}},
		{name: "closure allocates", src: head + "fn f(x: Int) -> Int uses realtime {\n  let g = fn(y: Int) -> Int { y }\n  g(x)\n}\n", want: []string{"XO1401", "XO1403"}},
		{name: "constant range is bounded", src: head + "fn f() -> Int uses realtime {\n  var t = 0\n  for i in 0..N { t += i }\n  t\n}\n"},
		{name: "collection loop needs a bound", src: head + "fn f(s: S) -> Int uses realtime {\n  var t = 0\n  for x in s.xs { t += x }\n  t\n}\n", want: []string{"XO1402"}},
		{name: "collection loop with a bound", src: head + "fn f(s: S) -> Int uses realtime {\n  var t = 0\n  for x in s.xs bound N { t += x }\n  t\n}\n"},
		{name: "while needs a bound", src: head + "fn f(n: Int) -> Int uses realtime {\n  var i = 0\n  while i < n { i += 1 }\n  i\n}\n", want: []string{"XO1402"}},
		{name: "bound must be constant", src: head + "fn f(n: Int) -> Int {\n  var i = 0\n  while i < n bound n { i += 1 }\n  i\n}\n", want: []string{"XO1402"}},
		{name: "constant range over its bound", src: head + "fn f() -> Int {\n  var t = 0\n  for i in 0..=N bound N { t += i }\n  t\n}\n", want: []string{"XO1402"}},
		{name: "call of an unmarked function", src: head + "fn g(x: Int) -> Int { x }\nfn f(x: Int) -> Int uses realtime {\n  g(x)\n}\n", want: []string{"XO1403"}},
		{name: "call of a marked function", src: head + "fn g(x: Int) -> Int uses realtime { x }\nfn f(x: Int) -> Int uses realtime {\n  g(x)\n}\n"},
		{name: "call of a function value", src: head + "fn f(g: fn(Int) -> Int, x: Int) -> Int uses realtime {\n  g(x)\n}\n", want: []string{"XO1403"}},
		{name: "self recursion", src: head + "fn f(x: Int) -> Int uses realtime {\n  if x == 0 { 0 } else { f(x - 1) }\n}\n", want: []string{"XO1404"}},
		{name: "mutual recursion", src: head + "fn f(x: Int) -> Int uses realtime {\n  if x == 0 { 0 } else { g(x - 1) }\n}\nfn g(x: Int) -> Int uses realtime {\n  f(x)\n}\n", want: []string{"XO1404"}},
		{name: "channel send blocks", src: head + "fn f(c: Chan[Int]) uses realtime {\n  c.send(1)\n}\n", want: []string{"XO1405"}},
		{name: "declared effect", src: head + "fn f(x: Int) -> Int uses realtime, net {\n  x\n}\n", want: []string{"XO1405"}},
		{name: "ffi may be declared", src: head + "fn f(x: Int) -> Int uses realtime, ffi {\n  x\n}\n"},
		{name: "realtime in a function type", src: head + "fn f(g: fn(Int) -> Int uses realtime) -> Int {\n  g(1)\n}\n", want: []string{"XO0702"}},
		{name: "realtime functions are pure", src: head + "fn f(c: Clock) -> Int uses realtime {\n  c.now().unix()\n}\n", want: []string{"XO0701", "XO1405"}},
	})
}
