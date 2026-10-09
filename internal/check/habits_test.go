package check

import "testing"

// Habit diagnostics (decision 0087): each habit from another language
// gives exactly one diagnostic, and close Xo programs give none. The
// golden files with the fixes applied are in internal/fix/testdata/habits.

func TestHabitsReported(t *testing.T) {
	runCases(t, []ruleCase{
		// Codex's habits (bench/results/codex-2026-10-09.md).
		{name: "rust bounds", src: "fn f[K: Hash + Eq](k: K) -> K { k }\n", want: []string{"XO0177"}},
		{name: "arm commas", src: "fn f(x: Int) -> Str {\n  match x {\n    1 => \"a\",\n    _ => \"b\",\n  }\n}\n", want: []string{"XO0169", "XO0169"}},
		{name: "var at call site", src: "fn bump(var n: Int) { n += 1 }\nfn f() -> Int {\n  var x = 1\n  bump(var x)\n  x\n}\n", want: []string{"XO0177"}},
		{name: "use after decl", src: "fn f() -> Int { json.encode_str(1).len() }\nuse std/json\n", want: []string{"XO0173"}},
		{name: "elided effect", src: `
struct Cache[K, V, fx: Effects] {
  load: fn(K) -> V ! Str uses fx
}
fn (c: Cache[K, V, fx]) get(key: K) -> V ! Str uses fx { c.load(key) }
fn direct(c: Cache[Int, Str]) -> Str uses clock {
  let f = fn(k: Int) -> Str uses clock {
    match c.get(k) {
      Ok(s) => s
      Err(e) => e
    }
  }
  f(1)
}
`, want: []string{"XO0701"}},
		// Go.
		{name: "nil", src: "fn f(x: Int?) -> Bool { x == nil }\n", want: []string{"XO0601"}},
		{name: "short decl", src: "fn f() -> Int {\n  x := 1\n  x\n}\n", want: []string{"XO0177"}},
		{name: "go error idiom", src: "fn g() -> Int ! Str { 1 }\nfn f() -> Int ! Str {\n  x, err := g()\n  if err != nil {\n    return err\n  }\n  x\n}\n", want: []string{"XO0177"}},
		{name: "func", src: "func f() -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "go result type", src: "fn f(x: Int) Int { x }\n", want: []string{"XO0177"}},
		{name: "go results with error", src: "fn f(x: Int) (Int, error) {\n  return x, nil\n}\n", want: []string{"XO0177", "XO0177"}},
		{name: "go param", src: "fn f(x Int) -> Int { x }\n", want: []string{"XO0177"}},
		{name: "go field", src: "struct P {\n  x Int\n}\n", want: []string{"XO0177"}},
		{name: "go type struct", src: "type P struct {\n  x: Int\n}\n", want: []string{"XO0177"}},
		{name: "go pointer receiver", src: "struct P { x: Int }\nfn (p *P) get() -> Int { p.x }\n", want: []string{"XO0177"}},
		{name: "go var type", src: "fn f() -> Int {\n  var x Int = 1\n  x\n}\n", want: []string{"XO0177"}},
		{name: "c for", src: "fn f() -> Int {\n  var t = 0\n  for i := 0; i < 10; i++ {\n    t += i\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "go range list", src: "fn f(xs: List[Int]) -> Int {\n  var t = 0\n  for _, x := range xs {\n    t += x\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "go range map", src: "fn f(m: Map[Str, Int]) -> Int {\n  var t = 0\n  for k, v := range m {\n    t += v + k.len()\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "go for cond", src: "fn f() -> Int {\n  var i = 0\n  for i < 3 {\n    i += 1\n  }\n  i\n}\n", want: []string{"XO0177"}},
		{name: "go for ever", src: "fn f() -> Int {\n  for {\n    return 1\n  }\n}\n", want: []string{"XO0177"}},
		{name: "increment", src: "fn f() -> Int {\n  var t = 0\n  t++\n  t\n}\n", want: []string{"XO0177"}},
		{name: "len", src: "fn f(xs: List[Int]) -> Int { len(xs) }\n", want: []string{"XO0601"}},
		{name: "append", src: "fn f(xs: List[Int]) -> List[Int] { append(xs, 1) }\n", want: []string{"XO0601"}},
		{name: "switch", src: "fn f(x: Int) -> Str {\n  switch x {\n  case 1, 2:\n    \"a\"\n  default:\n    \"b\"\n  }\n}\n", want: []string{"XO0177"}},
		{name: "slice", src: "fn f(xs: List[Int]) -> List[Int] { xs[1:3] }\n", want: []string{"XO0177"}},
		{name: "slice type", src: "fn f(xs: []Int) -> Int { xs.len() }\n", want: []string{"XO0177"}},
		{name: "map type", src: "fn f(m: map[Str]Int) -> Int { m.len() }\n", want: []string{"XO0177"}},
		{name: "address of", src: "fn bump(var n: Int) { n += 1 }\nfn f() -> Int {\n  var x = 1\n  bump(&x)\n  x\n}\n", want: []string{"XO0177"}},
		{name: "deref", src: "fn f(p: Int) -> Int { *p }\n", want: []string{"XO0177"}},
		{name: "package", src: "package main\n\nfn f() -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "go type names", src: "fn f(x: int) -> string { \"${x}\" }\n", want: []string{"XO0601", "XO0601"}},
		{name: "error type", src: "fn f() -> Int ! error { 1 }\n", want: []string{"XO0601"}},
		// Rust.
		{name: "let mut", src: "fn f() -> Int {\n  let mut x = 1\n  x += 1\n  x\n}\n", want: []string{"XO0177"}},
		{name: "impl", src: "struct P { x: Int }\nimpl P {\n  fn get(&self) -> Int { self.x }\n}\n", want: []string{"XO0177"}},
		{name: "trait", src: "trait Shape {\n  fn area(self) -> Float\n}\n", want: []string{"XO0177"}},
		{name: "amp self", src: "interface Shape {\n  fn area(&self) -> Float\n}\n", want: []string{"XO0177"}},
		{name: "angle generics", src: "fn f(xs: List<Int>) -> Int { xs.len() }\n", want: []string{"XO0177"}},
		{name: "vec", src: "fn f(xs: Vec[Int]) -> Int { xs.len() }\n", want: []string{"XO0601"}},
		{name: "path", src: "enum E {\n  A\n  B\n}\nfn f() -> E { E::A }\n", want: []string{"XO0177"}},
		{name: "rust closure", src: "fn f(xs: List[Int]) -> List[Int] { xs.map(|x| x + 1) }\n", want: []string{"XO0177"}},
		{name: "as cast", src: "fn f(x: Int) -> Float { x as Float }\n", want: []string{"XO0177"}},
		{name: "loop", src: "fn f() -> Int {\n  loop {\n    break\n  }\n  1\n}\n", want: []string{"XO0177"}},
		{name: "pub crate", src: "pub(crate) fn f() -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "unwrap", src: "fn f(x: Int?) -> Int { x.unwrap() }\n", want: []string{"XO0601"}},
		{name: "expect", src: "fn f(x: Int?) -> Int { x.expect(\"set\") }\n", want: []string{"XO0601"}},
		{name: "clone", src: "fn f(xs: List[Int]) -> List[Int] { xs.clone() }\n", want: []string{"XO0601"}},
		{name: "string type", src: "fn f(x: String) -> Str { x }\n", want: []string{"XO0601"}},
		{name: "vec macro", src: "fn f() -> List[Int] { vec![1, 2] }\n", want: []string{"XO0177"}},
		{name: "attribute", src: "#[derive(Debug, Clone)]\nstruct P { x: Int }\n", want: []string{"XO0150"}},
		// TypeScript.
		{name: "null", src: "fn f(x: Int?) -> Bool { x == null }\n", want: []string{"XO0601"}},
		{name: "function", src: "function f() -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "colon result", src: "fn f(x: Int): Int { x }\n", want: []string{"XO0177"}},
		{name: "arrow fn", src: "fn f(xs: List[Int]) -> List[Int] { xs.map((x) => x + 1) }\n", want: []string{"XO0177"}},
		{name: "ternary", src: "fn f(x: Int) -> Int { x > 0 ? 1 : 2 }\n", want: []string{"XO0177"}},
		{name: "strict equality", src: "fn f(x: Int) -> Bool { x === 1 }\n", want: []string{"XO0177"}},
		{name: "template string", src: "fn f(x: Int) -> Str { `v ${x}` }\n", want: []string{"XO0177"}},
		{name: "throw", src: "fn f(x: Int) -> Int ! Str {\n  if x < 0 {\n    throw \"neg\"\n  }\n  x\n}\n", want: []string{"XO0177"}},
		{name: "try catch", src: "fn g() -> Int ! Str { 1 }\nfn f() -> Int {\n  try {\n    g()\n  } catch (e) {\n    0\n  }\n}\n", want: []string{"XO0177"}},
		{name: "new", src: "struct P { x: Int }\nfn f() -> P { new P{x: 1} }\n", want: []string{"XO0177"}},
		{name: "this", src: "struct P { x: Int }\nfn (p: P) get() -> Int { this.x }\n", want: []string{"XO0601"}},
		{name: "length", src: "fn f(xs: List[Int]) -> Int { xs.length }\n", want: []string{"XO0601"}},
		{name: "camel method", src: "fn f(s: Str) -> Bool { s.startsWith(\"a\") }\n", want: []string{"XO0601"}},
		{name: "for of", src: "fn f(xs: List[Int]) -> Int {\n  var t = 0\n  for x of xs {\n    t += x\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "for paren", src: "fn f(xs: List[Int]) -> Int {\n  var t = 0\n  for (const x of xs) {\n    t += x\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "optional field", src: "struct P {\n  x?: Int\n}\n", want: []string{"XO0177"}},
		{name: "local const", src: "fn g() -> Int { 1 }\nfn f() -> Int {\n  const x = g()\n  x\n}\n", want: []string{"XO0408"}},
		{name: "block comment", src: "/* adds one */\nfn f(x: Int) -> Int { x + 1 }\n", want: []string{"XO0177"}},
		{name: "await", src: "fn g() -> Int { 1 }\nfn f() -> Int { await g() }\n", want: []string{"XO0177"}},
		// Python.
		{name: "def", src: "def f() -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "elif", src: "fn f(x: Int) -> Int {\n  if x > 1 { 1 } elif x > 0 { 2 } else { 3 }\n}\n", want: []string{"XO0177"}},
		{name: "and", src: "fn f(a: Bool, b: Bool) -> Bool { a and b }\n", want: []string{"XO0177"}},
		{name: "not", src: "fn f(a: Bool) -> Bool { not a }\n", want: []string{"XO0177"}},
		{name: "in test", src: "fn f(x: Int, xs: List[Int]) -> Bool { x in xs }\n", want: []string{"XO0177"}},
		{name: "True", src: "fn f() -> Bool { True }\n", want: []string{"XO0601"}},
		{name: "range call", src: "fn f() -> Int {\n  var t = 0\n  for i in range(10) {\n    t += i\n  }\n  t\n}\n", want: []string{"XO0601"}},
		{name: "lambda", src: "fn f(xs: List[Int]) -> List[Int] { xs.map(lambda x: x + 1) }\n", want: []string{"XO0177"}},
		{name: "fstring", src: "fn f(x: Int) -> Str { f\"v {x}\" }\n", want: []string{"XO0177"}},
		{name: "single quotes", src: "fn f() -> Str { 'abc' }\n", want: []string{"XO0152"}},
		{name: "hash comment", src: "# adds one\nfn f(x: Int) -> Int { x + 1 }\n", want: []string{"XO0150"}},
		{name: "raise", src: "fn f(x: Int) -> Int ! Str {\n  if x < 0 {\n    raise \"neg\"\n  }\n  x\n}\n", want: []string{"XO0177"}},
		{name: "pass", src: "fn f() {\n  pass\n}\n", want: []string{"XO0177"}},
		{name: "power", src: "fn f(x: Int) -> Int { x ** 2 }\n", want: []string{"XO0177"}},
		{name: "self in method", src: "struct P { x: Int }\nfn (p: P) get() -> Int { self.x }\n", want: []string{"XO0601"}},
		{name: "python cond", src: "fn f(x: Int) -> Int { 1 if x > 0 else 2 }\n", want: []string{"XO0177"}},
		{name: "print with os", src: "fn main(os: Os) {\n  print(\"hi\")\n}\n", want: []string{"XO0601"}},
		{name: "println with os", src: "fn main(os: Os) {\n  println(\"hi\")\n}\n", want: []string{"XO0601"}},
		// Swift.
		{name: "guard let", src: "fn f(x: Int?) -> Int {\n  guard let v = x else { return 0 }\n  v\n}\n", want: []string{"XO0177"}},
		{name: "throws", src: "fn f() throws -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "half open range", src: "fn f() -> Int {\n  var t = 0\n  for i in 0..<10 {\n    t += i\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "closed range", src: "fn f() -> Int {\n  var t = 0\n  for i in 0...10 {\n    t += i\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "swift interpolation", src: "fn f(x: Int) -> Str { \"v \\(x)\" }\n", want: []string{"XO0153"}},
		{name: "inout", src: "fn bump(n: inout Int) { n += 1 }\n", want: []string{"XO0177"}},
		{name: "try prefix", src: "fn g() -> Int ! Str { 1 }\nfn f() -> Int ! Str {\n  let x = try g()\n  x\n}\n", want: []string{"XO0177"}},
		{name: "force unwrap", src: "fn f(x: Int?) -> Int { x!! }\n", want: []string{"XO0177"}},
		// Kotlin.
		{name: "fun", src: "fun f() -> Int { 1 }\n", want: []string{"XO0177"}},
		{name: "val", src: "fn f() -> Int {\n  val x = 1\n  x\n}\n", want: []string{"XO0177"}},
		{name: "when", src: "fn f(x: Int) -> Str {\n  when (x) {\n    1 -> \"a\"\n    else -> \"b\"\n  }\n}\n", want: []string{"XO0177"}},
		{name: "elvis", src: "fn f(x: Int?) -> Int { x ?: 0 }\n", want: []string{"XO0177"}},
		{name: "until", src: "fn f() -> Int {\n  var t = 0\n  for i in 0 until 10 {\n    t += i\n  }\n  t\n}\n", want: []string{"XO0177"}},
		{name: "thin arrow arms", src: "fn f(x: Int) -> Str {\n  match x {\n    1 -> \"a\"\n    _ -> \"b\"\n  }\n}\n", want: []string{"XO0177", "XO0177"}},
	})
}

// TestHabitsNotReported keeps valid Xo that looks like a habit free of
// habit diagnostics.
func TestHabitsNotReported(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "xo forms", src: `
struct Job { id: Int, name: Str }
enum E {
  A
  B
}
interface Shape {
  fn area(self) -> Float
}
fn largest[T: Ord](a: T, b: T) -> T { if (a > b) { a } else { b } }
fn keyed[K: Hash, V](m: Map[K, V], k: K) -> V? { m.get(k) }
fn (var j: Job) rename(n: Str) { j.name = n }
fn loops(jobs: List[Job], m: Map[Str, Int]) -> Int {
  var t = 0
  for Job{id, name} in jobs {
    t += id + name.len()
  }
  for (k, v) in m {
    t += v + k.len()
  }
  outer: for i in 0..3 {
    for j in 0..=2 {
      if j == 1 { continue outer }
      t += i * j
    }
  }
  while t < 0 {
    t += 1
  }
  t - -1
}
fn options(o: Int?, r: Result[Int, Str]) -> Int? {
  let a = o?
  let b = r.ok() ?? 0
  let ok = r is Ok(_) && o is Some(_)
  if ok { Some(a + b) } else { None }
}
fn tries(r: Result[Int, Str]) -> Int ! Str {
  let n = r? + 1
  let s = "price: \$5, braces {x} ${n}"
  let c = 'a'
  let set = #{1, 2}
  n + s.len() + set.len() + Int(c)
}
fn slices(xs: List[Int]) -> List[Int] { xs[1..3] + xs[..1] + xs[2..] }
fn matcher(x: Int) -> Str {
  match x {
    1 | 2 => "small"
    n if n > 10 => "big"
    _ => "other"
  }
}
fn bits(a: Int, b: Int) -> Int { (a & b) | (a ^ b) }
fn names() -> Int {
  let new = 1
  let go = 2
  let when = 3
  let until = 4
  let of = 5
  let val = 6
  let f = fn(x: Int) -> Int { x }
  new + go + when + until + of + val + f(1)
}
fn closures(xs: List[Int]) -> List[Int] { xs.map(fn(x) { x * 2 }).filter(fn(x) { x > 2 }) }
fn picks(e: E) -> Int {
  match e {
    E.A => 1
    B => 2
  }
}
`},
		{name: "sig on lines", src: `
pub fn f(a: Int)
  -> Int
  requires a > 0
{
  a
}
`},
		{name: "map literal value", src: "fn f(x: Int) -> Map[Str, Int] { {\"a\": x, \"b\": 2} }\n"},
		{name: "struct literal in if", src: "struct P { x: Int }\nfn f(p: P) -> Bool { if (p == P{x: 0}) { true } else { false } }\n"},
		{name: "ensures", src: "fn f(a: Int) -> Int\n  ensures result > a\n{\n  a + 1\n}\n"},
	})
}
