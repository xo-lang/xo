# 0046: Reference counting with Perceus style ownership on the SSA IR

Status: Accepted (2026-10-08). Implemented in `internal/ssa/rc.go`
(`Module.InsertRC`), checked by `internal/ssa/verify_rc.go`, lowered by
`internal/llvm/fromssa.go`, runtime in `std/llrt/xo_rt.c`. Evidence:
`bench/results/llvm-rc-2026-10-08.md`.

## Context
The LLVM backend prototype (decision 0040, `bench/results/llvm-prototype-2026-10-08.md`)
never freed memory (a bump allocator) and kept list value semantics with
a sticky "shared" flag, so the first write after every copy cloned the
list even when the other copy was dead, and list heavy runs used up to
2.9x Go's memory. Decision 0045 added a shared SSA IR with block
parameters and explicit value semantics as the place to insert reference
counting. Xo has no mutable references, so values cannot form cycles
(except through future `Mutex`/`Atomic` handles), which makes reference
counting complete for the current language.

## Decision
Memory of the native backends is reference counted, with ownership made
explicit in the SSA IR by a pass after building (`InsertRC`), following
Perceus (Reinking, Xie, de Moura, Leijen, PLDI 2021) and Lean 4:

1. **Leaves, not aggregates.** Structs and tuples are values (LLVM
   structs in registers), not heap cells. Ownership is tracked per heap
   leaf: a `Str`, a `List`, or an enum with a heap payload, addressed by
   a root value and a field path (`v12.1.0`). A `field` read of a struct
   or tuple is an alias of its root's leaves. So taking a list out of the
   tuple a `var` call returns, or out of a struct that is rebuilt with
   `setfield` (`b.items.push(x)`, `with`), moves it with no count
   traffic.
2. **Roots.** A root owns its leaves from its definition (call results,
   constructors, block parameters, owned parameters, `reuse` results),
   except projections out of a heap container (`list.index`, `list.get`,
   enum `payload`), which borrow from the container and are retained
   when they must outlive it or are consumed, and borrowed parameters and
   string literals, which are always alive and never released.
3. **Operand modes.** Constructors (`struct`, `tuple`, `variant`),
   `setfield` (every leaf but the replaced field), edge arguments, `ret`,
   list updates (`list.set`, `list.push`, `list.pop`), the left operand of
   `str.concat`, and owned parameters of user functions consume;
   everything else borrows.
4. **Insertion.** Backward liveness over leaves. A consuming last use
   takes ownership; other consuming uses get a `retain` before; a
   borrowing last use gets a `release` after; a dead definition is
   released at once; a leaf live into one successor of a branch and not
   the other is released on that edge (the edge is split when the target
   has other predecessors or takes arguments). Paths that end in a fault
   or `unreachable` release nothing.
5. **Borrow inference.** A non `var` parameter is borrowed when no path
   of its function consumes it (to a fixpoint over the call graph,
   because passing it to an owned parameter consumes it). Callers keep
   ownership of borrowed arguments; a call that both consumes and borrows
   one value retains it first, so `append_all(xs, xs)` with one `var`
   parameter cannot see its own writes (golden `var_alias`).
6. **Cancellation.** `retain x ... release x` in one block is removed
   when nothing between consumes, retains, releases, or reuses x or the
   container x was projected from.
7. **Reuse (in place updates).** A list update needs a uniquely owned
   list. A list is provably unique when it is fresh from `list.new`,
   `list.set`, `list.push`, `list.pop`, or `reuse`, moved through
   constructors and block parameters (optimistic fixpoint), and never
   retained in the function: then the update writes in place with no
   check. Otherwise `reuse` is inserted before the update: a run time
   count check that passes the same cell on (count 1) or copies it and
   releases the original. A `var` List parameter of a function that
   updates lists is made unique once at entry, so loops over it run
   without checks. Strings: `str.concat` consumes its left operand and
   the runtime appends in place (doubling) when its count is 1, so
   `s = s + x` in a loop is linear.
8. **Verification.** When `Module.RC` is set, `Verify` simulates counts
   per leaf on every path: nothing used after it is consumed, no release
   of what is not held, predecessors agree on what is held at each block,
   every count is zero at `ret`, a loop does not redefine a value still
   held, and every list update gets a list held once that is a `reuse`
   result or provably unique. A test removes each inserted retain and
   release in turn and requires the verifier to reject the result.
9. **Runtime.** Every heap object starts with an i64 count; literals
   have an immortal count (2^60; INT64_MIN and never written since
   decision 0058). A size class allocator with free lists
   (16 byte classes to 1 KiB, powers of two to 256 KiB, malloc above)
   replaces the bump allocator, with a live allocation counter
   (`XO_LEAKCHECK=1` prints it at exit; the tests require zero).

## Consequences
- The LLVM backend frees memory deterministically and updates lists and
  strings in place when unique. On the bench programs peak RSS is now at
  or below Go's (sieve 53 MB against Go's 49, from 136; sort 78 against
  78 to 96, from 150; a string workload 16 MB against Go's 28, from
  10.4 GB), and times are equal or better than the prototype.
- Programs outside the SSA subset (generics, closures, failing
  functions, `==` on aggregates, recursive types, some List methods)
  still use the AST path, which never frees; `xo build` says so.
  Update 2026-10-08: all of these lower to SSA now (decision 0048).
- Counts are not atomic: the runtime is single threaded today. Tasks
  (structured concurrency) will need atomic counts for values that cross
  tasks, or a per task heap with copying at boundaries. Update
  2026-10-08: decision 0058 (counts stay plain until a value is marked
  shared; shared counts are atomic).
- Perceus "drop reuse" of a freed cell for a new constructor of the same
  size is not done: structs and enums are unboxed, so constructors do
  not allocate; a released list followed by `list.new` goes through the
  allocator's free list instead.
- Recursive types need boxes in the SSA lowering before they can be
  reference counted (a box is one more leaf kind). Done 2026-10-08,
  decision 0048 (with closures and generics).

## Alternatives
- **Tracing GC** (Boehm, or precise with stack maps via `gc.statepoint`
  or a shadow stack): no count traffic, but pauses, no deterministic
  frees, and no in place update of unique values, which is where Xo's
  value semantics pay off. Rejected for the native backend; the Go
  backend keeps Go's GC.
- **Counts on whole values** (retain a struct, generated per type
  helpers walk it): simpler, but moving a list out of a struct or tuple
  becomes retain plus release, the list is never seen as unique, and
  every `var` call result update copies.
- **Static uniqueness only** (no run time `reuse` check): misses every
  value coming from a parameter or a call; the check is one load and a
  well predicted branch.
- **Keep the sticky shared flag**: never frees and clones after every
  copy even when the other copy is dead.
