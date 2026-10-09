package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
)

// body parses src as the statements of a function body and renders them.
func body(t *testing.T, src string) string {
	t.Helper()
	f, ds := ParseFile("t.xo", []byte("fn t() {\n"+src+"\n}\n"), 0)
	if len(ds) > 0 {
		t.Fatalf("unexpected diagnostics for %q:\n%s", src, humans(ds))
	}
	b := f.Decls[0].(*ast.FnDecl).Body
	var ss []string
	for _, s := range b.Stmts {
		ss = append(ss, sexpr(s))
	}
	return strings.Join(ss, "; ")
}

func humans(ds []diag.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, diag.Human(d))
	}
	return strings.Join(out, "\n")
}

func TestExpressions(t *testing.T) {
	tests := []struct{ src, want string }{
		// precedence (core 15)
		{"a + b * c", "(+ a (* b c))"},
		{"a * b + c", "(+ (* a b) c)"},
		{"a | b & c", "(| a (& b c))"},
		{"a << 1 + 2", "(+ (<< a 1) 2)"},
		{"a == b && c != d || e", "(|| (&& (== a b) (!= c d)) e)"},
		{"a ?? b ?? c", "(?? a (?? b c))"},
		{"a ?? b || c", "(?? a (|| b c))"},
		{"-a.b", "(- (. a b))"},
		{"!f(x)?", "(! (? (call f x)))"},
		{"~x ^ y", "(^ (~ x) y)"},
		{"0..n + 1", "(.. 0 (+ n 1))"},
		{"x == 0..=9", "(== x (..= 0 9))"},
		{"(a + b) * c", "(* (paren (+ a b)) c)"},
		// literals
		{"()", "()"},
		{"(1, \"a\")", "(tuple 1 \"a\")"},
		{"(1,)", "(tuple 1)"},
		{"[1, 2, 3]", "[1 2 3]"},
		{"[]", "[]"},
		{"{\"a\": 1, \"b\": 2}", "(map (\"a\" 1) (\"b\" 2))"},
		{"{}", "(map)"},
		{"{1: User{id: 1}}", "(map (1 (struct User (id 1))))"},
		{"#{1, 2}", "#{1 2}"},
		{"#{}", "#{}"},
		{"???", "???"},
		{"b\"ok\"", "b\"ok\""},
		{"'a'", "'a'"},
		{"250ms", "250ms"},
		{"\"n = ${n + 1}!\"", "(str \"n = \" ${(+ n 1)} \"!\")"},
		{"\"${a ?? \"none\"}\"", "(str ${(?? a \"none\")})"},
		{"r\"raw ${x}\"", "\"raw ${x}\""},
		// postfix
		{"a.b.c(d, e: f)", "(call (. (. a b) c) d e: f)"},
		{"xs[i]", "(index xs i)"},
		{"xs[1..]", "(index xs (.. 1 _))"},
		{"xs[..n]", "(index xs (.. _ n))"},
		{"xs[a..b]", "(index xs (.. a b))"},
		{"parts[..parts.len() - 1]", "(index parts (.. _ (- (call (. parts len)) 1)))"},
		{"xs[..=3]", "(index xs (..= _ 3))"},
		{"m[Red]", "(index m Red)"},
		{"xs[0](1)", "(call (index xs 0) 1)"},
		{"fns[i](x)", "(call (index fns i) x)"},
		{"json.decode[User](b)", "(call (targs (. json decode) User) b)"},
		{"req.json[Map[Str, json.Patch[Int]]]()", "(call (targs (. req json) Map[Str, json.Patch[Int]]))"},
		{"f[(Int, Str), fn(Int) -> Str]()", "(call (targs f (Int, Str) <fn(Int) -> Str>))"},
		{"Mutex[Map[Str, Int]].new({})", "(call (. (targs Mutex Map[Str, Int]) new) (map))"},
		{"Chan[Job].new(cap: 64)", "(call (. (targs Chan Job) new) cap: 64)"},
		{"Job[Str]{id: 1, input: \"x\"}", "(struct (targs Job Str) (id 1) (input \"x\"))"},
		{"http.Request{method: \"GET\"}", "(struct (. http Request) (method \"GET\"))"},
		{"Config{}", "(struct Config)"},
		{"u with {name: \"b\"}", "(with u (name \"b\"))"},
		{"expect(p) == c with {n: 1}", "(== (call expect p) (with c (n 1)))"},
		{"a.await?", "(? (. a await))"},
		{"x?.y?", "(? (. (? x) y))"},
		{"r.err().or_fault(\"e\").display()", "(call (. (call (. (call (. r err)) or_fault) \"e\") display))"},
		{"LoadErr.NotFound(id: 42)", "(call (. LoadErr NotFound) id: 42)"},
		{"task.Timed.Failed(e)", "(call (. (. task Timed) Failed) e)"},
		// leading dot continuation (core 1.3)
		{"http.json(429, x)\n  .with_header(\"a\", \"b\")", "(call (. (call (. http json) 429 x) with_header) \"a\" \"b\")"},
		// closures
		{"fn(x) { x * 2 }", "(fn (x) {(* x 2)})"},
		{"fn(x: Int) -> Int { x }", "(fn (x: Int) -> Int {x})"},
		{"fn(var b, _k) { b }", "(fn (var b, _k) {b})"},
		{"fn() -> User ! LoadErr uses net { get()? }", "(fn () -> User ! LoadErr (uses net) {(? (call get))})"},
		{"fn() uses (net, clock) { 1 }", "(fn () (uses net clock) {1})"},
		{"fn() ! { 1 }", "(fn () ! _ {1})"},
		// control flow expressions
		{"if a { 1 } else if b { 2 } else { 3 }", "(if a {1} (if b {2} {3}))"},
		{"if let Some(e) = user.email { e } else { d }", "(iflet Some(e) (. user email) {e} {d})"},
		{"if let e = opt { e }", "(iflet e opt {e})"},
		{"if (p == Point{x: 0}) { 1 }", "(if (paren (== p (struct Point (x 0)))) {1})"},
		{"if x == Red { 1 }", "(if (== x Red) {1})"},
		{"match x { 1 => a\n _ => b }", "(match x (1 => a) (_ => b))"},
		{"match x {\n Some(v) if v > 0 => v\n Some(_) | None => 0\n}", "(match x (Some(v) if (> v 0) => v) ((| Some(_) None) => 0))"},
		{"match a {\n Null => u.n = None\n Value(n) => u.n = n\n X => return Err(e)\n Y => break\n Z => {}\n}",
			"(match a (Null => (= (. u n) None)) (Value(n) => (= (. u n) n)) (X => (return (call Err e))) (Y => (break)) (Z => (map)))"},
		{"match code {\n 200..=299 => \"ok\"\n 429 | 503 => \"later\"\n}", "(match code ((..= 200 299) => \"ok\") ((| 429 503) => \"later\"))"},
		{"match (a, b) {\n (true, None) => 1\n (_, Some(d)) if d.ok => 2\n}", "(match (tuple a b) ((tuple true None) => 1) ((tuple _ Some(d)) if (. d ok) => 2))"},
		{"scope s { s.spawn(fn() { f() }) }", "(scope s {(call (. s spawn) (fn () {(call f)}))})"},
		{"select {\n j = jobs.recv() => handle(j)\n out.send(r) => count += 1\n after 5s => log.warn(\"idle\")\n}",
			"(select (j = (call (. jobs recv)) => (call handle j)) ((call (. out send) r) => (+= count 1)) (after 5s => (call (. log warn) \"idle\")))"},
		{"select {\n after = ch.recv() => after\n}", "(select (after = (call (. ch recv)) => after))"},
		// `is` (core 3.5): precedence of ==, contextual
		{"r is Err(Timeout)", "(is r Err(Timeout))"},
		{"resp is Ok(_) && retries < 3", "(&& (is resp Ok(_)) (< retries 3))"},
		{"a + 1 is 2..=5", "(is (+ a 1) (..= 2 5))"},
		{"x is A | B || y", "(|| (is x (| A B)) y)"},
		{"!(x is None)", "(! (paren (is x None)))"},
		{"e is http.ClientErr.Timeout", "(is e http.ClientErr.Timeout)"},
		{"p is Point{x: 0, ..}", "(is p Point{x: 0, ..})"},
		{"is + 1", "(+ is 1)"},
		{"is.len()", "(call (. is len))"},
		// tuple fields (core 3.5)
		{"t.0", "(. t 0)"},
		{"t.0.1", "(. (. t 0) 1)"},
		{"pair().1.name", "(. (. (call pair) 1) name)"},
		// effect arguments (core 6.2, 15: TypeArg)
		{"Cache[Int, Item, net.dial].new()", "(call (. (targs Cache Int Item net.dial) new))"},
		{"Cache[Int, Item, (net, clock)]{a: 1}", "(struct (targs Cache Int Item (net, clock)) (a 1))"},
		{"run[Int, net](f)", "(call (targs run Int net) f)"},
		{"fns[i](x)", "(call (index fns i) x)"},
		{"{ let a = 1\n a }", "{(let a 1); a}"},
		{"{ outer: for x in xs { break outer } }", "{(outer: for x xs {(break outer)})}"},
		{"{ f(x) }", "{(call f x)}"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := body(t, "let v = "+tt.src); got != "(let v "+tt.want+")" {
				t.Errorf("got  %s\nwant (let v %s)", got, tt.want)
			}
		})
	}
}

func TestStatements(t *testing.T) {
	tests := []struct{ src, want string }{
		{"let x = 10", "(let x 10)"},
		{"let (a, b): (Int, Str) = pair", "(let (tuple a b) (Int, Str) pair)"},
		{"let _ = f()", "(let _ (call f))"},
		{"var total: Int = 0", "(var total Int 0)"},
		{"total += x", "(+= total x)"},
		{"st.config.port = 9090", "(= (. (. st config) port) 9090)"},
		{"xs[i] = x", "(= (index xs i) x)"},
		{"defer running.add(-1)", "(defer (call (. running add) (- 1)))"},
		{"defer { cleanup() }", "(defer {(call cleanup)})"},
		{"return", "(return)"},
		{"return Err(e)", "(return (call Err e))"},
		{"for (i, x) in xs.indexed() { f(i) }", "(for (tuple i x) (call (. xs indexed)) {(call f i)})"},
		{"for _ in 0..3 {}", "(for _ (.. 0 3) {})"},
		{"for x in xs { continue }", "(for x xs {(continue)})"},
		{"while cond { f() }", "(while cond {(call f)})"},
		{"while a < b && c { }", "(while (&& (< a b) c) {})"},
		{"while { break }", "(while _ {(break)})"},
		{"outer: while { break outer }", "(outer: while _ {(break outer)})"},
		{"expect(a) == b", "(== (call expect a) b)"},
		{"if a {\n x\n} else {\n y\n}", "(if a {x} {y})"},
		{"if r is Ok(_) { x }", "(if (is r Ok(_)) {x})"},
		{"if c is Red { x }", "(if (is c Red) {x})"},
		{"if (p is Point{x: 0, ..}) { x }", "(if (paren (is p Point{x: 0, ..})) {x})"},
		{"const LIMIT = 10\nlet y = LIMIT", "(const LIMIT 10); (let y LIMIT)"},
		{"const D: Duration = 5s", "(const D Duration 5s)"},
		{"total -= x", "(-= total x)"},
		{"total *= x", "(*= total x)"},
		{"total /= x", "(/= total x)"},
		{"total %= x", "(%= total x)"},
		{"pair.0 = 1", "(= (. pair 0) 1)"},
		{"st.counts.1 += 1", "(+= (. (. st counts) 1) 1)"},
		{"let f = fn(x: Int) -> Int uses (net, clock) { x }", "(let f (fn (x: Int) -> Int (uses net clock) {x}))"},
		{"let f = fn() uses net { g() }", "(let f (fn () (uses net) {(call g)}))"},
		{"let r = match x {\n A => 1\n}\nr", "(let r (match x (A => 1))); r"},
		{"a; b", "a; b"},
		{"let x = while { break }", "(let x (loop {(break)}))"},
		{"f(\n  a,\n  b,\n)", "(call f a b)"},
		{"let xs = [\n  1,\n  2,\n]", "(let xs [1 2])"},
		{"let m = {\n  \"a\": 1,\n}", "(let m (map (\"a\" 1)))"},
		{"let invalid = match r {\n Err(InvalidUrl(..)) => true\n _ => false\n}", "(let invalid (match r (Err(InvalidUrl(..)) => true) (_ => false)))"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := body(t, tt.src); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestPatterns(t *testing.T) {
	tests := []struct{ src, want string }{
		{"_", "_"},
		{"x", "x"},
		{"42", "42"},
		{"-1", "(- 1)"},
		{"\"GET\"", "\"GET\""},
		{"'a'", "'a'"},
		{"true", "true"},
		{"1..=9", "(..= 1 9)"},
		{"'a'..='z'", "(..= 'a' 'z')"},
		{"-5..0", "(.. (- 5) 0)"},
		{"A | B | C", "(| A B C)"},
		{"(a, b)", "(tuple a b)"},
		{"()", "(tuple)"},
		{"(a)", "a"},
		{"Some(p)", "Some(p)"},
		{"None", "None"},
		{"Timeout", "Timeout"},
		{"http.ClientErr.Canceled", "http.ClientErr.Canceled"},
		{"NotFound(p)", "NotFound(p)"},
		{"NotFound(id: p)", "NotFound(id: p)"},
		{"Db(msg: m, ..)", "Db(msg: m, ..)"},
		{"Variant(..)", "Variant(..)"},
		{"User{id: p, ..}", "User{id: p, ..}"},
		{"User{id}", "User{id}"},
		{"[]", "[]"},
		{"[first, ..]", "[first, ..]"},
		{"[.., last]", "[.., last]"},
		{"[a, .., z]", "[a, .., z]"},
		{"[src, dst]", "[src, dst]"},
		{"Some(Ok(v))", "Some(Ok(v))"},
		{"Err(LoadErr.NotFound(id))", "Err(LoadErr.NotFound(id))"},
		{"Some(1 | 2)", "Some((| 1 2))"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			got := body(t, "match v {\n"+tt.src+" => 0\n}")
			want := "(match v (" + tt.want + " => 0))"
			if got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}

func file(t *testing.T, src string, mode Mode) string {
	t.Helper()
	f, ds := ParseFile("t.xo", []byte(src), mode)
	if len(ds) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", humans(ds))
	}
	return sexpr(f)
}

func TestDeclarations(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"uses", "use std/http\nuse std/json as j\nuse acme/rate-limit\n", "(use std/http)\n(use std/json as j)\n(use acme/rate-limit)"},
		{"use go", "use go \"net/url\"\nuse go \"github.com/google/uuid\" as gu\nuse go\n", "(use go \"net/url\")\n(use go \"github.com/google/uuid\" as gu)\n(use go)"},
		{"fn", "fn f(a: Int, b: Str = \"x\") -> Int { a }", "(fn f (a: Int, b: Str = \"x\") -> Int {a})"},
		{"pub fn no result", "pub fn f() {}", "(fn pub f () {})"},
		{"main", "fn main(os: Os) ! {\n}", "(fn main (os: Os) ! _ {})"},
		{"receiver", "fn (var a: Account) deposit(amt: Money) requires amt > 0 {\n  a.balance += amt\n}",
			"(fn (recv var a Account) deposit (amt: Money) (requires (> amt 0)) {(+= (. a balance) amt)})"},
		{"type params and effects", "pub fn retry[T, E, fx: Effects](clock: Clock, f: fn() -> T ! E uses fx) -> T ! E\n  uses (clock, fx) { f() }",
			"(fn pub retry [T, E, fx: Effects] (clock: Clock, f: <fn() -> T ! E uses (fx)>) -> T ! E (uses clock fx) {(call f)})"},
		{"constraint", "fn largest[T: Ord](a: T, b: T) -> T { a }", "(fn largest [T: Ord] (a: T, b: T) -> T {a})"},
		{"clauses on separate lines", "fn f(id: Int) -> User ! LoadErr\n  uses net, clock\n  requires id > 0\n  requires id < 9 else LoadErr.Big\n  ensures result.id == id\n{\n  g()\n}",
			"(fn f (id: Int) -> User ! LoadErr (uses net clock) (requires (> id 0)) (requires (< id 9) else (. LoadErr Big)) (ensures (== (. result id) id)) {(call g)})"},
		{"requires else on next line", "fn f(n: Str) -> () ! E\n  requires n.len() > 0\n    else E.Empty\n{}",
			"(fn f (n: Str) -> () ! E (requires (> (call (. n len)) 0) else (. E Empty)) {})"},
		{"requires debug", "fn f(n: Int)\n  requires(debug) n > 0\n  ensures(debug) n > 0\n{}", "(fn f (n: Int) (requires(debug) (> n 0)) (ensures (> n 0)) {})"},
		{"multiline params", "fn f(\n  a: Int,\n  b: fn(T) -> R ! E uses fx,\n) -> Int { a }", "(fn f (a: Int, b: <fn(T) -> R ! E uses (fx)>) -> Int {a})"},
		{"fn type return with uses", "fn f(c: Clock) -> fn(Int) -> Int uses clock {\n  fn(x) { x }\n}", "(fn f (c: Clock) -> <fn(Int) -> Int uses (clock)> {(fn (x) {x})})"},
		{"fn type var param", "fn with[R](f: fn(var T) -> R) -> R { f() }", "(fn with [R] (f: <fn(var T) -> R>) -> R {(call f)})"},
		{"struct", "pub struct User {\n  pub id: Int\n  name: Str?\n  port: Int = 8080\n} derive Json, Ord",
			"(struct pub User [pub id: Int] [name: Str?] [port: Int = 8080] (derive Json Ord))"},
		{"struct commas", "struct Account { id: Int, balance: Money } invariant balance >= 0", "(struct Account [id: Int] [balance: Money] (invariant (>= balance 0)))"},
		{"struct generic", "struct Job[T] { id: Int, input: T }", "(struct Job [T] [id: Int] [input: T])"},
		{"struct multiline invariant", "struct P {\n  a: Int\n} invariant a >= 1 &&\n  a <= 9\n", "(struct P [a: Int] (invariant (&& (>= a 1) (<= a 9))))"},
		{"enum", "pub enum E {\n  NotFound(id: Int)\n  Load(from LoadErr)\n  Timeout\n}", "(enum E NotFound(id: Int) Load(from LoadErr) Timeout)"},
		{"enum commas generic derive", "enum Patch[T] { Absent, Null, Value(v: T) } derive Json", "(enum Patch [T] Absent Null Value(v: T) (derive Json))"},
		{"enum field named from", "enum E { Move(from: Str, to: Str) }", "(enum E Move(from: Str, to: Str))"},
		{"interface", "pub interface Store {\n  fn get(self, id: Int) -> User ! LoadErr uses net\n  fn put(self, u: User) -> () ! LoadErr uses net\n}",
			"(interface Store (fn get (self, id: Int) -> User ! LoadErr (uses net)) (fn put (self, u: User) -> () ! LoadErr (uses net)))"},
		{"interface one line", "interface Display { fn display(self) -> Str }", "(interface Display (fn display (self) -> Str))"},
		{"alias", "type UserId = Int", "(alias UserId Int)"},
		{"newtype", "type Email struct(Str)", "(newtype Email Str)"},
		{"fn type alias", "type Handler = fn(http.Request) -> http.Response ! Error uses (net, clock)", "(alias Handler <fn(http.Request) -> http.Response ! Error uses (net, clock)>)"},
		{"tuple and optional types", "type T = List[(Int, Str?)]?", "(alias T List[(Int, Str?)]?)"},
		{"unit and paren types", "type U = ()\ntype V = (fn() -> Int)?", "(alias U ())\n(alias V <fn() -> Int>?)"},
		{"const", "const MAX = 1000\npub const D: Config = Config{port: 8080}", "(const MAX 1000)\n(const D Config (struct Config (port 8080)))"},
		{"tests", "test \"a\" { expect(1) == 1 }\ntest \"b\" for xs: List[Int], n: Int seed 8812 uses net {\n}",
			"(test \"a\" {(== (call expect 1) 1)})\n(test \"b\" for (xs: List[Int], n: Int) seed 8812 (uses net) {})"},
		{"test clauses on following lines", "test \"b\" for n: Int\n  uses net\n{\n}", "(test \"b\" for (n: Int) (uses net) {})"},
		{"result type on the next line", "fn f(a: Int)\n  -> List[Int] uses fx\n{\n  [a]\n}", "(fn f (a: Int) -> List[Int] (uses fx) {[a]})"},
		{"qualified from", "enum ApiErr {\n  Client(from http.ClientErr)\n  Decode(from json.DecodeErr)\n}", "(enum ApiErr Client(from http.ClientErr) Decode(from json.DecodeErr))"},
		{"effect type arguments", "struct S { c: Cache[Int, Item, net.dial], d: Cache[Int, Item, (net, clock)], e: Cache[Int, Item, ()] }",
			"(struct S [c: Cache[Int, Item, net.dial]] [d: Cache[Int, Item, (net, clock)]] [e: Cache[Int, Item, ()]])"},
		{"comments everywhere", "// header\nfn f() { // trailing\n  // own line\n  g() // after\n}\n// footer\n", "(fn f () {(call g)})"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := file(t, tt.src, 0); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestSignatureMode(t *testing.T) {
	src := "fn (m: Mutex[T]) with[R](f: fn(var T) -> R) -> R\nfn (xs: List[T]) len() -> Int\n"
	if _, ds := ParseFile("t.xo", []byte(src), 0); len(ds) == 0 {
		t.Fatal("bodiless functions parsed without SignatureMode")
	}
	want := "(fn (recv m Mutex[T]) with [R] (f: <fn(var T) -> R>) -> R)\n(fn (recv xs List[T]) len () -> Int)"
	if got := file(t, src, SignatureMode); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestDocComments(t *testing.T) {
	src := `/// Returns the user.
/// Second line.
pub fn get() {}

/// Not attached: a blank line follows.

fn other() {}

struct S {
  /// The id.
  id: Int
}

enum E {
  /// First.
  A
  /// Second,
  /// two lines.
  B(from http.ClientErr)
}
`
	f, ds := ParseFile("t.xo", []byte(src), 0)
	if len(ds) > 0 {
		t.Fatal(humans(ds))
	}
	get := f.Decls[0].(*ast.FnDecl)
	if get.Doc == nil || get.Doc.Text != "Returns the user.\nSecond line." {
		t.Errorf("fn doc = %+v", get.Doc)
	}
	if other := f.Decls[1].(*ast.FnDecl); other.Doc != nil {
		t.Errorf("unexpected doc on other: %q", other.Doc.Text)
	}
	if d := f.Decls[2].(*ast.StructDecl).Fields[0].Doc; d == nil || d.Text != "The id." {
		t.Errorf("field doc = %+v", d)
	}
	if d := f.Decls[3].(*ast.EnumDecl).Variants[0].Doc; d == nil || d.Text != "First." {
		t.Errorf("variant doc = %+v", d)
	}
	if d := f.Decls[3].(*ast.EnumDecl).Variants[1].Doc; d == nil || d.Text != "Second,\ntwo lines." {
		t.Errorf("second variant doc = %+v", d)
	}
}

func TestSpans(t *testing.T) {
	src := "fn f() {\n  let x = a.b(c)\n}\n"
	f, _ := ParseFile("t.xo", []byte(src), 0)
	let := f.Decls[0].(*ast.FnDecl).Body.Stmts[0].(*ast.LetStmt)
	call := let.Value.(*ast.CallExpr)
	check := func(n ast.Node, want string) {
		t.Helper()
		sp := n.Span()
		if got := src[sp.Start.Offset:sp.End.Offset]; got != want {
			t.Errorf("%T span covers %q, want %q", n, got, want)
		}
	}
	check(let, "let x = a.b(c)")
	check(call, "a.b(c)")
	check(call.Fun, "a.b")
	check(call.Args[0], "c")
	check(f.Decls[0], "fn f() {\n  let x = a.b(c)\n}")

	// Interpolated expressions keep their real positions.
	src2 := "fn f() {\n  let s = \"x ${a + b} y\"\n}\n"
	f2, _ := ParseFile("t.xo", []byte(src2), 0)
	s := f2.Decls[0].(*ast.FnDecl).Body.Stmts[0].(*ast.LetStmt).Value.(*ast.StringLit)
	bin := s.Parts[1].Expr
	if sp := bin.Span(); src2[sp.Start.Offset:sp.End.Offset] != "a + b" || sp.Start.Line != 2 || sp.Start.Col != 16 {
		t.Errorf("interpolation span = %v", sp)
	}
}

// codes returns the diagnostic codes with their lines.
func codes(ds []diag.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, fmt.Sprintf("%s@%d", d.Code, d.Span.Line))
	}
	return strings.Join(out, " ")
}

func TestErrorRecovery(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // codes@line, in order
	}{
		{"missing operand then fine statements", "fn f() {\n  let x = 1 +\n}\nfn g() {}\n", "XO0160@3"},
		{"one error per statement", "fn f() {\n  let x = (1 2 3 4)\n  let y = [1 2]\n  ok()\n}\n", "XO0160@2 XO0160@3"},
		{"two statements on one line", "fn f() {\n  a b\n  c\n}\n", "XO0175@2"},
		{"error inside closure body recovers in place", "fn f() {\n  g(fn() {\n    let = 1\n    h()\n  })\n  let = 2\n}\n", "XO0165@3 XO0165@6"},
		{"missing trailing comma", "fn f() {\n  let xs = [\n    1,\n    2\n  ]\n}\n", "XO0161@4"},
		{"missing comma between lines", "fn f() {\n  g(\n    a\n    b,\n  )\n}\n", "XO0161@3"},
		{"else on next line", "fn f() {\n  if a {\n  }\n  else {\n  }\n}\n", "XO0162@4"},
		{"struct literal in condition", "fn f() {\n  if p == Point{x: 0} {\n    g()\n  }\n  let = 1\n}\n", "XO0163@2 XO0165@5"},
		{"empty struct literal in condition", "fn f() {\n  while p != Point{} {\n  }\n}\n", "XO0163@2"},
		{"unclosed brace", "fn f() {\n  g()\n", "XO0164@1"},
		{"bad pattern", "fn f() {\n  match x {\n    + => 1\n    A => 2\n    * => 3\n  }\n}\n", "XO0165@3 XO0165@5"},
		{"while cond as expression", "fn f() {\n  let x = while a { }\n}\n", "XO0166@2"},
		{"fn decl in block", "fn f() {\n  fn g() {}\n  struct X {}\n}\n", "XO0167@2 XO0167@3"},
		{"pub const in block", "fn f() {\n  pub const X = 1\n}\n", "XO0167@2"},
		{"bitwise compound assignment", "fn f() {\n  x &= 1\n  y <<= 2\n  z += 3\n}\n", "XO0160@2 XO0160@3"},
		{"is pattern binds a name", "fn f() {\n  let b = r is Ok(v)\n}\n", "XO0165@2"},
		{"is struct pattern in condition", "fn f() {\n  if p is Point{x: 0, ..} {\n    g()\n  }\n}\n", "XO0163@2"},
		{"interface method body", "interface I {\n  fn f(self) -> Int { 1 }\n}\n", "XO0160@2"},
		{"tuple field with prefix", "fn f() {\n  let a = t.0x1\n}\n", "XO0160@2"},
		{"statement at top level", "let x = 1\nfn f() {}\nx = 2\n", "XO0168@1 XO0168@3"},
		{"comma between arms", "fn f() {\n  match x {\n    A => 1,\n    B => 2\n  }\n}\n", "XO0169@3"},
		{"fn type effects need parens in alias", "type H = fn() uses a, b\n", "XO0170@1"},
		{"fn type effects need parens in params", "fn f(g: fn() uses net, clock, n: Int) {}\n", "XO0170@1"},
		{"rest not last", "fn f() {\n  match x {\n    A(.., b) => 1\n  }\n}\n", "XO0171@3"},
		{"missing body", "fn f() -> Int\nfn g() {}\n", "XO0172@1"},
		{"use after decl", "fn f() {}\nuse std/http\n", "XO0173@2"},
		{"malformed go import path", "use go \"\"\nuse go \"/abs\"\nuse go \"a${b}\"\n", "XO0158@1 XO0158@2 XO0158@3"},
		{"self outside interface", "fn f(self, x: Int) {}\n", "XO0174@1"},
		{"pub test", "pub test \"x\" {}\n", "XO0176@1"},
		{"bad declarations resync at column one", "struct S {\n  a Int\n  b: Str\n}\nfn f( {\n}\nfn g() {}\n", "XO0177@2 XO0160@5"},
		{"lexer and parser errors together", "fn f() {\n  let s = \"\\q\"\n  let t = @\n}\n", "XO0153@2 XO0150@3"},
		{"error in interpolation", "fn f() {\n  let s = \"${a +}\"\n}\n", "XO0160@2"},
		{"trailing tokens in interpolation", "fn f() {\n  let s = \"${a b}\"\n}\n", "XO0160@2"},
		{"bad type", "fn f(x: 1) {}\nfn g(y: Int) {}\n", "XO0160@1"},
		{"return as value", "fn f() {\n  let x = return\n}\n", "XO0160@2"},
		{"missing in", "fn f() {\n  for x xs {}\n  g()\n}\n", "XO0160@2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := ParseFile("t.xo", []byte(tt.src), 0)
			if got := codes(ds); got != tt.want {
				t.Errorf("got  %s\nwant %s\n%s", got, tt.want, humans(ds))
			}
		})
	}
}

func TestRecoveryKeepsGoodDeclarations(t *testing.T) {
	src := "fn a() {\n  let = \n}\nstruct B { x Int }\nfn c() { ok() }\nenum D { E }\n"
	f, ds := ParseFile("t.xo", []byte(src), 0)
	if len(ds) != 2 {
		t.Fatalf("want 2 errors, got:\n%s", humans(ds))
	}
	var kinds []string
	for _, d := range f.Decls {
		kinds = append(kinds, fmt.Sprintf("%T", d))
	}
	want := "[*ast.FnDecl *ast.StructDecl *ast.FnDecl *ast.EnumDecl]"
	if fmt.Sprint(kinds) != want {
		t.Errorf("decls = %v, want %v", kinds, want)
	}
	if c := f.Decls[2].(*ast.FnDecl); c.Name.Name != "c" || len(c.Body.Stmts) != 1 {
		t.Errorf("fn c not intact: %s", sexpr(c))
	}
}

func TestFixes(t *testing.T) {
	_, ds := ParseFile("t.xo", []byte("fn f() {\n  g(\n    a\n  )\n}\n"), 0)
	if len(ds) != 1 || len(ds[0].Fixes) != 1 {
		t.Fatalf("want one diagnostic with a fix, got %v", ds)
	}
	e := ds[0].Fixes[0].Edits[0]
	if e.Replace != "," || e.Span.Line != 3 || e.Span.Col != 6 {
		t.Errorf("fix edit = %+v, want insert `,` at 3:6", e)
	}

	_, ds = ParseFile("t.xo", []byte("fn f() {\n  if a == P{x: 1} {}\n}\n"), 0)
	if len(ds) != 1 || len(ds[0].Fixes) != 1 || len(ds[0].Fixes[0].Edits) != 2 {
		t.Fatalf("want parenthesize fix, got %v", ds)
	}
	ed := ds[0].Fixes[0].Edits
	if ed[0].Span.Col != 6 || ed[0].Replace != "(" || ed[1].Span.Col != 18 || ed[1].Replace != ")" {
		t.Errorf("edits = %+v", ed)
	}
}

func TestNoPanicOnTruncatedInput(t *testing.T) {
	srcs := []string{
		"fn f[T: Ord, fx: Effects](a: Int = 1) -> T ! E uses (a, b) requires x else y { match x { Some(a) if b => { c } _ => d } }",
		"struct S[T] { a: Int = 1 } invariant a > 0 derive Json\nenum E { A(from B), C(x: Int) }\ntest \"t\" for x: Int seed 1 uses net { let (a, b) = c }",
		"fn g() { select { a = b.recv() => c\n after 1s => d } scope s { x[1..] with {a: 1} } \"${a[\"b\"]}\" #{1} {1: 2} }",
	}
	for _, src := range srcs {
		for i := 0; i <= len(src); i++ {
			ParseFile("t.xo", []byte(src[:i]), 0)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add("fn main(os: Os) ! { let x = \"${y}\" }")
	f.Add("match x { A(..) | B => 1 }")
	f.Add("struct S { a: Int } derive Json")
	f.Fuzz(func(t *testing.T, src string) {
		ParseFile("f.xo", []byte(src), 0)
	})
}
