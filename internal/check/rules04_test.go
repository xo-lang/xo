package check

import (
	"strings"
	"testing"
)

// Rules added or changed in spec draft 0.4 (decisions 0028 to 0035). Each
// rule has positive cases (the program breaks the rule and gets the code)
// and negative cases (a close program that is accepted).

func TestDiscardNeedsReason(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "let underscore on a call", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  let _ = g()\n}\n", want: []string{"XO0206"}},
		{name: "let underscore on a binding", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  let r = g()\n  let _ = r\n}\n", want: []string{"XO0206"}},
		{name: "let underscore with a type", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  let _: Result[Int, E] = g()\n}\n", want: []string{"XO0206"}},
		{name: "let underscore on unit result", src: "fn f(fs: Fs) uses fs.write {\n  let _ = fs.remove(\"tmp\")\n}\n", want: []string{"XO0206"}},
		{name: "ignore_err with a reason", src: "fn f(fs: Fs) uses fs.write {\n  let _ = fs.remove(\"tmp\").ignore_err(\"best effort temp file cleanup\")\n}\n"},
		{name: "ignore_err as a statement", src: "fn f(fs: Fs) uses fs.write {\n  fs.remove(\"tmp\").ignore_err(\"best effort\")\n}\n"},
		{name: "ignore_err yields an optional", src: "enum E { A }\nfn g() -> Str ! E { \"n\" }\nfn f() -> Str {\n  g().ignore_err(\"nickname is optional\") ?? \"\"\n}\n"},
		{name: "ignore_err needs a reason", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() {\n  g().ignore_err()\n}\n", want: []string{"XO0403"}},
		{name: "let underscore on an optional", src: "fn f(m: Map[Str, Int]) {\n  let _ = m.get(\"a\")\n}\n"},
		{name: "let underscore on a plain value", src: "fn f() {\n  let _ = 2\n}\n"},
		{name: "tuple pattern with underscore is not covered", src: "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f() -> Int {\n  let (_, n) = (g(), 2)\n  n\n}\n"},
	})
}

func TestDiscardNeedsReasonFix(t *testing.T) {
	ds := checkSrc(t, "enum E { A }\nfn g() -> Int ! E { 1 }\nfn f(c: Bool) {\n  let _ = g()\n  let _ = if c { g() } else { g() }\n}\n")
	if len(ds) != 2 {
		t.Fatalf("want 2 diagnostics, got:\n%s", humanAll(ds))
	}
	want := []string{
		`4:14 .ignore_err("TODO: reason")`,
		`5:11 (|5:36 ).ignore_err("TODO: reason")`,
	}
	for i, d := range ds {
		if d.Code != "XO0206" || len(d.Fixes) != 1 {
			t.Fatalf("diagnostic %d: %s", i, humanAll(ds))
		}
		var parts []string
		for _, e := range d.Fixes[0].Edits {
			parts = append(parts, strings.TrimSpace(strings.Join([]string{itoa(e.Span.Line) + ":" + itoa(e.Span.Col), e.Replace}, " ")))
		}
		if got := strings.Join(parts, "|"); got != want[i] {
			t.Errorf("fix %d = %q, want %q", i, got, want[i])
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestIsPattern(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "is in expect", src: "enum Err1 { Timeout, Other }\nfn g() -> Int ! Err1 { 1 }\ntest \"t\" {\n  expect(g() is Err(Timeout))\n}\n"},
		{name: "is in a condition", src: "fn f(resp: Result[Int, Str], retries: Int) -> Bool {\n  if resp is Ok(_) && retries < 3 { true } else { false }\n}\n"},
		{name: "is with range and or", src: "fn f(n: Int, c: Rune) -> Bool { n is 200..=299 | 304 && c is 'a'..='z' }\n"},
		{name: "is with qualified variant", src: "use std/http\n\nfn f(e: http.ClientErr) -> Bool { e is http.ClientErr.Timeout }\n"},
		{name: "is type is Bool", src: "fn f(o: Int?) -> Int { o is Some(_) }\n", want: []string{"XO0401"}},
		{name: "is pattern does not fit", src: "fn f(o: Int?) -> Bool { o is Ok(_) }\n", want: []string{"XO0302"}},
		{name: "is binds nothing", src: "fn f(o: Int?) -> Bool { o is Some(v) }\n", want: []string{"XO0165"}},
		{name: "is on a struct pattern", src: "struct P { x: Int, y: Int }\nfn f(p: P) -> Bool { p is P{x: 0, ..} }\n"},
		{name: "is word is still a name", src: "fn f(is: Int) -> Int { is + 1 }\n"},
	})
}

func TestTupleFields(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "read", src: "fn f(p: (Int, Str)) -> Str { \"${p.0}${p.1}\" }\n"},
		{name: "nested", src: "fn f(p: (Int, (Str, Bool))) -> Bool { p.1.1 }\n"},
		{name: "type", src: "fn f(p: (Int, Str)) -> Int { p.1 }\n", want: []string{"XO0401"}},
		{name: "out of range", src: "fn f(p: (Int, Str)) -> Int { p.2 }\n", want: []string{"XO0601"}},
		{name: "not a tuple", src: "struct P { x: Int }\nfn f(p: P) -> Int { p.0 }\n", want: []string{"XO0601"}},
		{name: "assign on var", src: "fn f() -> (Int, Str) {\n  var p = (1, \"a\")\n  p.0 = 2\n  p.0 += 1\n  p\n}\n"},
		{name: "assign on let", src: "fn f() -> (Int, Str) {\n  let p = (1, \"a\")\n  p.0 = 2\n  p\n}\n", want: []string{"XO0502"}},
		{name: "call a tuple field", src: "fn f(p: (fn(Int) -> Int, Int)) -> Int { p.0(p.1) }\n"},
		{name: "indexed entries", src: "fn f(m: Map[Str, Int]) -> Int { m.entries().fold(0, fn(acc, e) { acc + e.1 }) }\n"},
	})
}

func TestLocalConst(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "local const", src: "fn f(n: Int) -> Int {\n  const LIMIT = 10\n  const D: Duration = 5s\n  n.min(LIMIT) + D.millis()\n}\n"},
		{name: "local const from local const", src: "fn f() -> Int {\n  const A = 2\n  const B = A * 3\n  B\n}\n"},
		{name: "local const from top level", src: "const BASE = 4\nfn f() -> List[Int] {\n  const XS = [BASE, 5]\n  XS\n}\n"},
		{name: "not a compile time value", src: "fn f(n: Int) -> Int {\n  const LIMIT = n\n  LIMIT\n}\n", want: []string{"XO0408"}},
		{name: "not a compile time call", src: "fn g() -> Int { 1 }\nfn f() -> Int {\n  const LIMIT = g()\n  LIMIT\n}\n", want: []string{"XO0408"}},
		{name: "naming", src: "fn f() -> Int {\n  const limit = 1\n  limit\n}\n", want: []string{"XO0101"}},
		{name: "unused", src: "fn f() {\n  const LIMIT = 1\n}\n", want: []string{"XO0110"}},
		{name: "immutable", src: "fn f() -> Int {\n  const LIMIT = 1\n  LIMIT = 2\n  LIMIT\n}\n", want: []string{"XO0502"}},
		{name: "shadowing in the same block", src: "fn f() -> Int {\n  let a = 1\n  const a = 2\n  a\n}\n", want: []string{"XO0603", "XO0101"}},
		{name: "spec 3.1 block", src: "struct Config { port: Int }\nfn f() -> Int {\n  let x = 10\n  var total = 0\n  total += x\n  const MAX_USERS = 1000\n  const DEFAULT = Config{port: 8080}\n  total + MAX_USERS + DEFAULT.port\n}\n"},
	})
}

func TestCompoundAssignment(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "all five", src: "fn f() -> Int {\n  var n = 100\n  n += 1\n  n -= 2\n  n *= 3\n  n /= 4\n  n %= 5\n  n\n}\n"},
		{name: "duration", src: "fn f() -> Duration {\n  var d = 1s\n  d *= 3\n  d -= 500ms\n  d /= 2\n  d\n}\n"},
		{name: "field", src: "struct S { n: Float }\nfn f() -> S {\n  var s = S{n: 1.0}\n  s.n *= 2.0\n  s\n}\n"},
		{name: "on let", src: "fn f() -> Int {\n  let n = 1\n  n *= 2\n  n\n}\n", want: []string{"XO0502"}},
		{name: "wrong type", src: "fn f() -> Str {\n  var s = \"a\"\n  s -= \"b\"\n  s\n}\n", want: []string{"XO0406"}},
	})
}

func TestDurationTimesInt(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "d * n", src: "fn f(n: Int) -> Duration { 1s * n }\n"},
		{name: "n * d", src: "fn f(n: Int) -> Duration { n * 1s }\n"},
		{name: "literal * d", src: "fn f(d: Duration) -> Duration { 2 * d }\n"},
		{name: "n * d is not Int", src: "fn f(n: Int) -> Int { n * 1s }\n", want: []string{"XO0401"}},
		{name: "float * d", src: "fn f(x: Float) -> Duration { x * 1s }\n", want: []string{"XO0406"}},
		{name: "n / d", src: "fn f(n: Int) -> Duration { n / 1s }\n", want: []string{"XO0406"}},
	})
}

func TestLostUpdateWidened(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "or_err copy", src: "struct U { name: Str }\nenum E { Missing }\nfn f(m: Map[Int, U], id: Int) -> () ! E {\n  var u = m.get(id).or_err(E.Missing)?\n  u.name = \"x\"\n}\n", want: []string{"XO0520"}},
		{name: "or_err copy written back", src: "struct U { name: Str }\nenum E { Missing }\nfn f(var m: Map[Int, U], id: Int) -> () ! E {\n  var u = m.get(id).or_err(E.Missing)?\n  u.name = \"x\"\n  m.put(id, u)\n}\n"},
		{name: "mutated after push", src: `
struct Failure { id: Int, err: Str }
fn f(ids: List[Int]) -> List[Failure] {
  var failures = []
  for id in ids {
    var fail = Failure{id: id, err: "failed"}
    failures.push(fail)
    fail.err = "timed out"
  }
  failures
}
`, want: []string{"XO0520"}},
		{name: "mutated before push", src: `
struct Failure { id: Int, err: Str }
fn f(ids: List[Int]) -> List[Failure] {
  var failures = []
  for id in ids {
    var fail = Failure{id: id, err: "failed"}
    fail.err = "timed out"
    failures.push(fail)
  }
  failures
}
`},
		{name: "mutated after put", src: "struct B { n: Int }\nfn f(var m: Map[Str, B]) {\n  var b = B{n: 1}\n  m.put(\"a\", b)\n  b.n += 1\n}\n", want: []string{"XO0520"}},
		{name: "mutated after index store", src: "struct B { n: Int }\nfn f(var m: Map[Str, B]) {\n  var b = B{n: 1}\n  m[\"a\"] = b\n  b.n += 1\n}\n", want: []string{"XO0520"}},
		{name: "var method after push", src: "fn f(var xs: List[List[Int]]) {\n  var row = [1]\n  xs.push(row)\n  row.push(2)\n}\n", want: []string{"XO0520"}},
		{name: "stored again", src: "struct B { n: Int }\nfn f(var m: Map[Str, B]) {\n  var b = B{n: 1}\n  m.put(\"a\", b)\n  b.n += 1\n  m.put(\"a\", b)\n}\n"},
		{name: "reset after store", src: `
fn f(lines: List[Str]) -> List[List[Str]] {
  var groups = []
  var cur = []
  for ln in lines {
    if ln == "" {
      groups.push(cur)
      cur = []
    } else {
      cur.push(ln)
    }
  }
  groups.push(cur)
  groups
}
`},
		{name: "snapshot per iteration", src: "struct S { n: Int }\nfn f(xs: List[Int]) -> List[S] {\n  var out = []\n  var s = S{n: 0}\n  for _x in xs {\n    out.push(s)\n    s.n += 1\n  }\n  out\n}\n"},
		{name: "stored in defer", src: "struct S { n: Int }\nfn f(var out: List[S]) {\n  var s = S{n: 0}\n  out.push(s)\n  defer out.push(s)\n  s.n += 1\n}\n"},
		{name: "mutated after reset only", src: "struct S { n: Int }\nfn f(var out: List[S]) -> S {\n  var s = S{n: 0}\n  out.push(s)\n  s = S{n: 5}\n  s.n += 1\n  s\n}\n"},
		{name: "parameter not checked", src: "struct S { n: Int }\nfn f(var out: List[S], var s: S) {\n  out.push(s)\n  s.n += 1\n}\n"},
	})
}

func TestStd04(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "try_send and try_recv", src: "fn f(ch: Chan[Int]) -> Int {\n  if ch.try_send(1) { 1 } else { ch.try_recv() ?? 0 }\n}\n"},
		{name: "set api", src: "fn f(s: Set[Int]) -> Set[Str] {\n  if s.is_empty() || s.any(fn(x) { x > 3 }) || s.all(fn(x) { x > 0 }) {\n    return #{}\n  }\n  s.filter(fn(x) { x % 2 == 0 }).map(fn(x) { \"${x}\" })\n}\n"},
		{name: "set map with failing callback", src: "enum E { Bad }\nfn g(x: Int) -> Str ! E { \"${x}\" }\nfn f(s: Set[Int]) -> Set[Str] ! E { s.map(fn(x) { g(x) })? }\n"},
		{name: "map_concurrent", src: "use std/task\n\nenum E { Bad }\nfn fetch(n: Int) -> Str ! E { \"${n}\" }\nfn f(ids: List[Int]) -> List[Str] {\n  let rs = task.map_concurrent(ids, 4, fn(id) { fetch(id) })\n  rs.map(fn(r) { r.ok() ?? \"\" })\n}\n"},
		{name: "http stream", src: "use std/http\n\nfn export(rows: List[Str]) -> http.Response {\n  http.stream(200, fn(w) {\n    for r in rows {\n      w.write(r.bytes())?\n    }\n    w.flush()\n  })\n}\n"},
		{name: "http stream effects", src: "use std/http\n\npub fn export(c: Clock) -> http.Response {\n  http.stream(200, fn(w) { w.write(\"${c.now()}\".bytes()) })\n}\n", want: []string{"XO0701"}},
		{name: "write_only", src: "fn f(fs: Fs) -> Fs { fs.sub(\"/out\").write_only() }\ntest \"t\" {\n  let w = os.fs.write_only()\n  expect(w.case_sensitive())\n}\n"},
		{name: "fake net failures", src: "use std/http\n\ntest \"t\" {\n  os.net.fail(\"GET https://a.test/x\", http.ClientErr.Timeout)\n  os.net.fail_next(\"GET https://a.test/y\", http.ClientErr.Timeout)\n  os.net.clear_failures()\n  expect(os.net.requests().len()) == 0\n}\n"},
		{name: "run_main", src: "fn main(os: Os) ! {\n  os.stdio.println(\"hi\")\n}\ntest \"main\" {\n  expect(testing.run_main(main, os)) == 0\n  expect(testing.run_main(main, os, args: [\"-v\"])) == 0\n}\n"},
		{name: "testing enum qualified", src: "test \"t\" {\n  os.fs.fail_next(testing.FsOp.Rename, \"a\", FsErr.Denied(path: \"a\"))\n  os.fs.fail_next(Write, \"b\", FsErr.Denied(path: \"b\"))\n}\n"},
		{name: "semaphore handle", src: "use std/task\n\nstruct Pool { sem: task.Semaphore }\nfn f(p: Pool) -> Int { p.sem.with(fn() { 1 }) }\nfn g() -> Int { f(Pool{sem: task.semaphore(2)}) }\n"},
	})
}

func TestTaskGroups(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "group from scope", src: "use std/task\n\nfn work() {}\nfn f() {\n  scope s {\n    let g: task.Group = s.group()\n    g.spawn(fn() { work() })\n    g.spawn_background(fn() { work() })\n  }\n}\n"},
		{name: "os tasks in main", src: "fn refresh(c: Clock) { c.sleep(1s) }\nfn main(os: Os) ! {\n  os.tasks.spawn_background(fn() { refresh(os.clock) })\n}\n"},
		{name: "os tasks in a test", src: "test \"t\" {\n  let t = os.tasks.spawn(fn() { 1 })\n  expect(t.await) == 1\n}\n"},
		{name: "group task error converts", src: "enum E { Boom } derive Display\nfn work() -> Int ! E { 1 }\nfn f() ! {\n  scope s {\n    let g = s.group()\n    g.spawn(fn() { work() })\n  }\n}\n"},
		{name: "group task error does not convert", src: "enum E { Boom }\nfn work() -> Int ! E { 1 }\nfn f() {\n  scope s {\n    let g = s.group()\n    g.spawn(fn() { work() })\n  }\n}\n", want: []string{"XO0801"}},
		{name: "group task error awaited", src: "enum E { Boom }\nfn work() -> Int ! E { 1 }\nfn f() -> Int {\n  scope s {\n    let g = s.group()\n    let t = g.spawn(fn() { work() })\n    t.await.ok() ?? 0\n  }\n}\n"},
		{name: "os tasks error converts in main", src: "enum E { Boom } derive Display\nfn work() ! E {}\nfn main(os: Os) ! {\n  os.tasks.spawn_background(fn() { work() })\n}\n"},
		{name: "os tasks error does not convert in main", src: "enum E { Boom }\nfn work() ! E {}\nfn main(os: Os) ! {\n  os.tasks.spawn_background(fn() { work() })\n}\n", want: []string{"XO0801"}},
		{name: "stored group task cannot fail", src: "use std/task\n\nenum E { Boom } derive Display\nfn work() ! E {}\nstruct Svc { tasks: task.Group }\nfn (s: Svc) kick() {\n  s.tasks.spawn_background(fn() { work() })\n}\n", want: []string{"XO0801"}},
		{name: "stored group task handles its error", src: "use std/task\nuse std/log\n\nenum E { Boom } derive Display\nfn work() ! E {}\nstruct Svc { tasks: task.Group }\nfn (s: Svc) kick() {\n  s.tasks.spawn_background(fn() {\n    match work() {\n      Ok(_) => ()\n      Err(e) => log.warn(\"work failed\", {\"err\": e})\n    }\n  })\n}\n"},
		{name: "stored group task awaited", src: "use std/task\n\nenum E { Boom }\nfn work() -> Int ! E { 1 }\nfn run(g: task.Group) -> Int {\n  let t = g.spawn(fn() { work() })\n  t.await.ok() ?? 0\n}\n"},
		{name: "group effects", src: "use std/task\n\npub fn kick(g: task.Group, c: Clock) {\n  g.spawn_background(fn() { c.sleep(1s) })\n}\n", want: []string{"XO0701"}},
		{name: "struct with group escapes as scope value", src: "use std/task\n\nstruct Cache { tasks: task.Group }\nfn f() -> Cache {\n  scope s {\n    Cache{tasks: s.group()}\n  }\n}\n", want: []string{"XO0802"}},
		{name: "group escapes through a binding", src: "use std/task\n\nstruct Cache { tasks: task.Group }\nfn f() -> Cache {\n  scope s {\n    let g = s.group()\n    let c = Cache{tasks: g}\n    c\n  }\n}\n", want: []string{"XO0802"}},
		{name: "group escapes by return", src: "use std/task\n\nfn f() -> task.Group {\n  scope s {\n    return s.group()\n  }\n}\n", want: []string{"XO0802"}},
		{name: "group stored outside", src: "use std/task\n\nfn f(g0: task.Group) {\n  var keep = g0\n  scope s {\n    keep = s.group()\n  }\n  keep.spawn(fn() { 1 })\n}\n", want: []string{"XO0802"}},
		{name: "group pushed outside", src: "use std/task\n\nfn f() -> List[task.Group] {\n  var gs = []\n  scope s {\n    gs.push(s.group())\n  }\n  gs\n}\n", want: []string{"XO0802"}},
		{name: "group sent on a channel", src: "use std/task\n\nfn f(ch: Chan[task.Group]) {\n  scope s {\n    ch.send(s.group())\n  }\n}\n", want: []string{"XO0802"}},
		{name: "closure over a group escapes", src: "fn work() {}\nfn f() -> fn() {\n  scope s {\n    let g = s.group()\n    fn() { g.spawn_background(fn() { work() }) }\n  }\n}\n", want: []string{"XO0802"}},
		{name: "group used inside", src: "use std/task\n\nstruct Cache { tasks: task.Group }\nfn work() {}\nfn (c: Cache) kick() { c.tasks.spawn_background(fn() { work() }) }\nfn f() -> Int {\n  scope s {\n    let c = Cache{tasks: s.group()}\n    c.kick()\n    1\n  }\n}\n"},
		{name: "os tasks may be stored", src: "use std/task\n\nstruct Cache { tasks: task.Group }\nfn main(os: Os) ! {\n  let c = scope s {\n    Cache{tasks: os.tasks}\n  }\n  let _ = c\n}\n"},
	})
}

// elisionBase is a generic cache with an effect parameter, as in the
// harder tasks 06 and 07 (decision 0033).
const elisionBase = `use std/http

pub enum UpstreamErr {
  Failed(msg: Str)
} derive Display

pub struct Cache[K, V, fx: Effects] {
  load: fn(K) -> V ! UpstreamErr uses fx
  items: Mutex[Map[K, V]]
}

pub fn new_cache[K, V, fx: Effects](load: fn(K) -> V ! UpstreamErr uses fx) -> Cache[K, V, fx] {
  Cache{load: load, items: Mutex.new({})}
}

pub fn (c: Cache[K, V, fx]) get(k: K) -> V ! UpstreamErr uses fx {
  if let Some(v) = c.items.with(fn(m) { m.get(k) }) {
    return v
  }
  let v = (c.load)(k)?
  c.items.with(fn(var m) { m.put(k, v) })
  v
}

fn fetch(client: http.Client, id: Int) -> Str ! UpstreamErr uses net.dial {
  let resp = client.get("https://up.test/items/${id}").map_err(fn(e) { UpstreamErr.Failed(msg: "${e}") })?
  resp.body.to_str() ?? ""
}
`

func TestEffectElision(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "explicit effect argument", src: elisionBase + "pub fn warm(c: Cache[Int, Str, net.dial]) -> Str ! UpstreamErr uses net.dial { c.get(1)? }\n"},
		{name: "explicit effect argument in an expression", src: elisionBase + "fn make(client: http.Client) -> Int {\n  let c = Cache[Int, Str, net.dial]{load: fn(id) { fetch(client, id) }, items: Mutex.new({})}\n  1\n}\n", want: []string{"XO0110"}},
		{name: "signature elision is an effect variable", src: elisionBase + "pub fn warm(c: Cache[Int, Str]) -> Str ! UpstreamErr { c.get(1)? }\n"},
		{name: "signature elision passes effects to the caller", src: elisionBase + "pub fn warm(c: Cache[Int, Str]) -> Str ! UpstreamErr { c.get(1)? }\npub fn top(client: http.Client) -> Str ! UpstreamErr {\n  warm(new_cache(fn(id) { fetch(client, id) }))\n}\n", want: []string{"XO0701"}},
		{name: "signature elision passes effects declared", src: elisionBase + "pub fn warm(c: Cache[Int, Str]) -> Str ! UpstreamErr { c.get(1)? }\npub fn top(client: http.Client) -> Str ! UpstreamErr uses net.dial {\n  warm(new_cache(fn(id) { fetch(client, id) }))\n}\n"},
		{name: "local elision is inferred", src: elisionBase + "pub fn top(client: http.Client) -> Str ! UpstreamErr uses net.dial {\n  let c: Cache[Int, Str] = new_cache(fn(id) { fetch(client, id) })\n  c.get(1)?\n}\n"},
		{name: "local elision is inferred and checked", src: elisionBase + "pub fn top(client: http.Client) -> Str ! UpstreamErr {\n  let c: Cache[Int, Str] = new_cache(fn(id) { fetch(client, id) })\n  c.get(1)?\n}\n", want: []string{"XO0701"}},
		{name: "struct field elision", src: elisionBase + "pub struct Svc {\n  cache: Cache[Int, Str]\n}\npub fn (s: Svc) item(id: Int) -> Str ! UpstreamErr { s.cache.get(id)? }\npub fn build(client: http.Client) -> Svc {\n  Svc{cache: new_cache(fn(id) { fetch(client, id) })}\n}\npub fn run(client: http.Client) -> Str ! UpstreamErr uses net.dial {\n  build(client).item(7)?\n}\n"},
		{name: "struct field elision checked at use", src: elisionBase + "pub struct Svc {\n  cache: Cache[Int, Str]\n}\npub fn (s: Svc) item(id: Int) -> Str ! UpstreamErr { s.cache.get(id)? }\npub fn run(client: http.Client) -> Str ! UpstreamErr {\n  let s = Svc{cache: new_cache(fn(id) { fetch(client, id) })}\n  s.item(7)?\n}\n", want: []string{"XO0701"}},
		{name: "result elision passes a parameter through", src: elisionBase + "pub struct Svc {\n  cache: Cache[Int, Str]\n}\npub fn (s: Svc) item(id: Int) -> Str ! UpstreamErr { s.cache.get(id)? }\npub fn wrap(c: Cache[Int, Str]) -> Svc { Svc{cache: c} }\npub fn run(client: http.Client) -> Str ! UpstreamErr uses net.dial {\n  wrap(new_cache(fn(id) { fetch(client, id) })).item(1)?\n}\n"},
		{name: "result elision passes a parameter through and is checked", src: elisionBase + "pub struct Svc {\n  cache: Cache[Int, Str]\n}\npub fn (s: Svc) item(id: Int) -> Str ! UpstreamErr { s.cache.get(id)? }\npub fn wrap(c: Cache[Int, Str]) -> Svc { Svc{cache: c} }\npub fn run(client: http.Client) -> Str ! UpstreamErr {\n  wrap(new_cache(fn(id) { fetch(client, id) })).item(1)?\n}\n", want: []string{"XO0701"}},
		{name: "receiver elision", src: elisionBase + "pub fn (c: Cache[K, V]) peek(k: K) -> V ! UpstreamErr { c.get(k)? }\n"},
		{name: "named receiver effect must be declared", src: elisionBase + "pub fn (c: Cache[K, V, fx]) peek(k: K) -> V ! UpstreamErr { c.get(k)? }\n", want: []string{"XO0701"}},
		{name: "pure cache", src: elisionBase + "pub fn warm(c: Cache[Int, Str]) -> Str ! UpstreamErr { c.get(1)? }\npub fn top() -> Str ! UpstreamErr {\n  warm(new_cache(fn(id) { \"${id}\" }))\n}\n"},
		{name: "wrong number of arguments", src: elisionBase + "pub fn warm(c: Cache[Int]) {}\n", want: []string{"XO0403"}},
	})
}

func TestSmallSyntax04(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "qualified from variant", src: "use std/http\nuse std/json\n\nenum ApiErr {\n  Client(from http.ClientErr)\n  Decode(from json.DecodeErr)\n} derive Display\nfn get(c: http.Client) -> Int ! ApiErr uses net.dial {\n  let resp = c.get(\"https://a.test\")?\n  json.decode[Int](resp.body)?\n}\n"},
		{name: "qualified from variant match", src: "use std/http\n\nenum ApiErr {\n  Client(from http.ClientErr)\n}\nfn f(e: ApiErr) -> Bool {\n  match e {\n    Client(http.ClientErr.Timeout) => true\n    Client(_) => false\n  }\n}\n"},
		{name: "closure uses list", src: "fn run(f: fn() -> Int uses (clock, rand)) -> Int uses (clock, rand) { f() }\nfn g(c: Clock, r: Rand) -> Int uses (clock, rand) {\n  run(fn() -> Int uses (clock, rand) { c.now().unix() + r.int(0, 9) })\n}\n"},
		{name: "closure uses list too small", src: "fn g(c: Clock, r: Rand) -> Int {\n  let f = fn() -> Int uses (clock) { c.now().unix() + r.int(0, 9) }\n  f()\n}\n", want: []string{"XO0701"}},
		{name: "doc comments on fields and variants", src: "struct S {\n  /// The id.\n  id: Int\n}\nenum E {\n  /// First.\n  A\n}\nfn f(s: S, e: E) -> Int {\n  match e {\n    A => s.id\n  }\n}\n"},
	})
}
