package check

import "testing"

// Rules from the planted bug replant (spec draft 0.6): Map index reads
// (XO0410), wildcard arms on own enums (XO0305), split critical sections
// (XO0804).

func TestMapIndexRead(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "read by index", src: "fn f(m: Map[Str, Int]) -> Int {\n  m[\"a\"]\n}\n", want: []string{"XO0410"}},
		{name: "read by index in an expression", src: "fn f(m: Map[Str, Int]) -> Int {\n  m[\"a\"] + 1\n}\n", want: []string{"XO0410"}},
		{name: "get_or_fault", src: "fn f(m: Map[Str, Int]) -> Int {\n  m.get_or_fault(\"a\", \"seeded at startup\")\n}\n"},
		{name: "get", src: "fn f(m: Map[Str, Int]) -> Int {\n  m.get(\"a\") ?? 0\n}\n"},
		{name: "write by index", src: "fn f(var m: Map[Str, Int]) {\n  m[\"a\"] = 1\n}\n"},
		{name: "list index read is still allowed", src: "fn f(xs: List[Int]) -> Int {\n  xs[0]\n}\n"},
	})
}

func TestWildcardOwnEnum(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "wildcard hides own variants", src: "enum E { A\n  B\n  C }\nfn f(e: E) -> Int {\n  match e {\n    A => 1\n    _ => 2\n  }\n}\n", want: []string{"XO0305"}},
		{name: "binding catch all hides own variants", src: "enum E { A\n  B }\nfn f(e: E) -> Int {\n  match e {\n    A => 1\n    other => 2\n  }\n}\n", want: []string{"XO0305", "XO0110"}},
		{name: "all variants listed", src: "enum E { A\n  B }\nfn f(e: E) -> Int {\n  match e {\n    A => 1\n    B => 2\n  }\n}\n"},
		{name: "or pattern lists the rest", src: "enum E { A\n  B\n  C }\nfn f(e: E) -> Int {\n  match e {\n    A => 1\n    B | C => 2\n  }\n}\n"},
		{name: "wildcard on a prelude enum is fine", src: "fn f(o: Int?) -> Int {\n  match o {\n    Some(v) => v\n    _ => 0\n  }\n}\n"},
		{name: "wildcard on a non enum is fine", src: "fn f(n: Int) -> Int {\n  match n {\n    0 => 1\n    _ => 2\n  }\n}\n"},
	})
}

func TestSplitCriticalSection(t *testing.T) {
	const head = "struct B { pub n: Int }\n"
	runCases(t, []ruleCase{
		{name: "read then write in two sections", src: head + "fn f(m: Mutex[Map[Str, B]]) {\n  let b = m.with(fn(s) { s.get(\"k\") ?? B{n: 0} })\n  m.with(fn(var s) { s.put(\"k\", b with {n: b.n + 1}) })\n}\n", want: []string{"XO0804"}},
		{name: "derived value flows into the second section", src: head + "fn f(m: Mutex[Map[Str, B]]) {\n  let b = m.with(fn(s) { s.get(\"k\") ?? B{n: 0} })\n  let next = b.n + 1\n  m.with(fn(var s) { s.put(\"k\", B{n: next}) })\n}\n", want: []string{"XO0804"}},
		{name: "check in one section then act in another", src: head + "enum E { Taken }\nfn f(m: Mutex[Map[Str, B]]) -> () ! E {\n  let taken = m.with(fn(s) { s.contains(\"k\") })\n  if taken {\n    return Err(E.Taken)\n  }\n  m.with(fn(var s) { s.put(\"k\", B{n: 1}) })\n}\n", want: []string{"XO0804"}},
		{name: "one section", src: head + "fn f(m: Mutex[Map[Str, B]]) {\n  m.with(fn(var s) {\n    let b = s.get(\"k\") ?? B{n: 0}\n    s.put(\"k\", b with {n: b.n + 1})\n  })\n}\n"},
		{name: "read only use afterwards", src: head + "fn f(m: Mutex[Map[Str, B]]) -> Int {\n  let b = m.with(fn(s) { s.get(\"k\") ?? B{n: 0} })\n  b.n\n}\n"},
		{name: "different mutexes", src: head + "fn f(m: Mutex[Map[Str, B]], o: Mutex[Map[Str, B]]) {\n  let b = m.with(fn(s) { s.get(\"k\") ?? B{n: 0} })\n  o.with(fn(var s) { s.put(\"k\", b) })\n}\n"},
	})
}
