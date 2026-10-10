package check

import "testing"

// Each rule has positive cases (the program breaks the rule and gets the
// code) and negative cases (a close program that is accepted).

func TestNaming(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "ok", src: `
struct UserRec { user_name: Str }
enum Color { DarkRed, Blue }
const MAX_USERS = 10
fn make_user(name: Str) -> UserRec { UserRec{user_name: name} }
fn apply[T, fx: Effects](f: fn() -> T uses fx) -> T uses fx { f() }
`},
		{name: "type", src: "struct user_rec { a: Int }\n", want: []string{"XO0101"}},
		{name: "function", src: "fn DoIt() {}\n", want: []string{"XO0101"}},
		{name: "const", src: "const max_users = 1\n", want: []string{"XO0101"}},
		{name: "field", src: "struct U { userName: Int }\n", want: []string{"XO0101"}},
		{name: "variant", src: "enum E { not_found }\n", want: []string{"XO0101"}},
		{name: "local", src: "fn f() -> Int {\n  let myVar = 1\n  myVar\n}\n", want: []string{"XO0101"}},
		{name: "param", src: "fn f(userId: Int) -> Int { userId }\n", want: []string{"XO0101"}},
		{name: "effect var", src: "fn f[FX: Effects](g: fn() uses FX) uses FX { g() }\n", want: []string{"XO0101"}},
		{name: "type param", src: "fn f[t](x: t) -> t { x }\n", want: []string{"XO0101"}},
		{name: "module alias", src: "use std/json as Json1\n\nfn f(b: Bytes) -> Int ! Json1.DecodeErr { Json1.decode[Int](b) }\n", want: []string{"XO0101"}},
	})
}

func TestUnused(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "ok", src: `
fn f(unused_param: Int) -> Int {
  let _ignored = 1
  let _ = 2
  let x = 3
  x
}
`},
		{name: "let", src: "fn f() {\n  let x = 1\n}\n", want: []string{"XO0110"}},
		{name: "var never read", src: "fn f() {\n  var x = 1\n  x = 2\n}\n", want: []string{"XO0110"}},
		{name: "import", src: "use std/json\n\nfn f() {}\n", want: []string{"XO0110"}},
		{name: "pattern binding", src: `
fn f(o: Int?) -> Int {
  match o {
    Some(v) => 1
    None => 0
  }
}
`, want: []string{"XO0110"}},
		{name: "pattern underscore ok", src: `
fn f(o: Int?) -> Int {
  match o {
    Some(_v) => 1
    None => 0
  }
}
`},
	})
}

func TestShadowing(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "nested ok", src: `
fn f() -> Int {
  let x = 1
  let y = {
    let x = 2
    x
  }
  x + y
}
`},
		{name: "same block", src: "fn f() -> Int {\n  let x = 1\n  let x = 2\n  x\n}\n", want: []string{"XO0603"}},
		{name: "duplicate top level", src: "fn f() {}\nfn f() {}\n", want: []string{"XO0603"}},
		{name: "predeclared", src: "struct Option { a: Int }\n", want: []string{"XO0603"}},
	})
}

func TestResolution(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "unknown name", src: "fn f() -> Int {\n  let count = 1\n  coutn\n}\n", want: []string{"XO0601", "XO0110"}},
		{name: "unknown method", src: "fn f(s: Str) -> Int { s.lenght() }\n", want: []string{"XO0601"}},
		{name: "unknown std function", src: "use std/http\n\nfn f() -> http.Response { http.jsno(200, 1) }\n", want: []string{"XO0601"}},
		{name: "unknown module", src: "use std/htp\n\nfn f() {}\n", want: []string{"XO0601"}},
		{name: "unknown type", src: "fn f(x: Strng) {}\n", want: []string{"XO0601"}},
		{name: "known", src: "use std/http\n\nfn f() -> http.Response { http.text(200, \"ok\") }\n"},
		{name: "alias needed", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"03-lib/a.xo":  "pub fn one() -> Int { 1 }\n",
			"main/main.xo": "use example.com/app/03-lib\n\nfn f() -> Int { 1 }\n",
		}, want: []string{"XO0602"}},
		{name: "alias given", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"03-lib/a.xo":  "pub fn one() -> Int { 1 }\n",
			"main/main.xo": "use example.com/app/03-lib as lib\n\nfn f() -> Int { lib.one() }\n",
		}},
		{name: "dash becomes underscore", files: map[string]string{
			"xo.mod":          "module example.com/app\nedition 2026\n",
			"rate-limit/a.xo": "pub fn one() -> Int { 1 }\n",
			"main/main.xo":    "use example.com/app/rate-limit\n\nfn f() -> Int { rate_limit.one() }\n",
		}},
		{name: "dash in a middle segment", files: map[string]string{
			"xo.mod":       "module github.com/xo-lang/app\nedition 2026\n",
			"util/a.xo":    "pub fn one() -> Int { 1 }\n",
			"main/main.xo": "use github.com/xo-lang/app/util\n\nfn f() -> Int { util.one() }\n",
		}},
		{name: "not pub", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"lib/a.xo":     "fn hidden() -> Int { 1 }\npub fn shown() -> Int { hidden() }\n",
			"main/main.xo": "use example.com/app/lib\n\nfn f() -> Int { lib.hidden() }\n",
		}, want: []string{"XO0605"}},
		{name: "non pub method satisfies interface", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"lib/a.xo":     "pub struct T { pub n: Int }\nfn (t: T) display() -> Str { \"t\" }\npub fn make() -> T { T{n: 1} }\n",
			"main/main.xo": "use example.com/app/lib\n\nfn show(d: Display) -> Str { \"${d}\" }\nfn f() -> Str { show(lib.make()) }\n",
		}},
		{name: "non pub method called", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"lib/a.xo":     "pub struct T { pub n: Int }\nfn (t: T) secret() -> Int { 1 }\npub fn make() -> T { T{n: 1} }\n",
			"main/main.xo": "use example.com/app/lib\n\nfn f() -> Int { lib.make().secret() }\n",
		}, want: []string{"XO0605"}},
		{name: "import cycle", files: map[string]string{
			"xo.mod":    "module example.com/app\nedition 2026\n",
			"a/a.xo":    "use example.com/app/b\n\npub fn fa() -> Int { b.fb() }\n",
			"b/b.xo":    "use example.com/app/a\n\npub fn fb() -> Int { 1 }\npub fn g() -> Int { a.fa() }\n",
			"c/main.xo": "fn main(os: Os) {}\n",
		}, want: []string{"XO0604"}},
		{name: "module not required", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"main/main.xo": "use github.com/acme/billing/invoice\n\nfn f() -> Int { invoice.total() }\n",
		}, want: []string{"XO1306"}},
		{name: "deny rule forbids a use", files: map[string]string{
			"xo.mod":         "module example.com/app\nedition 2026\n\ndeny example.com/app/domain -> example.com/app/store\n",
			"store/a.xo":     "pub fn one() -> Int { 1 }\n",
			"domain/main.xo": "use example.com/app/store\n\npub fn f() -> Int { store.one() }\n",
		}, want: []string{"XO1312"}},
		{name: "dot segments do not get past a deny rule", files: map[string]string{
			"xo.mod":         "module example.com/app\nedition 2026\n\ndeny example.com/app/domain -> example.com/app/store\n",
			"store/a.xo":     "pub fn one() -> Int { 1 }\n",
			"x/a.xo":         "pub fn two() -> Int { 2 }\n",
			"domain/main.xo": "use example.com/app/./store as s1\nuse example.com/app/x/../store as s2\n\npub fn f() -> Int { s1.one() + s2.one() }\n",
		}, want: []string{"XO0158", "XO0158", "XO1312", "XO1312"}},
		{name: "relative import reports only XO0158", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"lib/a.xo":     "pub fn one() -> Int { 1 }\n",
			"main/main.xo": "use ./lib as a\nuse example.com/app/lib/.. as b\nuse example.com/app/../lib as c\n\nfn f() -> Int { a.one() + b.one() + c.one() }\n",
		}, want: []string{"XO0158", "XO0158", "XO0158"}},
		{name: "deny pattern forbids a nested use", files: map[string]string{
			"xo.mod":         "module example.com/app\nedition 2026\n\ndeny example.com/app/domain -> example.com/app/...\n",
			"store/sql/a.xo": "pub fn one() -> Int { 1 }\n",
			"domain/main.xo": "use example.com/app/store/sql\n\npub fn f() -> Int { sql.one() }\n",
		}, want: []string{"XO1312"}},
		{name: "deny rule allows other uses", files: map[string]string{
			"xo.mod":         "module example.com/app\nedition 2026\n\ndeny example.com/app/store -> example.com/app/domain\n",
			"store/a.xo":     "pub fn one() -> Int { 1 }\n",
			"domain/main.xo": "use example.com/app/store\n\npub fn f() -> Int { store.one() }\n",
		}},
		{name: "deny rule names a missing package", files: map[string]string{
			"xo.mod":         "module example.com/app\nedition 2026\n\ndeny example.com/app/domain -> example.com/app/nope\n",
			"domain/main.xo": "pub fn f() -> Int { 1 }\n",
		}, want: []string{"XO1301"}},
		{name: "malformed deny line", files: map[string]string{
			"xo.mod":         "module example.com/app\nedition 2026\n\ndeny example.com/app/domain example.com/app/nope\n",
			"domain/main.xo": "pub fn f() -> Int { 1 }\n",
		}, want: []string{"XO1301"}},
		{name: "newtype of another module", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"lib/a.xo":     "pub type Email struct(Str)\n",
			"main/main.xo": "use example.com/app/lib\n\nfn f(e: lib.Email) -> Str {\n  let lib.Email(s) = e\n  s + e.0\n}\n",
		}},
		{name: "missing package of the main module", files: map[string]string{
			"xo.mod":       "module example.com/app\nedition 2026\n",
			"main/main.xo": "use example.com/app/nope\n\nfn f() -> Int { nope.total() }\n",
		}, want: []string{"XO0601"}},
		{name: "foreign method", src: "fn (s: Str) shout() -> Str { s }\n", want: []string{"XO0606"}},
		{name: "break outside loop", src: "fn f() {\n  break\n}\n", want: []string{"XO0607"}},
		{name: "break in loop", src: "fn f() {\n  while {\n    break\n  }\n}\n"},
	})
}

func TestTypes(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "no implicit numeric", src: "fn f(i: Int) -> Float { i }\n", want: []string{"XO0401"}},
		{name: "explicit numeric", src: "fn f(i: Int) -> Float { Float(i) }\n"},
		{name: "rune literal as byte", src: "fn f(b: Bytes) -> Bool { b[0] == '\"' || '{' == b[1] }\n"},
		{name: "rune literal pattern on byte", src: "fn f(c: UInt8) -> Int {\n  match c {\n    'a'..='z' => 1\n    '_' => 2\n    _ => 0\n  }\n}\n"},
		{name: "non ascii rune as byte", src: "fn f() -> UInt8 { 'é' }\n", want: []string{"XO0401"}},
		{name: "rune literal too big", src: "fn f() -> Int16 { '\U0001F600' }\n", want: []string{"XO0401"}},
		{name: "rune to int and back", src: "fn f(r: Rune) -> Rune? { Rune.try(Int(r) + 1) }\nfn g(n: UInt8) -> Rune { Rune(n) }\n"},
		{name: "rune to float", src: "fn f(r: Rune) -> Float { Float(r) }\n", want: []string{"XO0409"}},
		{name: "str to rune", src: "fn f(s: Str) -> Rune { Rune(s) }\n", want: []string{"XO0409"}},
		{name: "rune compared with byte", src: "fn f(c: UInt8, r: Rune) -> Bool { c == r }\n", want: []string{"XO0406"}},
		{name: "some wrapping", src: "fn f(i: Int) -> Int? { i }\n"},
		{name: "interface conversion", src: "fn show(d: Display) -> Str { \"${d}\" }\nfn f() -> Str { show(5) }\n"},
		{name: "interface not implemented", src: "struct P { x: Int }\nfn show(d: Display) -> Str { \"${d}\" }\nfn f() -> Str { show(P{x: 1}) }\n", want: []string{"XO0401"}},
		{name: "derive display", src: "struct P { x: Int } derive Display\nfn show(d: Display) -> Str { \"${d}\" }\nfn f() -> Str { show(P{x: 1}) }\n"},
		{name: "interpolate non display", src: "struct P { x: Int }\nfn f(p: P) -> Str { \"${p}\" }\n", want: []string{"XO0404"}},
		{name: "interpolate bytes", src: "fn f(b: Bytes) -> Str { \"${b}\" }\n", want: []string{"XO0404"}},
		{name: "missing field", src: "struct P { x: Int, y: Int }\nfn f() -> P { P{x: 1} }\n", want: []string{"XO0403"}},
		{name: "default field", src: "struct P { x: Int, y: Int = 0 }\nfn f() -> P { P{x: 1} }\n"},
		{name: "unknown field", src: "struct P { x: Int }\nfn f() -> P { P{x: 1, z: 2} }\n", want: []string{"XO0601"}},
		{name: "with update", src: "struct P { x: Int, y: Int }\nfn f(p: P) -> P { p with {y: 2} }\n"},
		{name: "with wrong type", src: "struct P { x: Int, y: Int }\nfn f(p: P) -> P { p with {y: \"a\"} }\n", want: []string{"XO0401"}},
		{name: "generic struct inference", src: "struct Job[T] { id: Int, input: T }\nfn f() -> Job[Str] { Job{id: 1, input: \"x\"} }\n"},
		{name: "generic struct mismatch", src: "struct Job[T] { id: Int, input: T }\nfn f() -> Job[Int] { Job{id: 1, input: \"x\"} }\n", want: []string{"XO0401"}},
		{name: "generic inference at call", src: "fn largest[T: Ord](a: T, b: T) -> T { if a > b { a } else { b } }\nfn f() -> Int { largest(1, 2) }\n"},
		{name: "constraint not met", src: "struct P { x: Int }\nfn largest[T: Ord](a: T, b: T) -> T { if a > b { a } else { b } }\nfn f(p: P) -> P { largest(p, p) }\n", want: []string{"XO0404"}},
		{name: "ambiguous type args", src: "use std/json\n\nfn f(b: Bytes) -> Int {\n  let _ = json.decode(b).ok()\n  1\n}\n", want: []string{"XO0402"}},
		{name: "explicit type args", src: "use std/json\n\nfn f(b: Bytes) -> Int ! json.DecodeErr { json.decode[Int](b) }\n"},
		{name: "duration math", src: "fn f(d: Duration, n: Int) -> Duration { d * n + 5s }\n"},
		{name: "duration plus int", src: "fn f(d: Duration) -> Duration { d + 5 }\n", want: []string{"XO0406"}},
		{name: "time minus time", src: "fn f(a: Time, b: Time) -> Duration { a - b }\n"},
		{name: "time plus time", src: "fn f(a: Time, b: Time) -> Time { a + b }\n", want: []string{"XO0406"}},
		{name: "duration ratio", src: "fn f(a: Duration, b: Duration) -> Int { a / b }\n"},
		{name: "never converts", src: "fn f(x: Int) -> Str {\n  if x > 0 { fault(\"no\") } else { \"ok\" }\n}\n"},
		{name: "type as value", src: "struct P { x: Int }\nfn f() -> Int {\n  let _ = P\n  1\n}\n", want: []string{"XO0407"}},
		{name: "bad conversion", src: "fn f(s: Str) -> Int { Int(s) }\n", want: []string{"XO0409"}},
		{name: "const call", src: "fn g() -> Int { 1 }\nconst X = g()\n", want: []string{"XO0408"}},
		{name: "const ok", src: "struct C { port: Int }\nconst DEFAULT = C{port: 8080}\nconst PORTS = [1, 2]\n"},
		{name: "closure param needs type", src: "fn f() -> Int {\n  let g = fn(x) { x }\n  1\n}\n", want: []string{"XO0405", "XO0110"}},
		{name: "closure from context", src: "fn f(xs: List[Int]) -> List[Int] { xs.map(fn(x) { x * 2 }) }\n"},
		{name: "tuples", src: "fn f(p: (Int, Str)) -> Str {\n  let (n, s) = p\n  \"${n}${s}\"\n}\n"},
		{name: "variant positional multi", src: "enum E { A(x: Int, y: Int) }\nfn f() -> E { E.A(1, 2) }\n", want: []string{"XO0403"}},
		{name: "variant named", src: "enum E { A(x: Int, y: Int) }\nfn f() -> E { E.A(x: 1, y: 2) }\n"},
		{name: "variant bare with expected type", src: "enum E { A(x: Int), B }\nfn f(c: Bool) -> E { if c { A(x: 1) } else { B } }\n"},
		{name: "argument count", src: "fn g(a: Int) -> Int { a }\nfn f() -> Int { g(1, 2) }\n", want: []string{"XO0403"}},
		{name: "named arguments", src: "fn g(a: Int, b: Int = 2) -> Int { a + b }\nfn f() -> Int { g(b: 3, a: 1) }\n"},
		{name: "interface method effect", src: "interface Store {\n  fn get(self, id: Int) -> Int uses net\n}\nfn f(s: Store) -> Int uses net { s.get(1) }\n"},
		{name: "fake satisfies capability", src: "fn now(c: Clock) -> Time uses clock { c.now() }\ntest \"t\" {\n  let _ = now(os.clock)\n}\n"},
		{name: "list index and slice", src: "fn f(xs: List[Int]) -> List[Int] {\n  let _a = xs[0]\n  xs[1..]\n}\n"},
		{name: "string index", src: "fn f(s: Str) -> Int {\n  let _c = s[0]\n  1\n}\n", want: []string{"XO0406"}},
	})
}

func TestErrors(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "unhandled", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  g()\n}\n", want: []string{"XO0201"}},
		{name: "unhandled unit", src: "enum E { A }\nfn g() ! E {}\nfn f() ! E {\n  g()\n  g()\n}\n", want: []string{"XO0201"}},
		{name: "discard without a reason", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  let _ = g()\n}\n", want: []string{"XO0206"}},
		{name: "discard with a reason", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  let _ = g().ignore_err(\"best effort\")\n  g().ignore_err(\"best effort\")\n}\n"},
		{name: "propagated", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() -> Int ! E { g()? + 1 }\n"},
		{name: "matched", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() -> Int {\n  match g() {\n    Ok(v) => v\n    Err(_) => 0\n  }\n}\n"},
		{name: "returned as is", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() -> Int ! E { g() }\n"},
		{name: "err return", src: "enum E { A }\nfn f(x: Int) -> Int ! E {\n  if x < 0 {\n    return Err(E.A)\n  }\n  x\n}\n"},
		{name: "tail err", src: "enum E { A }\nfn f(x: Int) -> Int ! E {\n  if x < 0 { Err(A) } else { x }\n}\n"},
		{name: "convert from variant", src: "enum LoadErr { Timeout }\nenum ApiErr { Load(from LoadErr) }\nfn g() -> Int ! LoadErr { 1 }\nfn f() -> Int ! ApiErr { g()? }\n"},
		{name: "convert to Error", src: "enum LoadErr { Timeout } derive Display\nfn g() -> Int ! LoadErr { 1 }\nfn f() -> Int ! { g()? }\n"},
		{name: "no conversion", src: "enum LoadErr { Timeout }\nenum ApiErr { Bad }\nfn g() -> Int ! LoadErr { 1 }\nfn f() -> Int ! ApiErr { g()? }\n", want: []string{"XO0202"}},
		{name: "not display to Error", src: "enum LoadErr { Timeout }\nfn g() -> Int ! LoadErr { 1 }\nfn f() -> Int ! { g()? }\n", want: []string{"XO0202"}},
		{name: "question in infallible", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() -> Int { g()? }\n", want: []string{"XO0205"}},
		{name: "result of result needs ok", src: "enum E { A }\nfn f(r: Result[Int, Str]) -> Result[Int, Str] ! E { r }\n", want: []string{"XO0203"}},
		{name: "result of result with ok", src: "enum E { A }\nfn f(r: Result[Int, Str]) -> Result[Int, Str] ! E { Ok(r) }\n"},
		{name: "option question in option fn", src: "fn f(s: Str?) -> Int? {\n  let t = s?\n  Int.parse(t)\n}\n"},
		{name: "option question in result fn", src: "enum E { Missing }\nfn f(s: Str?) -> Str ! E { s? }\n", want: []string{"XO0204"}},
		{name: "option or_err", src: "enum E { Missing }\nfn f(s: Str?) -> Str ! E { s.or_err(E.Missing)? }\n"},
		{name: "option question in plain fn", src: "fn f(s: Str?) -> Str { s? }\n", want: []string{"XO0205"}},
		{name: "requires else", src: "enum E { Bad }\nfn f(x: Int) -> Int ! E\n  requires x > 0 else E.Bad\n{\n  x\n}\n"},
		{name: "requires else must have the error type", src: "enum LoadErr { Bad }\nenum E { Load(from LoadErr) }\nfn f(x: Int) -> Int ! E\n  requires x > 0 else LoadErr.Bad\n{\n  x\n}\n", want: []string{"XO0401"}},
		{name: "branches need an expected type to convert", src: "fn f(c: Bool) -> Int {\n  let x = if c { 1 } else { None }\n  x ?? 0\n}\n", want: []string{"XO0401"}},
		{name: "branches convert with an expected type", src: "fn f(c: Bool) -> Int {\n  let x: Int? = if c { 1 } else { None }\n  x ?? 0\n}\n"},
		{name: "requires else in infallible", src: "enum E { Bad }\nfn f(x: Int) -> Int\n  requires x > 0 else E.Bad\n{\n  x\n}\n", want: []string{"XO0205"}},
		{name: "test body may fail", src: "enum E { A } derive Display\nfn g() -> Int ! E { 1 }\ntest \"t\" {\n  let v = g()?\n  expect(v) == 1\n}\n"},
	})
}

func TestMatch(t *testing.T) {
	runCases(t, []ruleCase{
		// newtypes (core 2.8, decision 0097)
		{name: "newtype value", src: "type Email struct(Str)\nfn f(e: Email) -> Int { e.0.len() }\n"},
		{name: "newtype value assigned", src: "type Email struct(Str)\nfn f() -> Str {\n  var e = Email(\"a\")\n  e.0 = \"b\"\n  e.0\n}\n"},
		{name: "newtype has only .0", src: "type Email struct(Str)\nfn f(e: Email) -> Str { e.1 }\n", want: []string{"XO0601"}},
		{name: "newtype pattern let", src: "type Email struct(Str)\nfn f(e: Email) -> Str {\n  let Email(s) = e\n  s\n}\n"},
		{name: "newtype pattern generic", src: "type Wrap[T] struct(T)\nfn f(w: Wrap[Int]) -> Int {\n  let Wrap(n) = w\n  n + w.0\n}\n"},
		{name: "newtype pattern refutable let", src: "type Email struct(Str)\nfn f(e: Email) -> Int {\n  let Email(\"x\") = e\n  1\n}\n", want: []string{"XO0303"}},
		{name: "newtype match exhaustive", src: "type Email struct(Str)\nfn f(e: Email) -> Int {\n  match e {\n    Email(\"\") => 0\n    Email(s) => s.len()\n  }\n}\n"},
		{name: "newtype match missing", src: "type Email struct(Str)\nfn f(e: Email) -> Int {\n  match e {\n    Email(\"\") => 0\n  }\n}\n", want: []string{"XO0301"}},
		{name: "newtype pattern without parens", src: "type Email struct(Str)\nfn f(e: Email) -> Int {\n  match e {\n    Email => 0\n  }\n}\n", want: []string{"XO0302"}},
		{name: "newtype pattern two fields", src: "type Email struct(Str)\nfn f(e: Email) -> Int {\n  match e {\n    Email(a, b) => 0\n  }\n}\n", want: []string{"XO0302"}},
		{name: "newtype pattern other type", src: "type Email struct(Str)\ntype Id struct(Int)\nfn f(i: Id) -> Int {\n  match i {\n    Email(s) => 0\n    Id(n) => n\n  }\n}\n", want: []string{"XO0302"}},
		{name: "variant wins over newtype", src: "type B struct(Int)\nenum E { B(n: Int), C }\nfn f(e: E) -> Int {\n  match e {\n    B(n) => n\n    C => 0\n  }\n}\n"},
		{name: "missing variant", src: "enum E { A, B, C }\nfn f(e: E) -> Int {\n  match e {\n    A => 1\n    B => 2\n  }\n}\n", want: []string{"XO0301"}},
		{name: "all variants", src: "enum E { A, B, C }\nfn f(e: E) -> Int {\n  match e {\n    A => 1\n    B | C => 2\n  }\n}\n"},
		{name: "nested missing", src: "fn f(x: Result[Int, Str]?) -> Int {\n  match x {\n    Some(Ok(v)) => v\n    None => 0\n  }\n}\n", want: []string{"XO0301"}},
		{name: "nested full", src: "fn f(x: Result[Int, Str]?) -> Int {\n  match x {\n    Some(Ok(v)) => v\n    Some(Err(_)) => -1\n    None => 0\n  }\n}\n"},
		{name: "bool", src: "fn f(b: Bool) -> Int {\n  match b {\n    true => 1\n  }\n}\n", want: []string{"XO0301"}},
		{name: "int needs wildcard", src: "fn f(n: Int) -> Str {\n  match n {\n    200..=299 => \"ok\"\n    429 | 503 => \"retry\"\n  }\n}\n", want: []string{"XO0301"}},
		{name: "int with wildcard", src: "fn f(n: Int) -> Str {\n  match n {\n    200..=299 => \"ok\"\n    429 | 503 => \"retry\"\n    _ => \"error\"\n  }\n}\n"},
		{name: "guard does not count", src: "fn f(o: Int?) -> Int {\n  match o {\n    Some(v) if v > 0 => v\n    None => 0\n  }\n}\n", want: []string{"XO0301"}},
		{name: "never field omitted", src: "use std/task\n\nfn f(clock: Clock) -> Int uses clock {\n  match task.with_timeout(clock, 1s, fn() { 1 }) {\n    Ok(v) => v\n    Err(task.Timed.TimedOut) => 0\n  }\n}\n"},
		{name: "never field required when inhabited", src: "use std/task\n\nenum E { Boom } derive Display\nfn g() -> Int ! E { 1 }\nfn f(clock: Clock) -> Int uses clock {\n  match task.with_timeout(clock, 1s, fn() { g() }) {\n    Ok(v) => v\n    Err(task.Timed.TimedOut) => 0\n  }\n}\n", want: []string{"XO0301"}},
		{name: "named fields and rest", src: "enum E { Db(msg: Str, code: Int), Other }\nfn f(e: E) -> Str {\n  match e {\n    Db(msg: m, ..) => m\n    Other => \"\"\n  }\n}\n"},
		{name: "missing field without rest", src: "enum E { Db(msg: Str, code: Int) }\nfn f(e: E) -> Str {\n  match e {\n    Db(msg: m) => m\n  }\n}\n", want: []string{"XO0302"}},
		{name: "wrong enum", src: "enum E { A }\nenum F { B }\nfn f(e: E) -> Int {\n  match e {\n    F.B => 1\n    _ => 0\n  }\n}\n", want: []string{"XO0302", "XO0305"}},
		{name: "lists", src: "fn f(xs: List[Int]) -> Int {\n  match xs {\n    [] => 0\n    [x] => x\n    [x, .., y] => x + y\n  }\n}\n"},
		{name: "lists missing", src: "fn f(xs: List[Int]) -> Int {\n  match xs {\n    [] => 0\n    [x, y, ..] => x + y\n  }\n}\n", want: []string{"XO0301"}},
		{name: "struct pattern", src: "struct P { x: Int, y: Int }\nfn f(p: P) -> Int {\n  match p {\n    P{x: 0, ..} => 0\n    P{x, y} => x + y\n  }\n}\n"},
		{name: "or pattern bindings", src: "fn f(r: Result[Int, Int]) -> Int {\n  match r {\n    Ok(v) | Err(v) => v\n  }\n}\n"},
		{name: "or pattern bindings differ", src: "fn f(r: Result[Int, Int]) -> Int {\n  match r {\n    Ok(v) | Err(_) => 1\n  }\n}\n", want: []string{"XO0304", "XO0110"}},
		{name: "refutable let", src: "fn f(o: Int?) -> Int {\n  let Some(v) = o\n  v\n}\n", want: []string{"XO0303"}},
		{name: "qualified std variant", src: "use std/http\n\nfn f(e: http.ClientErr) -> Int {\n  match e {\n    http.ClientErr.Timeout => 1\n    _ => 0\n  }\n}\n"},
		{name: "target os", src: "fn f() -> Str {\n  match target.os {\n    Linux => \"l\"\n    Darwin => \"d\"\n    Windows => \"w\"\n  }\n}\n", want: []string{"XO0301"}},
	})
}

func TestMutation(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "var ok", src: "fn f() -> List[Int] {\n  var xs = [1]\n  xs.push(2)\n  xs\n}\n"},
		{name: "push on let", src: "fn f() -> List[Int] {\n  let xs = [1]\n  xs.push(2)\n  xs\n}\n", want: []string{"XO0501"}},
		{name: "push on param", src: "fn f(xs: List[Int]) {\n  xs.push(2)\n}\n", want: []string{"XO0501"}},
		{name: "push on var param", src: "fn f(var xs: List[Int]) {\n  xs.push(2)\n}\n"},
		{name: "nested field of var", src: "struct S { users: Map[Str, Int] }\nfn f() -> S {\n  var st = S{users: {}}\n  st.users.put(\"a\", 1)\n  st\n}\n"},
		{name: "assign let", src: "fn f() -> Int {\n  let x = 1\n  x = 2\n  x\n}\n", want: []string{"XO0502"}},
		{name: "field assign let", src: "struct P { x: Int }\nfn f() -> P {\n  let p = P{x: 1}\n  p.x = 2\n  p\n}\n", want: []string{"XO0502"}},
		{name: "var receiver", src: "struct A { n: Int }\nfn (var a: A) inc() { a.n += 1 }\nfn f() -> A {\n  var a = A{n: 0}\n  a.inc()\n  a\n}\n"},
		{name: "var receiver on let", src: "struct A { n: Int }\nfn (var a: A) inc() { a.n += 1 }\nfn f() -> A {\n  let a = A{n: 0}\n  a.inc()\n  a\n}\n", want: []string{"XO0501"}},
		{name: "assign in non var receiver", src: "struct A { n: Int }\nfn (a: A) inc() { a.n += 1 }\n", want: []string{"XO0502"}},
		{name: "captured mutation", src: "fn f(xs: List[Int]) -> Int {\n  var total = 0\n  let _ys = xs.map(fn(x) {\n    total += x\n    x\n  })\n  total\n}\n", want: []string{"XO0502"}},
		{name: "mutex write", src: "fn f(m: Mutex[Map[Str, Int]]) {\n  m.with(fn(var mm) { mm.put(\"a\", 1) })\n}\n"},
		{name: "mutex read only view", src: "fn f(m: Mutex[Map[Str, Int]]) {\n  m.with(fn(mm) { mm.put(\"a\", 1) })\n}\n", want: []string{"XO0501"}},
		{name: "moved buffer", src: "fn f() -> List[Int] {\n  var b = Buf[Int].with_capacity(8)\n  b.push(1)\n  let xs = b.freeze()\n  b.push(2)\n  xs\n}\n", want: []string{"XO0510"}},
		{name: "buffer ok", src: "fn f() -> List[Int] {\n  var b = Buf[Int].new()\n  b.push(1)\n  b.push(2)\n  b.freeze()\n}\n"},
		{name: "buffer passed then used", src: "fn take(b: Buf[Int]) -> Int { b.len() }\nfn f() -> Int {\n  let b = Buf[Int].new()\n  let n = take(b)\n  n + b.len()\n}\n", want: []string{"XO0510"}},
		{name: "builder declared in loop body", src: "fn f(xs: List[Int]) -> Str {\n  var out = \"\"\n  for _x in xs {\n    var b = StrBuilder.new()\n    b.write(\"a\")\n    out = out + b.freeze()\n  }\n  out\n}\n"},
		{name: "builder let in loop body", src: "fn f(xs: List[Int]) -> Str {\n  var out = \"\"\n  for _x in xs {\n    let b = StrBuilder.new()\n    out = out + b.freeze()\n  }\n  out\n}\n"},
		{name: "buffer moved in loop", src: "fn take(b: StrBuilder) -> Str { b.freeze() }\nfn f(xs: List[Int]) {\n  let b = StrBuilder.new()\n  for _x in xs {\n    let _s = take(b)\n  }\n}\n", want: []string{"XO0510"}},
		{name: "bytes builder moved by freeze", src: "fn f() -> Bytes {\n  var b = BytesBuilder.new()\n  b.write_byte(1)\n  let out = b.freeze()\n  b.write(out)\n  out\n}\n", want: []string{"XO0510"}},
		{name: "buffer reassigned", src: "fn f() -> Str {\n  var b = StrBuilder.new()\n  let s = b.freeze()\n  b = StrBuilder.new()\n  b.write(s)\n  b.freeze()\n}\n"},
		{name: "lost update", src: `
struct Bucket { tokens: Int }
fn (var b: Bucket) take() -> Bool {
  b.tokens -= 1
  b.tokens >= 0
}
fn f(m: Map[Str, Bucket], k: Str) -> Bool {
  var b = m.get(k) ?? Bucket{tokens: 3}
  b.take()
}
`, want: []string{"XO0520"}},
		{name: "written back", src: `
struct Bucket { tokens: Int }
fn (var b: Bucket) take() -> Bool {
  b.tokens -= 1
  b.tokens >= 0
}
fn f(var m: Map[Str, Bucket], k: Str) -> Bool {
  var b = m.get(k) ?? Bucket{tokens: 3}
  let ok = b.take()
  m.put(k, b)
  ok
}
`},
		{name: "lost update by index", src: "struct P { x: Int }\nfn f(xs: List[P]) {\n  var p = xs[0]\n  p.x = 5\n}\n", want: []string{"XO0520"}},
		{name: "not a copy out", src: "struct P { x: Int }\nfn f() -> P {\n  var p = P{x: 1}\n  p.x = 5\n  p\n}\n"},
		{name: "not a copy out unused", src: "struct P { x: Int }\nfn f() {\n  var p = P{x: 1}\n  p.x = 5\n}\n", want: []string{"XO0110"}},
		{name: "read in defer counts", src: "struct P { x: Int }\nfn show(p: P) {}\nfn f(xs: List[P]) {\n  var p = xs[0]\n  defer show(p)\n  p.x = 5\n}\n"},
		{name: "update closure param not checked", src: "struct B { n: Int }\nfn f(var m: Map[Str, B]) -> Int {\n  m.update(\"k\", B{n: 0}, fn(var b) {\n    b.n += 1\n    b.n\n  })\n}\n"},
	})
}

func TestEffects(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "declared", src: "pub fn now(c: Clock) -> Time uses clock { c.now() }\n"},
		{name: "pub pure performs", src: "pub fn now(c: Clock) -> Time { c.now() }\n", want: []string{"XO0701"}},
		{name: "sub effect within parent", src: "pub fn load(f: Fs) -> Bytes ! FsErr uses fs { f.read(\"a\")? }\n"},
		{name: "parent not within sub", src: "pub fn load(f: Fs) -> () ! FsErr uses fs.read { f.write(\"a\", b\"\")? }\n", want: []string{"XO0701"}},
		{name: "inferred chain", src: "fn helper(c: Clock) -> Time { c.now() }\nfn middle(c: Clock) -> Time { helper(c) }\npub fn top(c: Clock) -> Time { middle(c) }\n", want: []string{"XO0701"}},
		{name: "inferred ok", src: "fn helper(c: Clock) -> Time { c.now() }\npub fn top(c: Clock) -> Time uses clock { helper(c) }\n"},
		{name: "effect variable", src: "pub fn apply[T, fx: Effects](f: fn() -> T uses fx) -> T uses fx { f() }\npub fn g(c: Clock) -> Time uses clock { apply(fn() { c.now() }) }\n"},
		{name: "effect variable not declared", src: "pub fn apply[T, fx: Effects](f: fn() -> T uses fx) -> T { f() }\n", want: []string{"XO0701"}},
		{name: "effect variable leaks", src: "pub fn apply[T, fx: Effects](f: fn() -> T uses fx) -> T uses fx { f() }\npub fn g(c: Clock) -> Time { apply(fn() { c.now() }) }\n", want: []string{"XO0701"}},
		{name: "closure into pure type", src: "fn run(f: fn() -> Int) -> Int { f() }\nfn g(c: Clock) -> Int { run(fn() { c.now().unix() }) }\n", want: []string{"XO0701"}},
		{name: "subsumption pure into effectful", src: "fn run(f: fn() -> Int uses clock) -> Int uses clock { f() }\npub fn g() -> Int uses clock { run(fn() { 1 }) }\n"},
		{name: "logging is ambient", src: "use std/log\n\npub fn g() { log.info(\"hi\") }\n"},
		{name: "mutex is pure", src: "pub fn g(m: Mutex[Int]) -> Int { m.with(fn(v) { v }) }\n"},
		{name: "unknown effect", src: "pub fn g() uses network {}\n", want: []string{"XO0702"}},
		{name: "contract must be pure", src: "fn ok(c: Clock) -> Bool { c.now().unix() > 0 }\nfn g(c: Clock) -> Int\n  requires ok(c)\n{\n  1\n}\n", want: []string{"XO0701"}},
		{name: "http handler effects at registration", src: "use std/http\n\npub fn routes(c: Clock) -> http.Router uses clock {\n  var r = http.router()\n  r.handle(\"GET /t\", fn(_req) { http.text(200, \"${c.now()}\") })\n  r\n}\n"},
		{name: "http handler effects undeclared", src: "use std/http\n\npub fn routes(c: Clock) -> http.Router {\n  var r = http.router()\n  r.handle(\"GET /t\", fn(_req) { http.text(200, \"${c.now()}\") })\n  r\n}\n", want: []string{"XO0701"}},
		{name: "interface impl effects", src: "struct Loud { c: Clock }\nfn (l: Loud) display() -> Str { \"${l.c.now()}\" }\nfn show(d: Display) -> Str { \"${d}\" }\nfn f(c: Clock) -> Str { show(Loud{c: c}) }\n", want: []string{"XO0701"}},
		{name: "test predeclared", src: "fn now(c: Clock) -> Time uses clock { c.now() }\ntest \"clock\" {\n  os.clock.advance(1s)\n  let t = now(os.clock)\n  expect(t) == Time.UNIX_EPOCH\n  expect(testing.gen[Int](1) > 0)\n}\n"},
		{name: "os outside test", src: "fn f() -> Time uses clock { os.clock.now() }\n", want: []string{"XO0601"}},
		{name: "select after needs clock", src: "pub fn f(ch: Chan[Int]) -> Int {\n  select {\n    v = ch.recv() => v ?? 0\n    after 1s => 0\n  }\n}\n", want: []string{"XO0701"}},
	})
}

func TestConcurrency(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "scope ok", src: "fn work(n: Int) -> Int { n * 2 }\nfn f() -> Int {\n  scope s {\n    let a = s.spawn(fn() { work(1) })\n    let b = s.spawn(fn() { work(2) })\n    a.await + b.await\n  }\n}\n"},
		{name: "spawn without scope", src: "fn work() {}\nfn f() {\n  spawn(fn() { work() })\n}\n", want: []string{"XO0803"}},
		{name: "spawn on non scope", src: "fn work() {}\nfn f(xs: List[Int]) {\n  xs.spawn(fn() { work() })\n}\n", want: []string{"XO0803"}},
		{name: "task error does not convert", src: "enum E { Boom }\nfn work() -> Int ! E { 1 }\nfn f() {\n  scope s {\n    s.spawn(fn() { work() })\n  }\n}\n", want: []string{"XO0801"}},
		{name: "task error converts", src: "enum E { Boom } derive Display\nfn work() -> Int ! E { 1 }\nfn f() ! {\n  scope s {\n    s.spawn(fn() { work() })\n  }\n}\n"},
		{name: "task error awaited", src: "enum E { Boom }\nfn work() -> Int ! E { 1 }\nfn f() -> Int {\n  scope s {\n    let t = s.spawn(fn() { work() })\n    match t.await {\n      Ok(v) => v\n      Err(_) => 0\n    }\n  }\n}\n"},
		{name: "scope handle escapes", src: "fn f() -> Int {\n  let h = scope s { s }\n  1\n}\n", want: []string{"XO0802", "XO0110"}},
		{name: "closure capturing the handle escapes", src: "fn work() {}\nfn f() -> fn() {\n  scope s {\n    fn() { let _ = s.spawn(fn() { work() }) }\n  }\n}\n", want: []string{"XO0802"}},
		{name: "closure capturing the handle stored outside", src: "fn work() {}\nfn f() {\n  var later = fn() {}\n  scope s {\n    let go_later = fn() { let _ = s.spawn(fn() { work() }) }\n    later = go_later\n  }\n  later()\n}\n", want: []string{"XO0802"}},
		{name: "closure capturing the handle used inside", src: "fn work() {}\nfn f() {\n  scope s {\n    let start = fn() { s.spawn(fn() { work() }) }\n    start()\n    start()\n  }\n}\n"},
		{name: "await with parens", src: "fn f() -> Int {\n  scope s {\n    let t = s.spawn(fn() { 1 })\n    t.await()\n  }\n}\n", want: []string{"XO0407"}},
	})
}

func TestHoles(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "hole", src: "fn f(x: Int) -> Int { ??? }\n", want: []string{"XO0900"}},
		{name: "no hole", src: "fn f(x: Int) -> Int { x }\n"},
	})
}

func TestUnhandledPositions(t *testing.T) {
	g := "enum E { A } derive Display\nfn g() -> Int ! E { 1 }\n"
	runCases(t, []ruleCase{
		{name: "if without else", src: g + "fn f(c: Bool) {\n  if c { g() }\n}\n", want: []string{"XO0201"}},
		{name: "match arm statement", src: g + "fn f(c: Bool) {\n  match c {\n    true => g()\n    false => ()\n  }\n}\n", want: []string{"XO0201"}},
		{name: "defer", src: g + "fn f() {\n  defer g()\n}\n", want: []string{"XO0201"}},
		{name: "loop body", src: g + "fn f(xs: List[Int]) {\n  for _x in xs {\n    g()\n  }\n}\n", want: []string{"XO0201"}},
		{name: "unit function tail", src: g + "fn f() {\n  g()\n}\n", want: []string{"XO0201"}},
		{name: "function value", src: g + "fn f(h: fn() -> Int ! E) {\n  h()\n}\n", want: []string{"XO0201"}},
		{name: "method", src: "enum E { A }\nstruct S { n: Int }\nfn (s: S) save() ! E {}\nfn f(s: S) {\n  s.save()\n}\n", want: []string{"XO0201"}},
		{name: "closure tail in unit closure", src: g + "fn run(f: fn()) { f() }\nfn f() {\n  run(fn() { g() })\n}\n", want: []string{"XO0201"}},
		{name: "collected into a list is ambiguous", src: g + "fn f(xs: List[Int]) -> List[Result[Int, E]] { xs.map(fn(_x) { g() }) }\n", want: []string{"XO0203"}},
		{name: "collected into a list", src: g + "fn f(xs: List[Int]) -> List[Result[Int, E]] { xs.map(fn(_x) { Ok(g()) }) }\n"},
		{name: "kept as a value", src: g + "fn f() -> Result[Int, E] { g() }\n"},
		{name: "non result discard ok", src: "fn g() -> Int { 1 }\nfn f() {\n  g()\n}\n"},
		{name: "bound to underscore name", src: g + "fn f() {\n  let _r = g()\n}\n", want: []string{"XO0201"}},
		{name: "bound and never used", src: g + "fn f() {\n  let r = g()\n}\n", want: []string{"XO0201"}},
		{name: "bound and matched", src: g + "fn f() -> Int {\n  let r = g()\n  match r {\n    Ok(v) => v\n    Err(_) => 0\n  }\n}\n"},
	})
}

func TestEdgeCases(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "pattern binding immutable", src: "fn f(o: Int?) -> Int {\n  match o {\n    Some(v) => {\n      v += 1\n      v\n    }\n    None => 0\n  }\n}\n", want: []string{"XO0502"}},
		{name: "for binding immutable", src: "fn f(xs: List[Int]) {\n  for x in xs {\n    x = 1\n  }\n}\n", want: []string{"XO0502", "XO0110"}},
		{name: "closure var param mutable", src: "fn f(m: Mutex[Int]) {\n  m.with(fn(var n) { n += 1 })\n}\n"},
		{name: "option equality with value", src: "fn f(o: Int?) -> Bool { o == 5 }\n"},
		{name: "option equality none", src: "fn f(o: Int?) -> Bool { o == None }\n"},
		{name: "interface elements", src: "fn f() -> List[Display] { [1, \"a\", 2.5] }\n"},
		{name: "interface elements fail", src: "struct P { x: Int }\nfn f() -> List[Display] { [1, P{x: 1}] }\n", want: []string{"XO0401"}},
		{name: "closure returned is not performed", src: "pub fn make(c: Clock) -> fn() -> Time uses clock { fn() { c.now() } }\n"},
		{name: "closure returned with wrong effects", src: "pub fn make(c: Clock) -> fn() -> Time { fn() { c.now() } }\n", want: []string{"XO0701"}},
		{name: "calling a returned closure", src: "pub fn make(c: Clock) -> fn() -> Time uses clock { fn() { c.now() } }\npub fn use_it(c: Clock) -> Time { make(c)() }\n", want: []string{"XO0701"}},
		{name: "recursion", src: "fn a(c: Clock, n: Int) -> Int { if n == 0 { c.now().unix() } else { b(c, n - 1) } }\nfn b(c: Clock, n: Int) -> Int { a(c, n) }\npub fn top(c: Clock) -> Int { b(c, 3) }\n", want: []string{"XO0701"}},
		{name: "field function call effects", src: "struct H { f: fn() -> Int uses net }\npub fn call(h: H) -> Int { h.f() }\n", want: []string{"XO0701"}},
		{name: "generic interface method", src: "interface Store[T] {\n  fn get(self, id: Int) -> T?\n}\nstruct Mem { xs: List[Str] }\nfn (m: Mem) get(id: Int) -> Str? { m.xs.get(id) }\nfn first(s: Store[Str]) -> Str { s.get(0) ?? \"\" }\nfn f() -> Str { first(Mem{xs: []}) }\n"},
		{name: "generic interface mismatch", src: "interface Store[T] {\n  fn get(self, id: Int) -> T?\n}\nstruct Mem { xs: List[Int] }\nfn (m: Mem) get(id: Int) -> Int? { m.xs.get(id) }\nfn first(s: Store[Str]) -> Str { s.get(0) ?? \"\" }\nfn f() -> Str { first(Mem{xs: []}) }\n", want: []string{"XO0401"}},
		{name: "type alias", src: "type UserId = Int\nfn f(id: UserId) -> Int { id + 1 }\n"},
		{name: "newtype distinct", src: "type Email struct(Str)\nfn send(e: Email) {}\nfn f() {\n  send(\"a@b\")\n}\n", want: []string{"XO0401"}},
		{name: "newtype construct", src: "type Email struct(Str)\nfn send(e: Email) {}\nfn f() {\n  send(Email(\"a@b\"))\n}\n"},
		{name: "labeled break", src: "fn f(xs: List[List[Int]]) {\n  outer: for ys in xs {\n    for y in ys {\n      if y == 0 { break outer }\n    }\n  }\n}\n"},
		{name: "unknown label", src: "fn f(xs: List[Int]) {\n  for _x in xs {\n    break outr\n  }\n}\n", want: []string{"XO0607"}},
		{name: "while never ends function", src: "fn f(c: Clock) -> Int {\n  while {\n    c.sleep(1s)\n  }\n}\n"},
		{name: "while with break is unit", src: "fn f() -> Int {\n  while {\n    break\n  }\n}\n", want: []string{"XO0401"}},
		{name: "map literal empty", src: "fn f() -> Map[Str, Int] {\n  var m = {}\n  m.put(\"a\", 1)\n  m\n}\n"},
		{name: "list inferred later", src: "fn f() -> List[Int] {\n  var xs = []\n  xs.push(1)\n  xs\n}\n"},
		{name: "sized int literal", src: "fn f() -> UInt8 {\n  let x: UInt8 = 200\n  x + 1\n}\n"},
		{name: "float literal to int", src: "fn f() -> Int { 1.5 }\n", want: []string{"XO0401"}},
		{name: "if let shorthand", src: "fn f(o: Str?) -> Str {\n  if let s = o { s } else { \"none\" }\n}\n"},
		{name: "struct literal in condition parenthesized", src: "struct P { x: Int }\nfn f(p: P) -> Bool {\n  if (p == P{x: 0}) { true } else { false }\n}\n"},
		{name: "ensures result old", src: "fn f(x: Int) -> Int\n  ensures result > old(x)\n{\n  x + 1\n}\n"},
		{name: "invariant", src: "struct A { balance: Int } invariant balance >= 0\n"},
		{name: "invariant wrong type", src: "struct A { balance: Int } invariant balance\n", want: []string{"XO0401"}},
		{name: "test property", src: "test \"reverse\" for xs: List[Int] {\n  expect(xs.reverse().reverse()) == xs\n}\n"},
		{name: "expect mismatch", src: "test \"t\" {\n  expect(1) == \"a\"\n}\n", want: []string{"XO0406"}},
		{name: "test file expect anywhere", files: map[string]string{
			"m/a.xo":      "fn double(x: Int) -> Int { x * 2 }\n",
			"m/a_test.xo": "fn check_double() {\n  expect(double(2)) == 4\n}\ntest \"d\" {\n  check_double()\n}\n",
		}},
		{name: "std error displays", src: "use std/http\n\nfn f(e: http.ClientErr) -> Str { \"${e}\" }\n"},
		{name: "json derive", src: "use std/json\n\nstruct U { id: Int, tags: List[Str], nick: Str? } derive Json\nfn f(u: U) -> Bytes { json.encode(u) }\n"},
		{name: "json missing derive", src: "use std/json\n\nstruct U { id: Int }\nfn f(u: U) -> Bytes { json.encode(u) }\n", want: []string{"XO0404"}},
		{name: "json derive field", src: "struct V { x: Int }\nstruct U { v: V } derive Json\n", want: []string{"XO0404"}},
		{name: "ord compare", src: "struct V { x: Int } derive Ord\nfn f(a: V, b: V) -> Bool { a < b }\n"},
		{name: "ord compare missing", src: "struct V { x: Int }\nfn f(a: V, b: V) -> Bool { a < b }\n", want: []string{"XO0404"}},
		{name: "min max", src: "fn f(a: Duration, b: Duration) -> Duration { a.min(b).max(1s) }\n"},
		{name: "generic method receiver", src: "struct Stack[T] { items: List[T] }\nfn (s: Stack[T]) top() -> T? { s.items.last() }\nfn f(s: Stack[Int]) -> Int { s.top() ?? 0 }\n"},
		{name: "generic method wrong result", src: "struct Stack[T] { items: List[T] }\nfn (s: Stack[T]) top() -> T? { s.items.last() }\nfn f(s: Stack[Int]) -> Str { s.top() ?? \"\" }\n", want: []string{"XO0401"}},
		{name: "closure error inferred", src: "enum E { A } derive Display\nfn g(x: Int) -> Int ! E { x }\nfn f(xs: List[Int]) -> List[Int] ! E { xs.map(fn(x) { g(x)? + 1 }) }\n"},
		{name: "closure error not allowed", src: "enum E { A } derive Display\nfn g(x: Int) -> Int ! E { x }\nfn run(f: fn(Int) -> Int) -> Int { f(1) }\nfn f() -> Int { run(fn(x) { g(x)? }) }\n", want: []string{"XO0205"}},
		{name: "var arg needs var", src: "fn bump(var n: Int) { n += 1 }\nfn f() -> Int {\n  let n = 1\n  bump(n)\n  n\n}\n", want: []string{"XO0502"}},
		{name: "var arg ok", src: "fn bump(var n: Int) { n += 1 }\nfn f() -> Int {\n  var n = 1\n  bump(n)\n  n\n}\n"},
		{name: "scope value", src: "fn f() -> Int {\n  let v = scope s {\n    let t = s.spawn(fn() { 2 })\n    t.await * 2\n  }\n  v\n}\n"},
		{name: "spawn error into closure error", src: "use std/task\n\nenum E { A } derive Display\nfn g() -> Int ! E { 1 }\nfn f(c: Clock) -> Int ! task.Timed[E] uses clock {\n  task.with_timeout(c, 1s, fn() {\n    scope s {\n      s.spawn(fn() { g() })\n      1\n    }\n  })\n}\n"},
		{name: "select", src: "fn f(a: Chan[Int], b: Chan[Str]) -> Str uses clock {\n  select {\n    x = a.recv() => \"${x ?? 0}\"\n    y = b.recv() => y ?? \"\"\n    after 5s => \"idle\"\n  }\n}\n"},
		{name: "chan send move", src: "fn f(ch: Chan[Buf[Int]]) -> Int {\n  let b = Buf[Int].new()\n  ch.send(b)\n  b.len()\n}\n", want: []string{"XO0510"}},
		{name: "spawn captures buffer", src: "fn f() -> Int {\n  let b = Buf[Int].new()\n  scope s {\n    s.spawn(fn() { b.len() })\n  }\n  b.len()\n}\n", want: []string{"XO0510"}},
		{name: "lost update in loop is written back", src: "struct P { x: Int }\nfn f(var xs: List[P]) {\n  for i in 0..xs.len() {\n    var p = xs[i]\n    p.x += 1\n    xs.set(i, p)\n  }\n}\n"},
		{name: "lost update in loop", src: "struct P { x: Int }\nfn f(xs: List[P]) {\n  for i in 0..xs.len() {\n    var p = xs[i]\n    p.x += 1\n  }\n}\n", want: []string{"XO0520"}},
		{name: "read later in loop", src: "struct P { x: Int }\nfn show(p: P) {}\nfn f(xs: List[P]) {\n  var p = xs[0]\n  for _i in 0..3 {\n    show(p)\n    p.x += 1\n  }\n}\n"},
	})
}

// TestNoCascade: one mistake gives one diagnostic; everything that depends
// on the bad expression is suppressed (core 10.2).
func TestNoCascade(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "unknown call used later", src: "fn f() -> Int {\n  let x = unknown_fn(1)\n  let y = x.field + 1\n  let z = y.method()\n  z * 2\n}\n", want: []string{"XO0601"}},
		{name: "unknown type in signature", src: "fn f(u: Usr) -> Str {\n  let n = u.name\n  \"${n}\"\n}\n", want: []string{"XO0601"}},
		{name: "mismatch then use", src: "fn f() -> Int {\n  let x: Int = \"a\"\n  x + 1\n}\n", want: []string{"XO0401"}},
		{name: "bad match subject", src: "enum E { A, B }\nfn f() -> Int {\n  match nope() {\n    A => 1\n  }\n}\n", want: []string{"XO0601"}},
		{name: "unknown field then interpolation", src: "struct P { x: Int }\nfn f(p: P) -> Str { \"${p.y}\" }\n", want: []string{"XO0601"}},
		{name: "bad argument does not block call", src: "fn g(a: Int, b: Str) -> Int { a }\nfn f() -> Int { g(nope, 1) }\n", want: []string{"XO0601", "XO0401"}},
		{name: "or_err copy is a lost update since 0.4", src: "struct U { name: Str }\nenum E { Missing }\nfn f(m: Map[Int, U], id: Int) -> () ! E {\n  var u = m.get(id).or_err(E.Missing)?\n  u.name = \"x\"\n}\n", want: []string{"XO0520"}},
	})
}

func TestRecursiveTypes(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "recursive struct", src: "struct Node { name: Str, children: List[Node] }\nfn count(n: Node) -> Int { n.children.fold(1, fn(acc, c) { acc + count(c) }) }\n"},
		{name: "recursive enum match", src: "enum Tree {\n  Leaf\n  Branch(l: Tree, r: Tree)\n}\nfn depth(t: Tree) -> Int {\n  match t {\n    Leaf => 0\n    Branch(l, r) => 1 + depth(l).max(depth(r))\n  }\n}\n"},
		{name: "recursive enum nested missing", src: "enum Tree {\n  Leaf\n  Branch(l: Tree, r: Tree)\n}\nfn f(t: Tree) -> Int {\n  match t {\n    Leaf => 0\n    Branch(Leaf, _) => 1\n  }\n}\n", want: []string{"XO0301"}},
		{name: "alias cycle", src: "type A = B\ntype B = A\nfn f(a: A) {}\n", want: []string{"XO0407"}},
		{name: "const cycle", src: "const A = B\nconst B = A\n", want: []string{"XO0407"}},
	})
}
