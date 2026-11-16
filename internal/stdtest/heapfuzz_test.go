// A fuzz harness for std/heap against Go's container/heap (decision 0616's
// fuzz kind). Each case is an initial list (empty, short or up to 200
// values, from a small range so equal keys are common) and up to 120
// operations (push, pop, remove at an index, fix at an index with a new
// value, peek); std/heap, after Go's heap.go, must return Go's values and
// leave Go's exact element order (Init, Push, Pop, Remove and Fix on an
// IntHeap, the less function a < b). The cases run twice: on a heap from
// new plus pushes or from_list, and on a tracked heap whose moved callback
// must leave every element's last reported index at its place.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/heapfuzz/heapfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestHeapFuzzCorpusCurrent checks it is what this
// generator writes (XO_HEAPFUZZ_WRITE=1 rewrites it). TestHeapFuzz runs a
// fresh seed (XO_HEAPFUZZ_SEED, default 2; XO_HEAPFUZZ_N cases, default
// 2000) on XO_HEAPFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"container/heap"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

type hpInts []int

func (h hpInts) Len() int           { return len(h) }
func (h hpInts) Less(i, j int) bool { return h[i] < h[j] }
func (h hpInts) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *hpInts) Push(x any)        { *h = append(*h, x.(int)) }
func (h *hpInts) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// hpCase is an initial list, the operations (code, argument), and Go's
// returned values and final order.
type hpCase struct {
	init     []int
	ops      [][2]int
	got, end []int
}

func hpCases(r *rand.Rand, n int) []hpCase {
	cs := make([]hpCase, 0, n)
	for len(cs) < n {
		span := []int{3, 10, 100, 1 << 20}[r.Intn(4)]
		var init []int
		switch k := r.Intn(4); k {
		case 0:
		case 1:
			init = make([]int, 1+r.Intn(8))
		default:
			init = make([]int, r.Intn(201))
		}
		for i := range init {
			init[i] = r.Intn(span) - span/4
		}
		c := hpCase{init: append([]int(nil), init...)}
		h := hpInts(append([]int(nil), init...))
		heap.Init(&h)
		for k := r.Intn(121); k > 0; k-- {
			code := r.Intn(5)
			if r.Intn(3) == 0 {
				code = 0 // pushes a third of the time keep heaps from emptying
			}
			arg := r.Intn(span) - span/4
			if code == 2 || code == 3 {
				if len(h) == 0 {
					continue
				}
				arg = r.Intn(len(h))
			}
			c.ops = append(c.ops, [2]int{code, arg})
			switch code {
			case 0:
				heap.Push(&h, arg)
			case 1:
				if len(h) == 0 {
					c.got = append(c.got, -999999)
				} else {
					c.got = append(c.got, heap.Pop(&h).(int))
				}
			case 2:
				c.got = append(c.got, heap.Remove(&h, arg).(int))
			case 3:
				v := r.Intn(span) - span/4
				c.ops[len(c.ops)-1] = [2]int{3, arg*1000000000 + (v + 500000000)}
				h[arg] = v
				heap.Fix(&h, arg)
			case 4:
				if len(h) == 0 {
					c.got = append(c.got, -999999)
				} else {
					c.got = append(c.got, h[0])
				}
			}
		}
		c.end = append([]int(nil), h...)
		cs = append(cs, c)
	}
	return cs
}

const hpCheckXo = `use std/heap

/// The operations on h: the values pop, remove and peek gave (-999999 for
/// None), and the final elements.
fn replay(h0: heap.Heap[(Int, Int)], ops: List[(Int, Int)]) -> (List[Int], List[(Int, Int)]) {
  var h = h0
  var got: List[Int] = []
  var id = 1000000
  for (code, arg) in ops {
    match code {
      0 => {
        h.push((arg, id))
        id += 1
      }
      1 => got.push(h.pop().map(fn(x: (Int, Int)) -> Int { x.0 }) ?? -999999)
      2 => got.push(h.remove(arg).map(fn(x: (Int, Int)) -> Int { x.0 }) ?? -999998)
      3 => {
        let i = arg / 1000000000
        let v = arg % 1000000000 - 500000000
        h.fix(i, (v, id))
        id += 1
      }
      _ => got.push(h.peek().map(fn(x: (Int, Int)) -> Int { x.0 }) ?? -999999)
    }
  }
  (got, h.to_list())
}

fn less(a: (Int, Int), b: (Int, Int)) -> Bool { a.0 < b.0 }

fn with_ids(xs: List[Int]) -> List[(Int, Int)] {
  var out: List[(Int, Int)] = []
  for i in 0..xs.len() {
    out.push((xs[i], i))
  }
  out
}

fn keys(xs: List[(Int, Int)]) -> List[Int] { xs.map(fn(x: (Int, Int)) -> Int { x.0 }) }

/// Space separated Ints ("" for none).
fn ints(s: Str) -> List[Int] {
  if s == "" {
    return []
  }
  s.split(" ").map(fn(w: Str) -> Int { Int.parse(w) ?? fault("int ${w}") })
}

/// Space separated code:arg pairs.
fn ops_of(s: Str) -> List[(Int, Int)] {
  if s == "" {
    return []
  }
  s.split(" ").map(fn(w: Str) -> (Int, Int) {
    let (a, b) = w.cut(":") ?? fault("op ${w}")
    (Int.parse(a) ?? fault("code"), Int.parse(b) ?? fault("arg"))
  })
}

test "std/heap's order and answers are Go's container/heap" {
  var bad: List[Str] = []
  var k = 0
  for (init_s, ops_s, got_s, end_s) in CASES {
    let (init, ops, want_got, want_end) = (ints(init_s), ops_of(ops_s), ints(got_s), ints(end_s))
    let (got, end) = replay(heap.from_list(with_ids(init), less), ops)
    if got != want_got || keys(end) != want_end {
      bad.push("case ${k}: got ${got.debug()} ${keys(end).debug()} want ${want_got.debug()} ${want_end.debug()}")
    }
    k += 1
  }
  expect(bad[0..(if bad.len() < 5 { bad.len() } else { 5 })]) == []
}

test "a tracked heap reports where each element lands" {
  var bad: List[Str] = []
  var k = 0
  for (init_s, ops_s, got_s, _) in CASES {
    let (init, ops, want_got) = (ints(init_s), ops_of(ops_s), ints(got_s))
    let at = Mutex[Map[Int, Int]].new({})
    var h = heap.tracked(less, fn(x: (Int, Int), i: Int) { at.with(fn(var m: Map[Int, Int]) { m[x.1] = i }) })
    for x in with_ids(init) {
      h.push(x)
    }
    // pushed one at a time, the order differs from from_list's Init: check
    // the positions and the number of answers only
    let (got, items) = replay(h, ops)
    for i in 0..items.len() {
      let r = at.with_read(fn(m: Map[Int, Int]) -> Int { m.get(items[i].1) ?? -1 })
      if r != i {
        bad.push("case ${k}: element ${items[i].1} at ${i}, reported at ${r}")
        break
      }
    }
    if got.len() != want_got.len() {
      bad.push("case ${k}: ${got.len()} answers, want ${want_got.len()}")
    }
    k += 1
  }
  expect(bad[0..(if bad.len() < 5 { bad.len() } else { 5 })]) == []
}
`

func hpInts2Xo(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return `"` + strings.Join(parts, " ") + `"`
}

func hpXoFile(seed int64, cases []hpCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/heapfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// container/heap; do not edit: XO_HEAPFUZZ_WRITE=1 go test ./internal/stdtest -run TestHeapFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(hpCheckXo)
	b.WriteString("\n/// (initial list, operations as code:arg (0 push arg, 1 pop, 2 remove at arg, 3 fix at arg / 1e9 with arg % 1e9 - 5e8, 4 peek), Go's answers, Go's final order)\nconst CASES: List[(Str, Str, Str, Str)] = [\n")
	for _, c := range cases {
		ops := make([]string, len(c.ops))
		for i, o := range c.ops {
			ops[i] = fmt.Sprintf("%d:%d", o[0], o[1])
		}
		fmt.Fprintf(&b, "  (%s, %q, %s, %s),\n", hpInts2Xo(c.init), strings.Join(ops, " "), hpInts2Xo(c.got), hpInts2Xo(c.end))
	}
	b.WriteString("]\n")
	return b.String()
}

// hpCoverage fails when the cases stop reaching empty heaps, pops of an
// empty heap, removes and fixes.
func hpCoverage(t *testing.T, cases []hpCase) {
	t.Helper()
	empty, popEmpty, removes, fixes := 0, 0, 0, 0
	for _, c := range cases {
		if len(c.init) == 0 {
			empty++
		}
		for _, g := range c.got {
			if g == -999999 {
				popEmpty++
			}
		}
		for _, o := range c.ops {
			switch o[0] {
			case 2:
				removes++
			case 3:
				fixes++
			}
		}
	}
	n := len(cases)
	if empty < n/10 || popEmpty < n/20 || removes < n*3 || fixes < n*3 {
		t.Fatalf("coverage fell: %d empty starts, %d pops of an empty heap, %d removes, %d fixes in %d cases", empty, popEmpty, removes, fixes, n)
	}
}

const hpCorpus = "testdata/corpus/heapfuzz/heapfuzz_test.xo"

// TestHeapFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestHeapFuzzCorpusCurrent(t *testing.T) {
	cases := hpCases(rand.New(rand.NewSource(1)), 120)
	hpCoverage(t, cases)
	bxCorpusCurrent(t, hpCorpus, "XO_HEAPFUZZ_WRITE", hpXoFile(1, cases))
}

// TestHeapFuzz runs a fresh seed's cases.
func TestHeapFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the heap fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_HEAPFUZZ_SEED", 2))
	n := envInt(t, "XO_HEAPFUZZ_N", 2000)
	cases := hpCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "heapfuzz_test.xo", hpXoFile(seed, cases), envBackends("XO_HEAPFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
