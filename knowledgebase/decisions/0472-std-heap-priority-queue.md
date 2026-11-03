# 0472: std/heap, a priority queue with Remove and Fix (proposal)

Status: Proposed

## Context

The 2026-10-30 backend survey (std-coverage.md, rank 13) found that a scheduler (timers, retries, a
job queue ordered by due time) needs a priority queue whose entries can be removed or re-prioritized:
cancelling a timer, moving a job's due time. Go's answer is `container/heap` (Init, Push, Pop, Fix,
Remove over a `heap.Interface` of Len, Less, Swap, Push, Pop). Xo has no such type, and `List.sort_by`
after every push is O(n log n) per operation. Go's interface needs a user type with five methods and
Go's idiom for Fix and Remove is that the element stores its own index, kept by `Swap`. In Xo values are
copied and there are no pointers to elements, so the index has to be reported by the heap.

## Options

1. `std/heap`, a binary min-heap by a comparison the caller gives, with a callback that reports each
   element's index when it moves (Go's `Swap` bookkeeping, without the interface):

   ```xo
   struct Heap[T] {}
   pub fn new[T](less: fn(T, T) -> Bool, moved: fn(T, Int) = ...) -> Heap[T]   // less(a, b): a comes out before b
   pub fn from_list[T](items: List[T], less: fn(T, T) -> Bool) -> Heap[T]       // heap.Init, O(n)
   fn (var h: Heap[T]) push(x: T)                 // O(log n); heap.Push
   fn (var h: Heap[T]) pop() -> T?                // the least, removed; None when empty; heap.Pop
   fn (h: Heap[T]) peek() -> T?                   // the least, kept
   fn (h: Heap[T]) get(i: Int) -> T?              // the element at heap position i
   fn (var h: Heap[T]) remove(i: Int) -> T?       // heap.Remove: the element at position i; None out of range
   fn (var h: Heap[T]) fix(i: Int, x: T)          // heap.Fix: replace the element at i by x and restore the order
   fn (h: Heap[T]) len() -> Int
   fn (h: Heap[T]) is_empty() -> Bool
   fn (h: Heap[T]) to_list() -> List[T]           // in heap order, not sorted
   ```

   `moved(x, i)` is called each time an element lands at position `i` (a push, a sift, a remove), so a
   caller keeps `Map[Key, Int]` of positions for `remove` and `fix`, or ignores it. Equal elements come
   out in an unspecified order, as Go's heap (not stable).
   Cost: `std/heap.xo`, a native implementation (about 90 lines: sift up and down as Go's heap.go),
   Go backend through `std/go_compiled.txt` if a generic struct holding a function compiles there (else
   a small std/rt/heap.go), spec/std.md section, std/changes, corpus (random operation sequences against
   Go's container/heap on a slice of ints, with Remove and Fix by position, comparing every pop and the
   heap array after each operation, since Go's algorithm is deterministic and ours ports it), a timing
   row against container/heap.
2. Only `push`, `pop`, `peek` (no `remove`, `fix`, `moved`): smaller, and enough for a plain job queue,
   but a cancellable timer then needs lazy deletion, the survey's very complaint.
3. A heap that stores keys with priorities (`push(key, priority)`, `update(key, priority)`,
   `remove(key)`): the nicest for schedulers, but it needs `Eq + Hash` keys and a map, so a second
   type; it can be built over option 1 later.
4. Nothing: services keep `List.sort_by` or a hand-written heap.

## Recommendation

Option 1: it is Go's heap.go algorithm (so the corpus can compare the whole array, not only the pops)
with the interface replaced by a comparison function and the `Swap` bookkeeping by `moved`.

## Consequences

A new public module (hence this ADR). The comparison is stored in the heap, so a `Heap` holds a closure
and is not `Eq`, `Hash` or `Json`. A `Heap` is a value as `Deque` is: a copy shares storage until one is
written. Nothing in the prelude changes.

## Alternatives

Listed above (2, 3, 4). Also rejected: a `heap.Interface` interface (five methods per user type), which
Xo's by-value structs cannot back with an index the heap owns.
