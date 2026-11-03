package main

import (
	"container/heap"
	"fmt"
	"math/rand"
	"strings"
)

// refHeap is Go's container/heap on a slice of ints, the reference.
type refHeap []int

func (h refHeap) Len() int           { return len(h) }
func (h refHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h refHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *refHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *refHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// The std/heap corpus: random sequences of push, pop, remove and fix on
// container/heap, the whole heap array and the result written after every
// operation (Go's algorithm is deterministic, and std/heap ports it, so the
// arrays match, not only the pops), and heap.Init on random slices.
func heapCorpus() string {
	rg := rand.New(rand.NewSource(56))
	var o strings.Builder
	o.WriteString(header("Go's container/heap on random operation sequences (the heap array after every operation) and heap.Init on random slices, against std/heap"))
	o.WriteString("use std/heap\n\n")
	o.WriteString("/// (operations as (kind, position, value), then per operation the result (-1 for none) and the heap array)\nconst RUNS: List[(List[(Str, Int, Int)], List[(Int, List[Int])])] = [\n")
	for run := 0; run < 150; run++ {
		var h refHeap
		var ops, res []string
		n := 5 + rg.Intn(60)
		vals := 1 + rg.Intn(40)
		for k := 0; k < n; k++ {
			kind := rg.Intn(8)
			result := -1
			var op string
			switch {
			case kind < 4 || len(h) == 0:
				v := rg.Intn(vals)
				heap.Push(&h, v)
				op = fmt.Sprintf(`("push", 0, %d)`, v)
			case kind < 5:
				result = heap.Pop(&h).(int)
				op = `("pop", 0, 0)`
			case kind < 6:
				i := rg.Intn(len(h) + 2)
				op = fmt.Sprintf(`("remove", %d, 0)`, i)
				if i < len(h) {
					result = heap.Remove(&h, i).(int)
				}
			default:
				i := rg.Intn(len(h))
				v := rg.Intn(vals)
				h[i] = v
				heap.Fix(&h, i)
				op = fmt.Sprintf(`("fix", %d, %d)`, i, v)
			}
			var arr []string
			for _, x := range h {
				arr = append(arr, fmt.Sprint(x))
			}
			ops = append(ops, op)
			res = append(res, fmt.Sprintf("(%d, [%s])", result, strings.Join(arr, ", ")))
		}
		o.WriteString(fmt.Sprintf("  ([%s], [%s]),\n", strings.Join(ops, ", "), strings.Join(res, ", ")))
	}
	o.WriteString("]\n\n/// (a slice, the heap array heap.Init makes of it)\nconst INITS: List[(List[Int], List[Int])] = [\n")
	for k := 0; k < 120; k++ {
		n := rg.Intn(40)
		h := make(refHeap, n)
		var in, out []string
		for i := range h {
			h[i] = rg.Intn(30)
			in = append(in, fmt.Sprint(h[i]))
		}
		heap.Init(&h)
		for _, x := range h {
			out = append(out, fmt.Sprint(x))
		}
		o.WriteString(fmt.Sprintf("  ([%s], [%s]),\n", strings.Join(in, ", "), strings.Join(out, ", ")))
	}
	o.WriteString("]\n\n")
	o.WriteString(heapTests)
	return o.String()
}

const heapTests = `fn lt(a: Int, b: Int) -> Bool { a < b }

test "a heap's array matches container/heap after every operation" {
  var bad: List[Int] = []
  var ri = 0
  for (ops, want) in RUNS {
    ri += 1
    var h = heap.new(lt)
    var k = 0
    for (kind, i, v) in ops {
      var got = -1
      if kind == "push" {
        h.push(v)
      } else if kind == "pop" {
        got = h.pop() ?? -1
      } else if kind == "remove" {
        got = h.remove(i) ?? -1
      } else {
        h.fix(i, v)
      }
      let (wres, warr) = want[k]
      if got != wres || h.to_list() != warr {
        bad.push(ri * 1000 + k)
        break
      }
      k += 1
    }
  }
  expect(bad) == []
}

test "from_list is heap.Init" {
  var bad: List[Int] = []
  var i = 0
  for (input, want) in INITS {
    i += 1
    let h = heap.from_list(input, lt)
    if h.to_list() != want {
      bad.push(i)
    }
  }
  expect(bad) == []
}

struct Job {
  due: Int
  name: Str
}

test "moved reports each element's position, so a job can be cancelled and rescheduled" {
  let pos = Mutex[Map[Str, Int]].new({})
  var h = heap.tracked(
    fn(a: Job, b: Job) -> Bool { a.due < b.due },
    fn(j: Job, i: Int) { pos.with(fn(var m: Map[Str, Int]) { m[j.name] = i }) },
  )
  for (n, d) in [("a", 50), ("b", 20), ("c", 40), ("d", 10), ("e", 30), ("f", 60)] {
    h.push(Job{due: d, name: n})
  }
  expect(h.peek().map(fn(j) { j.name })) == Some("d")
  // every job's reported position holds that job
  for n in ["a", "b", "c", "d", "e", "f"] {
    let i = pos.with_read(fn(m: Map[Str, Int]) -> Int { m.get(n) ?? -1 })
    expect(h.get(i).map(fn(j) { j.name })) == Some(n)
  }
  // cancel "b", then move "a" to the front
  let ib = pos.with_read(fn(m: Map[Str, Int]) -> Int { m.get("b") ?? -1 })
  expect(h.remove(ib).map(fn(j) { j.name })) == Some("b")
  let ia = pos.with_read(fn(m: Map[Str, Int]) -> Int { m.get("a") ?? -1 })
  h.fix(ia, Job{due: 5, name: "a"})
  var order: List[Str] = []
  while true {
    match h.pop() {
      Some(j) => order.push(j.name)
      None => break
    }
  }
  expect(order) == ["a", "d", "e", "c", "f"]
}

test "an empty heap, out of range positions and a copy" {
  var h = heap.new(lt)
  expect(h.pop()) == None
  expect(h.peek()) == None
  expect(h.remove(0)) == None
  expect(h.get(0)) == None
  expect(h.is_empty()) == true
  h.push(3)
  h.push(1)
  expect(h.remove(5)) == None
  expect(h.remove(-1)) == None
  var c = h
  c.push(0)
  expect(h.len()) == 2
  expect(c.len()) == 3
  expect(c.peek()) == Some(0)
  expect(h.peek()) == Some(1)
}
`
