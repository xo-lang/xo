package check

import (
	"strings"
	"testing"
)

// The xo side of the planted bug benchmark (bench/planted/README.md). A
// small rate limiter, modeled on task 05, is checked clean; each variant
// plants one bug by text replacement. Bug classes X1 to X6 must be rejected
// with the listed code (or, for nil and data races, shown to be
// inexpressible: every attempt is rejected). N1 to N4 are logic bugs that xo
// does not prevent; their variants must check clean. G1 (and X1d) are
// correct programs that spec 0.4 made expressible; they must check clean.

const plantedBase = `use std/http
use std/log

pub struct Limit {
  pub capacity: Int
  pub refill_per_sec: Float
}

pub enum ConfigErr {
  NotANumber(text: Str)
  OutOfRange(value: Int)
} derive Display

/// Token bucket for one client key.
pub struct Bucket {
  tokens: Float
  last: Time
}

pub fn new_bucket(limit: Limit, now: Time) -> Bucket {
  Bucket{tokens: Float(limit.capacity), last: now}
}

fn (var b: Bucket) refill(limit: Limit, now: Time) {
  let elapsed = now - b.last
  if elapsed > 0s {
    b.tokens = (b.tokens + elapsed.seconds() * limit.refill_per_sec).min(Float(limit.capacity))
    b.last = now
  }
}

fn (var b: Bucket) take(limit: Limit, now: Time) -> Bool {
  b.refill(limit, now)
  if b.tokens >= 1.0 {
    b.tokens -= 1.0
    true
  } else {
    false
  }
}

pub enum Decision {
  Allowed
  Limited(retry_after: Duration)
}

pub struct Limiter {
  buckets: Mutex[Map[Str, Bucket]]
  limit: Limit
}

pub fn new_limiter(limit: Limit) -> Limiter {
  Limiter{buckets: Mutex.new({}), limit: limit}
}

pub fn check(l: Limiter, key: Str, now: Time) -> Decision {
  let limit = l.limit
  l.buckets.with(fn(var m) {
    m.update(key, new_bucket(limit, now), fn(var b) {
      if b.take(limit, now) {
        Decision.Allowed
      } else {
        Decision.Limited(retry_after: 1s)
      }
    })
  })
}

pub fn status_for(d: Decision) -> Int {
  match d {
    Allowed => 200
    Limited(..) => 429
  }
}

pub fn parse_capacity(s: Str) -> Int ! ConfigErr {
  let n = Int.parse(s).or_err(ConfigErr.NotANumber(text: s))?
  if n < 1 || n > 1000 {
    return Err(ConfigErr.OutOfRange(value: n))
  }
  n
}

fn evict_idle(l: Limiter, now: Time, idle: Duration) -> Int {
  l.buckets.with(fn(var m) {
    let before = m.len()
    m = m.filter(fn(_k, b) { now - b.last <= idle })
    before - m.len()
  })
}

fn run_evictor(l: Limiter, clock: Clock) {
  while {
    clock.sleep(1m)
    let n = evict_idle(l, clock.now(), 10m)
    if n > 0 {
      log.info("evicted", {"count": n})
    }
  }
}

fn handler(l: Limiter, clock: Clock) -> fn(http.Request) -> http.Response uses clock {
  fn(req) {
    let key = req.header("X-Api-Key") ?? req.remote_ip
    let d = check(l, key, clock.now())
    match d {
      Allowed => http.text(status_for(d), "ok")
      Limited(retry_after: wait) => http.empty(status_for(d)).with_header("Retry-After", "${wait.ceil_seconds()}")
    }
  }
}

fn main(os: Os) ! {
  let capacity = parse_capacity(os.env.get("CAPACITY") ?? "20")?
  let limiter = new_limiter(Limit{capacity: capacity, refill_per_sec: 5.0})
  var r = http.router()
  r.handle("GET /", handler(limiter, os.clock))
  let srv = http.server(os.net.listen(":8080")?, r)
  scope s {
    s.spawn_background(fn() { run_evictor(limiter, os.clock) })
    srv.serve()?
  }
}

test "limits after capacity" {
  let l = new_limiter(Limit{capacity: 2, refill_per_sec: 1.0})
  let now = os.clock.now()
  expect(check(l, "a", now)) == Decision.Allowed
  expect(check(l, "a", now)) == Decision.Allowed
  expect(status_for(check(l, "a", now))) == 429
}
`

type plantedVariant struct {
	id    string
	class string
	edits [][2]string // old, new; each old must occur exactly once
	want  []string    // codes; nil means accepted
}

var plantedVariants = []plantedVariant{
	{id: "X1", class: "ignored error result", edits: [][2]string{{
		"  let capacity = parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")?\n",
		"  parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")\n  let capacity = 20\n",
	}}, want: []string{"XO0201"}},
	{id: "X1b", class: "ignored error result (used as the success value)", edits: [][2]string{{
		"parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")?",
		"parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")",
	}}, want: []string{"XO0401"}},
	{id: "X2", class: "nil: there is no nil literal", edits: [][2]string{{
		"Limiter{buckets: Mutex.new({}), limit: limit}",
		"Limiter{buckets: nil, limit: limit}",
	}}, want: []string{"XO0601"}},
	{id: "X2b", class: "nil: None is not a value of a non optional type", edits: [][2]string{{
		"Limiter{buckets: Mutex.new({}), limit: limit}",
		"Limiter{buckets: None, limit: limit}",
	}}, want: []string{"XO0401"}},
	{id: "X2c", class: "nil: a field cannot be left out", edits: [][2]string{{
		"Limiter{buckets: Mutex.new({}), limit: limit}",
		"Limiter{limit: limit}",
	}}, want: []string{"XO0403"}},
	{id: "X2d", class: "nil: an optional cannot be used without unwrapping", edits: [][2]string{{
		"let key = req.header(\"X-Api-Key\") ?? req.remote_ip",
		"let key = req.header(\"X-Api-Key\")",
	}}, want: []string{"XO0401"}},
	{id: "X3", class: "missing enum case", edits: [][2]string{{
		"    Limited(..) => 429\n",
		"",
	}}, want: []string{"XO0301"}},
	{id: "X4", class: "data race: a closure cannot mutate captured state", edits: [][2]string{{
		"    s.spawn_background(fn() { run_evictor(limiter, os.clock) })\n",
		"    var hits = 0\n    s.spawn_background(fn() { hits += 1 })\n    log.info(\"hits\", {\"n\": hits})\n",
	}}, want: []string{"XO0502"}},
	{id: "X4b", class: "data race: shared state is only reachable through the mutex, and a read only view cannot be mutated", edits: [][2]string{{
		"  l.buckets.with(fn(var m) {\n    m.update(",
		"  l.buckets.with(fn(m) {\n    m.update(",
	}}, want: []string{"XO0501"}},
	{id: "X4c", class: "data race: a value copied out of the mutex is a private copy", edits: [][2]string{{
		"  l.buckets.with(fn(var m) {\n    m.update(key",
		"  let m = l.buckets.with(fn(m) { m })\n  l.buckets.with(fn(var _m) {\n    m.update(key",
	}}, want: []string{"XO0501", "XO0804"}},
	{id: "X5", class: "task outliving its scope: spawn without a scope", edits: [][2]string{{
		"    s.spawn_background(fn() { run_evictor(limiter, os.clock) })\n",
		"    spawn(fn() { run_evictor(limiter, os.clock) })\n",
	}}, want: []string{"XO0803"}},
	{id: "X5b", class: "task outliving its scope: the scope handle cannot escape", edits: [][2]string{{
		"  scope s {\n    s.spawn_background",
		"  let outer = scope h { h }\n  outer.spawn_background(fn() { run_evictor(limiter, os.clock) })\n  scope s {\n    s.spawn_background",
	}}, want: []string{"XO0802"}},
	{id: "X6", class: "lost update", edits: [][2]string{{
		"pub fn status_for(",
		"pub fn check_copy(l: Limiter, key: Str, now: Time) -> Bool {\n  let limit = l.limit\n  l.buckets.with(fn(m) {\n    var b = m.get(key) ?? new_bucket(limit, now)\n    b.take(limit, now)\n  })\n}\n\npub fn status_for(",
	}}, want: []string{"XO0520"}},
	{id: "X1c", class: "ignored error result discarded with `let _ =` (spec 0.4, decision 0031)", edits: [][2]string{{
		"  let capacity = parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")?\n",
		"  let _ = parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")\n  let capacity = 20\n",
	}}, want: []string{"XO0206"}},
	{id: "X1d", class: "error discarded with a reason is accepted (spec 0.4)", edits: [][2]string{{
		"  let capacity = parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\")?\n",
		"  let capacity = parse_capacity(os.env.get(\"CAPACITY\") ?? \"20\").ignore_err(\"fall back to the default\") ?? 20\n",
	}}},
	{id: "X5c", class: "task outliving its scope: a struct holding a task group escapes the scope (spec 0.4)", edits: [][2]string{
		{"use std/log\n", "use std/log\nuse std/task\n"},
		{"pub fn status_for(", "struct Evictor {\n  tasks: task.Group\n}\n\nfn start_evictor(l: Limiter, clock: Clock) -> Evictor {\n  scope s {\n    let e = Evictor{tasks: s.group()}\n    e.tasks.spawn_background(fn() { run_evictor(l, clock) })\n    e\n  }\n}\n\npub fn status_for("},
	}, want: []string{"XO0802"}},
	{id: "X6b", class: "lost update: a local mutated after being appended (spec 0.4, decision 0034)", edits: [][2]string{{
		"pub fn status_for(",
		"pub fn fresh_buckets(l: Limiter, keys: List[Str], now: Time) -> List[Bucket] {\n  var out = []\n  for _k in keys {\n    var b = new_bucket(l.limit, now)\n    out.push(b)\n    b.refill(l.limit, now)\n  }\n  out\n}\n\npub fn status_for(",
	}}, want: []string{"XO0520"}},
	{id: "G1", class: "a cache that stores a task group and refreshes in the background from a method (spec 0.4, decision 0032)", edits: [][2]string{
		{"use std/log\n", "use std/log\nuse std/task\n"},
		{"pub fn status_for(", "pub struct LimitCache {\n  limits: Mutex[Map[Str, Limit]]\n  tasks: task.Group\n}\n\nfn (c: LimitCache) limit_for(key: Str, clock: Clock) -> Limit {\n  let hit = c.limits.with(fn(m) { m.get(key) })\n  c.tasks.spawn_background(fn() { c.refresh(key, clock) })\n  hit ?? Limit{capacity: 20, refill_per_sec: 5.0}\n}\n\nfn (c: LimitCache) refresh(key: Str, clock: Clock) {\n  clock.sleep(1s)\n  c.limits.with(fn(var m) { m.put(key, Limit{capacity: 20, refill_per_sec: 5.0}) })\n}\n\npub fn status_for("},
		{"  let limiter = new_limiter(Limit{capacity: capacity, refill_per_sec: 5.0})\n", "  let cache = LimitCache{limits: Mutex.new({}), tasks: os.tasks}\n  let limiter = new_limiter(cache.limit_for(\"default\", os.clock) with {capacity: capacity})\n"},
	}},
	{id: "N1", class: "off by one", edits: [][2]string{{"if b.tokens >= 1.0 {", "if b.tokens > 1.0 {"}}},
	{id: "N2", class: "wrong status code", edits: [][2]string{{"Limited(..) => 429", "Limited(..) => 503"}}},
	{id: "N3", class: "wrong time math", edits: [][2]string{{"elapsed.seconds() * limit.refill_per_sec", "elapsed.seconds() / limit.refill_per_sec"}}},
	{id: "N3b", class: "wrong time math (units)", edits: [][2]string{{"Decision.Limited(retry_after: 1s)", "Decision.Limited(retry_after: 1ms)"}}},
	{id: "N4", class: "inverted condition", edits: [][2]string{{"if b.take(limit, now) {", "if !b.take(limit, now) {"}}},
}

func TestPlantedBase(t *testing.T) {
	ds := checkSrc(t, plantedBase)
	if len(ds) != 0 {
		t.Fatalf("base program must check clean:\n%s", humanAll(ds))
	}
}

func TestPlantedBugs(t *testing.T) {
	for _, v := range plantedVariants {
		t.Run(v.id, func(t *testing.T) {
			src := plantedBase
			for _, e := range v.edits {
				if n := strings.Count(src, e[0]); n != 1 {
					t.Fatalf("edit target occurs %d times: %q", n, e[0])
				}
				src = strings.Replace(src, e[0], e[1], 1)
			}
			ds := checkSrc(t, src)
			got := codes(ds)
			if v.want == nil {
				if len(ds) != 0 {
					t.Errorf("%s (%s): xo does not prevent this class, so the variant must be accepted, got:\n%s", v.id, v.class, humanAll(ds))
				}
				return
			}
			for _, w := range v.want {
				found := false
				for _, g := range got {
					if g == w {
						found = true
					}
				}
				if !found {
					t.Errorf("%s (%s): want %s, got %v\n%s", v.id, v.class, w, got, humanAll(ds))
				}
			}
			if len(got) != len(v.want) {
				t.Errorf("%s (%s): want exactly %v, got %v\n%s", v.id, v.class, v.want, got, humanAll(ds))
			}
		})
	}
}
