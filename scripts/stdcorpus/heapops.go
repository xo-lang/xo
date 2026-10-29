package main

import (
	"container/heap"
	"fmt"
	"math/rand"
	"strings"
)

// The List.heap_* corpus (decision 0410): random sequences of push, pop,
// remove, fix (set then fix) and heapify on a list of ints, replayed
// through Go's container/heap on an IntHeap; the corpus holds each
// sequence, the values pop and remove returned in order, and the final
// list, which the Xo methods must reproduce exactly (the same swaps in the
// same order, so equal elements stay where Go leaves them).

type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func heapopsCorpus() string {
	var o strings.Builder
	o.WriteString(header("container/heap on random operation sequences"))
	o.WriteString("\n/// (the initial list, the operations (code, argument), the values pop and remove returned, the final list)\n")
	o.WriteString("/// codes: 0 push(arg), 1 pop, 2 remove(arg mod len), 3 set(arg mod len, arg / 7) then fix, 4 heapify\n")
	o.WriteString("const CASES: List[(List[Int], List[(Int, Int)], List[Int], List[Int])] = [\n")
	r := rand.New(rand.NewSource(11))
	for c := 0; c < 300; c++ {
		span := []int{3, 10, 100, 100000}[r.Intn(4)]
		init := make([]int, r.Intn(12))
		for i := range init {
			init[i] = r.Intn(span)
		}
		h := intHeap(append([]int(nil), init...))
		// the initial list is heapified first only when the sequence says so: it
		// starts as given (a heap or not), as Go's functions assume a heap, so
		// the first operation is heapify
		var ops [][2]int
		var got []int
		ops = append(ops, [2]int{4, 0})
		heap.Init(&h)
		for k, n := 0, 5+r.Intn(60); k < n; k++ {
			code := []int{0, 0, 0, 1, 1, 2, 3, 3, 4}[r.Intn(9)]
			arg := r.Intn(span)
			switch code {
			case 0:
				heap.Push(&h, arg)
			case 1:
				if len(h) == 0 {
					code = 0
					heap.Push(&h, arg)
				} else {
					got = append(got, heap.Pop(&h).(int))
				}
			case 2:
				if len(h) == 0 {
					code = 0
					heap.Push(&h, arg)
				} else {
					got = append(got, heap.Remove(&h, arg%len(h)).(int))
				}
			case 3:
				if len(h) == 0 {
					code = 0
					heap.Push(&h, arg)
				} else {
					i := arg % len(h)
					h[i] = arg / 7
					heap.Fix(&h, i)
				}
			case 4:
				// make it not a heap first is not allowed (Go's functions
				// assume one); Init on a heap changes nothing, but a
				// shuffle then Init is what heapify is for
				if len(h) > 0 {
					h.Swap(0, len(h)-1)
					arg = 1
				}
				heap.Init(&h)
			}
			ops = append(ops, [2]int{code, arg})
		}
		fmt.Fprintf(&o, "  (%s, [", xlist(init))
		for i, p := range ops {
			if i > 0 {
				o.WriteString(", ")
			}
			fmt.Fprintf(&o, "(%d, %d)", p[0], p[1])
		}
		fmt.Fprintf(&o, "], %s, %s),\n", xlist(got), xlist(h))
	}
	o.WriteString("]\n\n")
	o.WriteString(`
fn replay(init: List[Int], ops: List[(Int, Int)]) -> (List[Int], List[Int]) {
  var xs = init
  var got: List[Int] = []
  for (code, arg) in ops {
    if code == 0 {
      xs.heap_push(arg)
    } else if code == 1 {
      got.push(xs.heap_pop() ?? -1)
    } else if code == 2 {
      got.push(xs.heap_remove(arg % xs.len()))
    } else if code == 3 {
      let i = arg % xs.len()
      xs.set(i, arg / 7)
      xs.heap_fix(i)
    } else {
      // code 4 with argument 1: swap the first and last (breaking the heap
      // order the way the generator did), then heapify
      if arg == 1 && xs.len() > 0 {
        let a = xs[0]
        let b = xs[xs.len() - 1]
        xs[0] = b
        xs[xs.len() - 1] = a
      }
      xs.heapify()
    }
  }
  (got, xs)
}

test "List.heap_push, heap_pop, heap_remove, heap_fix and heapify are container/heap" {
  var bad: List[Str] = []
  var i = 0
  for (init, ops, want_got, want_xs) in CASES {
    let (got, xs) = replay(init, ops)
    if got != want_got || xs != want_xs {
      bad.push("case ${i}: got ${got.debug()} ${xs.debug()} want ${want_got.debug()} ${want_xs.debug()}")
    }
    i += 1
  }
  expect(bad) == []
}

test "heap_pop of an empty list is None" {
  var xs = [1, 2, 3]
  expect(xs.heap_pop()) == Some(1)
  var e: List[Int] = []
  expect(e.heap_pop()) == None
}
`)
	return o.String()
}

func xlist(xs []int) string {
	var b strings.Builder
	b.WriteString("[")
	for i, x := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d", x)
	}
	b.WriteString("]")
	return b.String()
}
