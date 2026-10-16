# 0133: Real-time functions (`uses realtime`), loop bounds, and deferred frees

Status: Accepted, implemented (2026-10-10). Spec: core 6.5 (new), 3.5
(`bound`), 10.6 (XO1401 to XO1405, XO-F006, XO-F007). Code:
`internal/check/realtime.go` (the rules), `internal/parser` and
`selfhost/parser` (`bound`), `internal/ssa/build_stmt.go` and
`internal/gen` (the bound check), `internal/ssa/build_call.go` (section
entry and exit), `std/llrt/xo_rt.c` (`xo_realtime_enter` and
`xo_realtime_exit`, forwarders since 2026-10-15 to
`std/native/llrt/rtsec.xo`; the deferred free queue is
`std/native/llrt/obj.xo` rt_defer and rt_drain). Tests: the
`internal/check/testdata/golden/realtime_*` goldens, the
`internal/llvm/testdata/golden/realtime_*` goldens (Go, LLVM, and arm64
agree), `internal/gen/testdata/golden/test_loop_bound` (fuzzing),
`internal/llvm/realtime_test.go` (TestRealtimeNative: XO-F007 cases,
deferral counts, the demo), `std/llrt/testdata/realtime.c`
(TestRuntimeRealtime). Demo: `examples/realtime-control`. Numbers:
`bench/results/realtime-2026-10-10.md`.

## Context
The owner asked (2026-10-09, low priority) for a real-time mode for the
native backends: no allocation after initialization, compiler enforced
bounded loops and no recursion in functions marked real-time, contracts
for loop bounds, frees that cannot stall a real-time section, and
allocation that a worst case execution time (WCET) analysis can reason
about. Reference counting (decisions 0046, 0058) frees memory at the
last release, so a store that drops the last reference to a long List
walks and frees the whole list inside whatever code did the store; that
is the one source of unbounded work a program cannot see in its own
source. Clang's function effects (`nonblocking`, `nonallocating`, 2024)
and RealtimeSanitizer are the closest precedent: a function attribute,
checked transitively at compile time, backed by a run time check.

## Decision
1. **The marker is `uses realtime`** on a function declaration (or
   method). `realtime` lives in the effect namespace and the uses clause,
   so the parser, formatter, and every agent's habit of where effects go
   are unchanged, but it is a *discipline*, not an effect: it is not part
   of the function's effect set and does not take part in subsumption. A
   real-time function has a declared effect set like any function with a
   uses clause (so it is checked, not inferred); besides `realtime` it may
   declare only `ffi` (C functions are trusted to be bounded). `realtime`
   in a closure, a function type, or a test is XO0702. The inverse design
   (an `alloc` effect every allocating function would have to declare)
   was rejected: nearly every function allocates, so it would put a label
   on all of them for the benefit of the few that must not.
2. **What a real-time function may not do** (checked on every backend,
   at the construct):
   - **XO1401, allocation:** list, map, and set literals, string
     interpolation, `+` on Str, List, or Bytes (and `+=`), slicing,
     closures, `m[k] = v` on a Map, and every prelude method not on the
     bounded list (growth such as `push`, `put`, `insert`, `add`, and
     every method that returns a new collection or Str). Allowed: `xs[i]`,
     `xs[i] = v` (in place when the List is unique), `len`, `get`,
     `contains`, the Int, Float, Rune, Duration, Option, Result, and
     Atomic methods that compute a scalar. The arguments of `fault(...)`
     are exempt: a fault ends the section.
   - **XO1402, unbounded loop:** `while` without `bound N`, the infinite
     `while { }`, and a `for` that is not over a range with constant ends
     and has no `bound N`. A bound must be a constant Int; a `for` over a
     constant range longer than its bound is XO1402 in any function.
   - **XO1403, unchecked call:** a call of a function that is not
     real-time (user, std, or Go bound), of a function value, or of an
     interface method (including methods of a type parameter's
     constraint). Calls of functions in a `use c` module are allowed.
   - **XO1404, recursion:** a cycle in the call graph of real-time
     functions, self calls included. Since rule XO1403 restricts calls to
     real-time functions, function values, and interfaces, this covers
     recursion through function values and interfaces too.
   - **XO1405, blocking:** channels, `Mutex.with` and `with_read`,
     `scope`, `select`, task spawn and await, `dbg`, and declaring an
     effect other than `ffi` (capability calls are interface calls and
     XO1403 already; this names the reason).
   - Faults stay allowed: overflow, index, and contract checks are
     bounded. A fault ends the real-time section (point 5).
3. **Loop bounds are contracts:** `for p in e bound N { }` and
   `while c bound N { }` (contextual word `bound` after the iterated
   value or condition; N a constant Int expression) are accepted in every
   function. A constant range is checked statically. Otherwise every
   backend counts iterations and faults with **XO-F006** when the loop
   starts iteration N + 1. `xo test` fuzzes a function with a bounded
   loop like one with `requires` (core 9.1), so a reachable violation is
   found and shrunk like a failed `ensures`. Bounds from `requires n <= K`
   on a parameter (the bound inferred from a precondition) are left for
   later.
4. **Initialization is everything else.** Allocation is allowed wherever
   no real-time function is running: `main`, constructors, and setup
   code preallocate. The fixed-capacity collection is a List built during
   initialization and written by index in the real-time function
   (Perceus reuse makes `xs[i] = v` in place on a unique List, decision
   0046). No pool type was needed: a fixed-size List of values is the
   pool, and an index the handle. `Buf` and `StrBuilder` stay
   growth-only and are not allowed in real-time code.
5. **Native runtime (all three backends check; only native enforces):**
   - A call from code that is not real-time to a real-time function is a
     *real-time section*: the builder brackets it with
     `xo_realtime_enter` and `xo_realtime_exit` (a per-thread depth;
     calls between real-time functions add nothing).
   - **Deferred frees.** Inside a section, a reference count that reaches
     zero (`xo_rc_free` for strings, `xo_rl_free` for lists,
     `xo_rc_defer` for boxes and closures, `xo_h_free` for handles) puts
     the object on a fixed queue of 256 entries per thread instead of
     freeing it and its children; the outermost exit frees the queue
     after the section's result exists. A full queue frees at once and
     counts an overflow (`XO_RTSTATS=1` prints it), so the queue never
     allocates.
   - **Allocation is a fault.** `xo_alloc` inside a section counts the
     allocation; the outermost exit faults with **XO-F007** naming the
     function when any happened (copy on write of a shared List, a
     recursive enum's box, a defaulted struct field: what the static
     rules cannot see). Counting instead of faulting at the allocation
     keeps contract failure messages, built just before their fault,
     intact.
   - A fault inside a section leaves it (depth reset, queue freed) before
     the fault unwinds. Native tasks are cooperative and a real-time
     function cannot block, so a section never moves between threads.
   - The Go backend checks the rules and the loop bounds; it has no
     sections (its collector does not free at a release).

## Consequences
- A program states its real-time part in signatures, and an agent gets
  stable codes for each violation. Code that is not marked is unchanged
  (one comparison of a thread local depth in `xo_alloc` and in the
  zero-count paths when a section never runs).
- The rules are syntactic and conservative: a call of a helper that is
  not marked is rejected even when it would not allocate; mark the
  helper. Prelude methods are on the bounded list or not, by name.
- Measured (bench/results/realtime-2026-10-10.md, examples/realtime-control,
  M4 Pro): on the ticks that drop a 20,001 object schedule, the output
  latency is 166 to 209 ns real-time against 30 to 114 us without the
  mark (the frees move to the section's exit, which costs the same or a
  little more); the worst output latency over 200,000 ticks is 1.2 to
  7.8 us against 39 to 114 us (one OS deschedule of 1.3 ms aside). The
  same controller written without the rules has a 60 times slower median
  tick (2.6 us against 42 ns). The section's own cost is below the
  clock's 42 ns resolution.
- Left for later: bounds inferred from `requires`; real-time function
  types and interface methods (so a callback can be called); real-time
  std functions beyond the prelude (std/math, std/time arithmetic);
  `clock` reads inside a section; stack depth bounds (the call graph is
  acyclic, so a bound exists, but nothing computes it); a WCET report
  (`xo vet --wcet`: loop bounds times instruction counts per function).

## Alternatives
- **An `alloc` effect that real-time code may not have**: see point 1.
- **Inference for unmarked callees** (Clang infers for inline
  definitions): local reasoning is lost and pub signatures would change
  meaning silently when a body changes; explicit marks were preferred.
- **Faulting at the allocation itself**: simpler, but a failed contract's
  message (built from strings) would be replaced by the allocation fault.
- **A pool or fixed-capacity collection type**: a preallocated List
  written by index already has the property; a new type would need
  std/rt, the native runtime, and the spec for no new guarantee.
- **Deferring frees to an explicit `realtime.flush()`**: one more thing to
  forget; the end of the outermost section is the natural point.

## Note (2026-10-16, decision 0241)

std/native/rt.xo declares the pure loads mem_load, slot_load,
static_word, and tlocal_get `uses realtime`, so the runtime's own
real-time functions (std/internal rt_queued and rt_allocs, obj.xo
rt_heap) may call them. rt is std internal; the rules for programs do
not change.
