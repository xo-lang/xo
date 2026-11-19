// A fuzz harness for std/task's map_concurrent, Semaphore and Once against
// answers computed here in Go (decision 0616's fuzz kind; task #152). Each
// case is one of:
//
//   - map_concurrent over 0 to 40 items, each an Int, a fail flag and a
//     spin count, with a limit from 1 to 45: the results must be each
//     item's Ok(mix(x)) or Err(x / 2 + 7) in input order, and no more than
//     limit items may be in flight at once;
//   - a Semaphore of 1 to 8 permits that 1 to 40 concurrent callers go
//     through: the same per caller results, never more than permits held;
//   - a Once that 1 to 40 concurrent callers get: f runs exactly once and
//     every caller sees its result, Ok or Err;
//   - a limit or permit count of 0 or below: the fault message, caught by
//     task.isolate.
//
// x comes from the hostile dictionary (internal/hostile) in a quarter of
// the items and the limit faults; mix is a product modulo a prime that
// cannot overflow, the same in Go and Xo. The cases
// from seed 1 are the committed corpus
// testdata/corpus/taskfuzz/taskfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestTaskFuzzCorpusCurrent checks it is what this
// generator writes (XO_TASKFUZZ_WRITE=1 rewrites it). TestTaskFuzz runs a
// fresh seed (XO_TASKFUZZ_SEED, default 2; XO_TASKFUZZ_N cases, default
// 2000) on XO_TASKFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

type tkItem struct {
	x    int64
	fail bool
	spin int
}

type tkCase struct {
	kind  int // 0 map_concurrent, 1 semaphore, 2 once, 3 map fault, 4 semaphore fault
	limit int
	items []tkItem
	want  []tkResult
	msg   string
}

type tkResult struct {
	ok bool
	v  int64
}

func tkMix(x int64) int64 { return (x % 1000003) * 2654435761 % 1000003 }

func tkWork(it tkItem) tkResult {
	if it.fail {
		return tkResult{false, it.x/2 + 7}
	}
	return tkResult{true, tkMix(it.x)}
}

func tkItems(r *rand.Rand, n int) []tkItem {
	its := make([]tkItem, n)
	for i := range its {
		var x int64
		switch r.Intn(4) {
		case 0:
			x = hostile.Pick(r, hostile.Ints())
		default:
			x = r.Int63n(2000001) - 1000000
		}
		its[i] = tkItem{x: x, fail: r.Intn(4) == 0, spin: []int{0, 0, 10, 200, 2000}[r.Intn(5)]}
	}
	return its
}

// tkBadLimit is a limit or permit count of 0 or below: a small one or a
// negative value of the hostile dictionary (math.MinInt64 among them).
func tkBadLimit(r *rand.Rand) int {
	if r.Intn(2) == 0 {
		for {
			if v := hostile.Pick(r, hostile.Ints()); v <= 0 {
				return int(v)
			}
		}
	}
	return -r.Intn(5)
}

func tkCases(r *rand.Rand, n int) []tkCase {
	cs := make([]tkCase, 0, n)
	for len(cs) < n {
		var c tkCase
		switch k := r.Intn(20); {
		case k < 9:
			c.kind = 0
			c.items = tkItems(r, r.Intn(41))
			c.limit = 1 + r.Intn(45)
			if r.Intn(4) == 0 {
				c.limit = 1
			}
			for _, it := range c.items {
				c.want = append(c.want, tkWork(it))
			}
		case k < 14:
			c.kind = 1
			c.items = tkItems(r, 1+r.Intn(40))
			c.limit = 1 + r.Intn(8)
			for _, it := range c.items {
				c.want = append(c.want, tkWork(it))
			}
		case k < 18:
			c.kind = 2
			c.items = tkItems(r, 1+r.Intn(40))
			w := tkWork(c.items[0])
			for range c.items {
				c.want = append(c.want, w)
			}
		case k == 18:
			c.kind = 3
			c.limit = tkBadLimit(r)
			c.msg = fmt.Sprintf("map_concurrent needs a positive limit, got %d", c.limit)
		default:
			c.kind = 4
			c.limit = tkBadLimit(r)
			c.msg = fmt.Sprintf("a semaphore needs at least one permit, got %d", c.limit)
		}
		cs = append(cs, c)
	}
	return cs
}

const tkCheckXo = `use std/task

fn mix(x: Int) -> Int { (x % 1000003) * 2654435761 % 1000003 }

/// An item's work: spin, then Err(x / 2 + 7) when it fails, else Ok(mix(x)).
fn work(x: Int, fail: Bool, spin: Int) -> Int ! Int {
  var acc = 0
  for k in 0..spin {
    acc += k
  }
  if fail {
    return Err(x / 2 + 7 + acc * 0)
  }
  mix(x) + acc * 0
}

fn flat(rs: List[Result[Int, Int]]) -> List[(Bool, Int)] {
  var out: List[(Bool, Int)] = []
  for r in rs {
    match r {
      Ok(v) => out.push((true, v))
      Err(e) => out.push((false, e))
    }
  }
  out
}

/// A case's results, the most items in flight at once, and how many times
/// a Once ran f.
fn run(kind: Int, limit: Int, items: List[(Int, Bool, Int)]) -> (List[(Bool, Int)], Int, Int) {
  let live = Mutex[(Int, Int)].new((0, 0))
  let runs = Mutex[Int].new(0)
  let all = items.len().max(1)
  var rs: List[Result[Int, Int]] = []
  if kind == 0 {
    rs = task.map_concurrent(items, limit, fn(it: (Int, Bool, Int)) -> Int ! Int {
      live.with(fn(var p: (Int, Int)) { p = (p.0 + 1, p.1.max(p.0 + 1)) })
      defer live.with(fn(var p: (Int, Int)) { p = (p.0 - 1, p.1) })
      work(it.0, it.1, it.2)
    })
  } else if kind == 1 {
    let sem = task.semaphore(limit)
    rs = task.map_concurrent(items, all, fn(it: (Int, Bool, Int)) -> Int ! Int {
      sem.with(fn() -> Int ! Int {
        live.with(fn(var p: (Int, Int)) { p = (p.0 + 1, p.1.max(p.0 + 1)) })
        defer live.with(fn(var p: (Int, Int)) { p = (p.0 - 1, p.1) })
        work(it.0, it.1, it.2)
      })
    })
  } else {
    let o: task.Once[Int, Int] = task.once()
    let first = items[0]
    rs = task.map_concurrent(items, all, fn(it: (Int, Bool, Int)) -> Int ! Int {
      o.get(fn() -> Int ! Int {
        runs.with(fn(var n: Int) { n += 1 })
        work(first.0, first.1, it.2)
      })
    })
  }
  (flat(rs), live.with_read(fn(p: (Int, Int)) -> Int { p.1 }), runs.with_read(fn(n: Int) -> Int { n }))
}

/// The fault a limit or permit count of n raises, caught by task.isolate.
fn fault_of(kind: Int, n: Int) -> Str {
  let r = task.isolate(fn() -> Int ! Never {
    if kind == 3 {
      let _ = task.map_concurrent([1], n, fn(x: Int) -> Int ! Never { x })
    } else {
      let _ = task.semaphore(n)
    }
    0
  })
  match r {
    Err(task.Isolated.Faulted(f)) => f.msg
    _ => "no fault"
  }
}

test "std/task's map_concurrent, Semaphore and Once give the precomputed results within their limits" {
  var bad: List[Str] = []
  for (i, (kind, limit, items, want, msg)) in CASES.indexed() {
    if kind >= 3 {
      let got = fault_of(kind, limit)
      if got != msg {
        bad.push("case ${i}: fault ${got}")
      }
    } else {
      let (got, most, runs) = run(kind, limit, items)
      if got != want {
        bad.push("case ${i}: results ${got.debug()}")
      } else if kind < 2 && most > limit {
        bad.push("case ${i}: ${most} in flight, limit ${limit}")
      } else if kind == 2 && runs != 1 {
        bad.push("case ${i}: f ran ${runs} times")
      }
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`

func tkXoFile(seed int64, cases []tkCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/taskfuzz_test.go (seed %d, %d cases);\n", seed, len(cases))
	b.WriteString("// do not edit: XO_TASKFUZZ_WRITE=1 go test ./internal/stdtest -run TestTaskFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(tkCheckXo)
	b.WriteString("\n/// (kind, limit or permits, items (x, fail, spin), the results (ok, value), the fault message)\nconst CASES: List[(Int, Int, List[(Int, Bool, Int)], List[(Bool, Int)], Str)] = [\n")
	for _, c := range cases {
		its := make([]string, len(c.items))
		for i, it := range c.items {
			its[i] = fmt.Sprintf("(%d, %t, %d)", it.x, it.fail, it.spin)
		}
		ws := make([]string, len(c.want))
		for i, w := range c.want {
			ws[i] = fmt.Sprintf("(%t, %d)", w.ok, w.v)
		}
		fmt.Fprintf(&b, "  (%d, %d, [%s], [%s], %q),\n", c.kind, c.limit, strings.Join(its, ", "), strings.Join(ws, ", "), c.msg)
	}
	b.WriteString("]\n")
	return b.String()
}

// tkCoverage fails when the cases stop reaching every kind, a limit of 1,
// limits under the item count, failures and empty lists.
func tkCoverage(t *testing.T, cases []tkCase) {
	t.Helper()
	kinds := map[int]int{}
	one, under, fails, empty := 0, 0, 0, 0
	for _, c := range cases {
		kinds[c.kind]++
		if c.kind < 2 && c.limit == 1 {
			one++
		}
		if c.kind < 2 && c.limit < len(c.items) {
			under++
		}
		if c.kind == 0 && len(c.items) == 0 {
			empty++
		}
		for _, w := range c.want {
			if !w.ok {
				fails++
				break
			}
		}
	}
	n := len(cases)
	if len(kinds) < 5 || one < n/20 || under < n/5 || fails < n/3 || empty == 0 {
		t.Fatalf("coverage fell: kinds %v, %d limit 1, %d limit under the items, %d with failures, %d empty of %d", kinds, one, under, fails, empty, n)
	}
}

const tkCorpus = "testdata/corpus/taskfuzz/taskfuzz_test.xo"

// TestTaskFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestTaskFuzzCorpusCurrent(t *testing.T) {
	cases := tkCases(rand.New(rand.NewSource(1)), 400)
	tkCoverage(t, cases)
	bxCorpusCurrent(t, tkCorpus, "XO_TASKFUZZ_WRITE", tkXoFile(1, cases))
}

// TestTaskFuzz runs a fresh seed's cases.
func TestTaskFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the task fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_TASKFUZZ_SEED", 2))
	n := envInt(t, "XO_TASKFUZZ_N", 2000)
	cases := tkCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "taskfuzz_test.xo", tkXoFile(seed, cases), envBackends("XO_TASKFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
