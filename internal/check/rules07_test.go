package check

import "testing"

// Channel loops, task.Once, and task.map_concurrent_until (decision 0043).

func TestChanLoop(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "binds the element type", src: "fn f(ch: Chan[Int]) -> Int {\n  var n = 0\n  for x in ch {\n    n += x\n  }\n  n\n}\n"},
		{name: "element type is checked", src: "fn f(ch: Chan[Int]) {\n  for x in ch {\n    let s: Str = x\n    let _ = s.len()\n  }\n}\n", want: []string{"XO0401"}},
		{name: "tuple pattern", src: "fn f(ch: Chan[(Int, Str)]) -> Int {\n  var n = 0\n  for (i, _) in ch {\n    n += i\n  }\n  n\n}\n"},
		{name: "struct pattern", src: "struct Job {\n  id: Int\n}\nfn f(ch: Chan[Job]) -> Int {\n  var n = 0\n  for Job{id} in ch {\n    n += id\n  }\n  n\n}\n"},
		{name: "wildcard", src: "fn f(ch: Chan[Int]) -> Int {\n  var n = 0\n  for _ in ch {\n    n += 1\n  }\n  n\n}\n"},
		{name: "refutable pattern", src: "fn f(ch: Chan[Int?]) -> Int {\n  var n = 0\n  for Some(x) in ch {\n    n += x\n  }\n  n\n}\n", want: []string{"XO0303"}},
		{name: "break and continue", src: "fn f(ch: Chan[Int]) -> Int {\n  var n = 0\n  for x in ch {\n    if x < 0 {\n      continue\n    }\n    if x > 9 {\n      break\n    }\n    n += x\n  }\n  n\n}\n"},
		{name: "labeled break", src: "fn f(ch: Chan[Int]) {\n  outer: for _ in 0..3 {\n    for x in ch {\n      if x == 0 {\n        break outer\n      }\n    }\n  }\n}\n"},
		{name: "a task is not iterable", src: "fn g() -> Int { 1 }\nfn f() {\n  scope s {\n    let t = s.spawn(fn() { g() })\n    for x in t {\n      let _ = x\n    }\n  }\n}\n", want: []string{"XO0406"}},
	})
}

func TestOnce(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "get shares the result type", src: "use std/task\n\nfn f() -> Int {\n  let o = task.once[Int, Never]()\n  o.get(fn() { 1 })\n}\n"},
		{name: "a handle stored in a struct", src: "use std/task\n\nenum E { Down } derive Display\nstruct C { cfg: task.Once[Str, E] }\nfn load() -> Str ! E { \"x\" }\nfn f(c: C) -> Str ! E { c.cfg.get(fn() { load() }) }\nfn g() -> Str ! E { f(C{cfg: task.once()}) }\n"},
		{name: "f result type must match", src: "use std/task\n\nfn f() -> Int {\n  let o = task.once[Int, Never]()\n  o.get(fn() { \"s\" })\n}\n", want: []string{"XO0401"}},
		{name: "errors must be handled", src: "use std/task\n\nenum E { Down } derive Display\nfn load() -> Int ! E { 1 }\nfn f() -> Int {\n  let o = task.once[Int, E]()\n  o.get(fn() { load() })\n}\n", want: []string{"XO0401"}},
		{name: "errors propagate with ?", src: "use std/task\n\nenum E { Down } derive Display\nfn load() -> Int ! E { 1 }\nfn f() -> Int ! E {\n  let o = task.once[Int, E]()\n  o.get(fn() { load() })?\n}\n"},
		{name: "effects of f flow to the caller", src: "use std/task\n\npub fn f(c: Clock) -> Int {\n  let o = task.once[Int, Never]()\n  o.get(fn() {\n    c.sleep(1s)\n    1\n  })\n}\n", want: []string{"XO0701"}},
	})
}

func TestMapConcurrentUntil(t *testing.T) {
	runCases(t, []ruleCase{
		{name: "results are Timed", src: "use std/task\n\nenum E { Bad } derive Display\nfn fetch(n: Int) -> Str ! E { \"${n}\" }\nfn f(c: Clock, ids: List[Int]) -> List[Str] uses clock {\n  let rs = task.map_concurrent_until(c, 1s, ids, 4, fn(id) { fetch(id) })\n  rs.map(fn(r) {\n    match r {\n      Ok(v) => v\n      Err(TimedOut) => \"timeout\"\n      Err(Failed(e)) => \"${e}\"\n    }\n  })\n}\n"},
		{name: "needs the clock effect", src: "use std/task\n\npub fn f(c: Clock, ids: List[Int]) -> Int {\n  task.map_concurrent_until(c, 1s, ids, 4, fn(id) { id }).len()\n}\n", want: []string{"XO0701"}},
		{name: "Never failure needs no Failed arm", src: "use std/task\n\nfn f(c: Clock, ids: List[Int]) -> List[Int] uses clock {\n  task.map_concurrent_until(c, 1s, ids, 4, fn(id) { id * 2 }).map(fn(r) {\n    match r {\n      Ok(v) => v\n      Err(TimedOut) => 0\n    }\n  })\n}\n"},
	})
}
