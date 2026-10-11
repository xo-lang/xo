package check

import "testing"

// TestAcceptsIdioms checks a program that uses many features together and
// must produce no diagnostics (false positives are as costly as misses).
func TestAcceptsIdioms(t *testing.T) {
	src := `use std/http
use std/json
use std/log
use std/task

type Handler = fn(http.Request) -> http.Response ! Error uses net
type UserId = Int

pub struct User {
  pub id: UserId
  pub name: Str
  pub tags: Set[Str] = #{}
} derive Json, Display

pub enum Shape {
  Circle(r: Float)
  Square(side: Float)
} derive Display

pub struct Stack[T] {
  items: List[T]
}

pub fn (s: Stack[T]) peek() -> T? { s.items.last() }

pub fn (var s: Stack[T]) push(x: T) { s.items.push(x) }

fn reply[T: Json](status: Int, v: T) -> http.Response { http.json(status, v) }

fn area(s: Shape) -> Float {
  match s {
    Circle(r) => r * r * 3.14
    Square(side: x) => x * x
  }
}

fn both(a: Bool, b: Bool) -> Int {
  match (a, b) {
    (true, true) => 3
    (true, false) | (false, true) => 1
    (false, false) => 0
  }
}

fn stats(xs: List[Int]) -> (Int, Int) {
  let total = xs.fold(0, fn(acc, x) { acc + x })
  let biggest = xs.sort().last() ?? 0
  (total, biggest)
}

fn names(users: List[User]) -> Str {
  users
    .filter(fn(u) { !u.name.is_empty() })
    .sort_by(fn(u) { u.id })
    .map(fn(u) { u.name.to_upper() })
    .join(", ")
}

fn lookup(m: Map[Str, Int], keys: List[Str]) -> Int {
  var sum = 0
  for (i, k) in keys.indexed() {
    sum += (m.get(k) ?? i)
  }
  for (k, v) in m {
    if k.starts_with("x") {
      sum -= v
    }
  }
  sum
}

fn parse_all(lines: List[Str]) -> List[Int] ! json.DecodeErr {
  lines.map(fn(l) { json.decode_str[Int](l) })
}

fn first_word(s: Str) -> Str? {
  let i = s.find(" ")?
  Some(s[..i])
}

fn bump(o: Int?) -> Int? { o.map(fn(x) { x + 1 }) }

fn describe(r: Result[Int, json.DecodeErr]) -> Str {
  match r.map_err(fn(e) { e.msg }) {
    Ok(v) => "${v}"
    Err(msg) => msg
  }
}

fn transfer(var from: Int, var to: Int, amt: Int) -> () ! Error
  requires amt > 0
  ensures from + to == old(from + to)
{
  from -= amt
  to += amt
}

fn counter() -> Int {
  let hits = Atomic[Int].new(0)
  let inc = fn() { hits.add(1) }
  inc()
  inc()
  hits.get()
}

fn pipeline(ch: Chan[Int], out: Chan[Int]) -> Int uses clock, log {
  var n = 0
  select {
    v = ch.recv() => n += v ?? 0
    out.send(n) => n += 1
    after 1s => log.warn("idle")
  }
  n
}

fn with_defer(xs: List[Int]) -> Int {
  var done = false
  defer {
    if !done {
      log.info("aborted")
    }
  }
  let n = xs.len()
  done = true
  n
}

fn run_jobs(clock: Clock, jobs: List[Int]) -> List[Int] ! task.Timed[Never] uses clock {
  task.with_timeout(clock, 5s, fn() {
    let results = Mutex[Map[Int, Int]].new({})
    scope s {
      for j in jobs {
        s.spawn(fn() { results.with(fn(var m) { m.put(j, j * 2) }) })
      }
    }
    results.with(fn(m) { m.values() })
  })
}

pub fn routes(store: Mutex[Map[Int, User]]) -> http.Router uses net {
  var r = http.router()
  let h: Handler = fn(req) {
    let id = req.param_int("id")
    let u = store.with(fn(m) { m.get(id) }).or_err("no user ${id}")?
    reply(200, u)
  }
  r.handle("GET /users/{id:Int}", h)
  r.handle("GET /health", fn(_req) { http.text(200, "ok") })
  r
}

fn main(os: Os) ! {
  var st = Stack[Int]{items: []}
  st.push(1)
  let top = st.peek() ?? 0
  let (total, biggest) = stats([1, 2, 3])
  let shapes = [Shape.Circle(r: 1.0), Square(side: 2.0)]
  os.stdio.println("${top} ${total} ${biggest} ${shapes.len()} ${area(shapes[0])} ${both(true, false)}")
  let srv = http.server(os.net.listen(":8080")?, routes(Mutex.new({})))
  srv.serve()?
}

test "stack" {
  var st = Stack[Str]{items: []}
  st.push("a")
  expect(st.peek()) == Some("a")
  expect(names([User{id: 2, name: "b"}, User{id: 1, name: "a"}])) == "A, B"
  let parsed = parse_all(["1", "2"])?
  expect(parsed) == [1, 2]
  expect(first_word("hello world")) == Some("hello")
}
`
	ds := checkSrc(t, src)
	if len(ds) != 0 {
		t.Errorf("want no diagnostics:\n%s", humanAll(ds))
	}
}
