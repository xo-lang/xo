package check

import (
	"fmt"
	"testing"
)

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
		// decision 0080: a `_` hiding at least 8 variants and more than
		// half of the enum is a default
		{name: "default arm on a large enum", src: bigEnum(20) + "fn f(e: E) -> Int {\n  match e {\n    V0 => 1\n    V1 | V2 => 2\n    _ => 0\n  }\n}\n"},
		{name: "default arm hiding exactly half", src: bigEnum(16) + "fn f(e: E) -> Int {\n  match e {\n    V0 | V1 | V2 | V3 | V4 | V5 | V6 | V7 => 1\n    _ => 0\n  }\n}\n", want: []string{"XO0305"}},
		{name: "few hidden on a large enum", src: bigEnum(20) + "fn f(e: E) -> Int {\n  match e {\n    V0 | V1 | V2 | V3 | V4 | V5 | V6 | V7 | V8 | V9 | V10 | V11 | V12 | V13 | V14 | V15 => 1\n    _ => 0\n  }\n}\n", want: []string{"XO0305"}},
		{name: "most hidden on a small enum", src: bigEnum(9) + "fn f(e: E) -> Int {\n  match e {\n    V0 | V1 => 1\n    _ => 0\n  }\n}\n", want: []string{"XO0305"}},
	})
}

// bigEnum declares `enum E` with variants V0 to V(n-1).
func bigEnum(n int) string {
	s := "enum E {\n"
	for i := 0; i < n; i++ {
		s += fmt.Sprintf("  V%d\n", i)
	}
	return s + "}\n"
}

func TestWildcardOwnEnumMessage(t *testing.T) {
	ds := checkSrc(t, bigEnum(30)+"fn f(e: E) -> Int {\n  match e {\n    V0 | V1 | V2 | V3 | V4 | V5 | V6 | V7 | V8 | V9 | V10 | V11 | V12 | V13 | V14 | V15 | V16 | V17 | V18 => 1\n    _ => 0\n  }\n}\n")
	if len(ds) != 1 {
		t.Fatalf("want one diagnostic, got %s", humanAll(ds))
	}
	want := "this arm hides `V19`, `V20`, `V21`, `V22`, `V23`, `V24`, `V25`, `V26` and 3 more of `E`, declared in this module"
	if ds[0].Message != want {
		t.Errorf("message = %q, want %q", ds[0].Message, want)
	}
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
		{name: "read in with_read then write", src: head + "fn f(m: Mutex[Map[Str, B]]) {\n  let b = m.with_read(fn(s) { s.get(\"k\") ?? B{n: 0} })\n  m.with(fn(var s) { s.put(\"k\", b with {n: b.n + 1}) })\n}\n", want: []string{"XO0804"}},
		{name: "with_read after with_read acts on nothing", src: head + "fn f(m: Mutex[Map[Str, B]]) -> Int {\n  let b = m.with_read(fn(s) { s.get(\"k\") ?? B{n: 0} })\n  m.with_read(fn(s) { s.len() + b.n })\n}\n"},
	})
}

// TestWithRead checks Mutex.with_read (decision 0063): the closure gets a
// read only value, so mutation is rejected as for any read only binding,
// and a `var` parameter (a copy whose changes would be lost) is rejected.
func TestWithRead(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "read", src: "fn f(m: Mutex[Map[Str, Int]]) -> Int {\n  m.with_read(fn(s) { s.get(\"k\") ?? 0 })\n}\n"},
		{name: "read with an error", src: "enum E { Missing }\nfn f(m: Mutex[Map[Str, Int]]) -> Int ! E {\n  m.with_read(fn(s) -> Int ! E { s.get(\"k\").or_err(E.Missing) })\n}\n"},
		{name: "mutating method", src: "fn f(m: Mutex[Map[Str, Int]]) {\n  m.with_read(fn(s) { s.put(\"k\", 1) })\n}\n", want: []string{"XO0501"}},
		{name: "assignment", src: "fn f(m: Mutex[Int]) {\n  m.with_read(fn(n) { n = 2 })\n}\n", want: []string{"XO0502"}},
		{name: "field assignment", src: "struct B { pub n: Int }\nfn f(m: Mutex[B]) {\n  m.with_read(fn(b) { b.n = 2 })\n}\n", want: []string{"XO0502"}},
		{name: "var parameter", src: "fn f(m: Mutex[Map[Str, Int]]) {\n  m.with_read(fn(var s) { s.put(\"k\", 1) })\n}\n", want: []string{"XO0401"}},
		{name: "var parameter for a by value function type", src: "fn apply(f: fn(Int) -> Int) -> Int { f(1) }\nfn g() -> Int {\n  apply(fn(var x) { x += 1\n    x })\n}\n", want: []string{"XO0401"}},
		{name: "var parameter for a var function type", src: "fn apply(f: fn(var Int)) {}\nfn g() {\n  apply(fn(var x) { x += 1 })\n}\n"},
	})
}
