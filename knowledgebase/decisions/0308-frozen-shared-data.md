# 0308: Frozen shared data: `Shared[T]`, read without counts or locks, reclaimed at safe points

Status: Deferred (owner, 2026-10-21): measured, it buys nothing over
encoding inside the lock (see the addendum "Deferred: the evidence").
Was Accepted (owner, 2026-10-21). Reference branches: frozen-s1 (stages 1
and 2 and the elision prototype, local to Xo Lang 2) and
wip/0308b-proto on origin (the bench variants and scale-0308.sh).

## Context

The owner approved 32-core scale run (CI run 39608730597, servers on 16
cores, 1024 connections, bench/perf/http) measured the native CRUD
server at 10.4k requests a second against Go's 164.5k, 126 against 39
microseconds of CPU a request. Its CPU profile
(~/xo-lead/notes/lead/scale-ci2/xo-cpu.txt) has 65% of the server's CPU
in three reference count helpers: `dupp User` 39.7%, `dup Str?` 13.7%,
`drop Str` 11.6% (34% on 12 shared vCPUs in Docker, the same helpers:
the cost grows with cores). GET /users runs
`s.state.with_read(fn(st) { st.users.values().sort_by(fn(u) { u.id }) })`:
every request copies every User (about 1,032 at 1024 connections) out of
the Mutex's shared read, and every copy is an atomic increment, and later decrement,
of a count word all cores share (decision 0058 marks a value shared when
it crosses into a Mutex). The cache line moves between cores on every
operation. Go copies the same records under its RWMutex at no count
cost. The counts are required by the language as it is: the copies
outlive the lock, so no borrow elision applies (the alternative first
considered, eliding counts for a collection borrowed for a whole loop,
does not fit this path).

The owner asked for explicit, opt in frozen values: deeply immutable,
copied on any thread with no atomic operation, reclaimed when replaced
after every task that might still read them has passed a safe point
(read, copy, update: RCU), rather than automatic sticky counts (an
object made immortal on sharing is never freed, so a long running server
that briefly shares data would grow without bound).

Two facts shape the design:

1. The count encoding already has a free, never written, range. Decision
   0058's count word is `c > 0` thread local, `c = -n` shared (bit 62
   set), and `INT64_MIN` immortal; every retain and release, in the
   arm64 and amd64 emitters (compiler/arm64/rc.xo `tbz t, #62`), the
   LLVM backend (compiler/llvm/gen.xo SHARED_BIT) and llrt (rc.xo `inc`,
   `dec`), treats any negative count with bit 62 clear as immortal: it
   reads the word and writes nothing. A count of bit 63 set, bit 62 clear
   and nonzero low bits is therefore already copied and dropped for free
   by every backend, with no code generation change on the hot paths.
2. Reclaiming at safe points is unsound if a frozen pointer can outlive
   the read: with value semantics, `let u = users[3]` copies a pointer,
   and if that copy is free it is also untracked. A task that keeps it
   past its next park (a response written after the read) would read
   freed memory. The design below keeps frozen pointers inside the read.

## Decision

1. **`Shared[T]`** (std, beside `Mutex[T]` and `Atomic[T]`, spec core
   5.4): a cell holding a frozen version of a value, for data read much
   more often than written.

   ```
   let users = Shared[Map[Int, User]].new({})
   let n = users.read(fn(m) { m.len() })                     // no lock, no counts
   users.update(fn(var m) { m.put(u.id, u) })                // writers serialized
   let body = users.read(fn(m) { json.encode(m.values()) })  // serialize inside the read
   ```

   - `s.read(f: fn(T) -> R ! E uses fx) -> R ! E uses fx`: f gets the
     current version read only. Readers take no lock and change no count.
     Copies of the version, or of any part of it, inside f are pointer
     copies whose retain and release do nothing (item 3).
   - `s.update(f: fn(var T) -> R ! E uses fx) -> R ! E uses fx`: writers
     are serialized by a lock of their own (readers never wait for it).
     f gets the current version; changes copy on write as for any value
     (a frozen object's count is never 1, so an in place update always
     copies the touched path and shares the untouched parts). When f
     returns successfully the new value is frozen and published, and the
     old version is retired (item 5); an error or fault publishes nothing.
   - `s.get() -> T` is `s.read(fn(v) { v })`: a thawed copy (item 4).
   - There is no `freeze(v)` expression or `Frozen[T]` value type in this
     decision: frozen data lives only inside a `Shared` cell, so the
     runtime always knows which version a reader may be using. A
     standalone `Frozen[T]` handle would itself need a count (one atomic
     operation a copy, on one line all cores share) or the same
     reclamation scheme with escaping handles; it can follow if a use
     needs it.

2. **What may be shared.** T is any value type whose values are
   immutable all the way down: Int, Float, Bool, Str, Bytes, List, Map,
   Set, tuples, user structs and enums of those, `T?`, and function
   values (their environment is frozen with them). Not allowed (a new
   checker error, `XO` code claimed at implementation): `Mutex`,
   `Atomic`, channels, task handles, capabilities, `Handle`
   kinds, and `use go` values, which have identity or mutable state that
   freezing cannot make immutable. Writes through the read only view are
   already impossible (value semantics, XO0502); no other write rule is
   needed.

3. **The frozen count.** A frozen object's count word is
   `INT64_MIN | k`, k >= 1: bit 63 set, bit 62 clear, k in the low 62
   bits the number of versions (of any cell) that reference the object.
   - Every existing retain and release treats it as immortal: nothing is
     written, the cache line stays shared by all cores (a read of a
     shared line costs no coherence traffic).
   - k changes only under a cell's writer lock, when a version is
     published (k + 1 for each frozen object the new version shares with
     an older one) or reclaimed (k - 1; at 0 the object is freed and its
     children are released the same way). Persistent data structure
     counting: a publish touches only what f changed, plus one k + 1 for
     each frozen child the changed path still points to, and a
     reclamation stops at children other versions still hold.
   - `INT64_MIN` itself (k = 0) stays the immortal literal count.
   - Freezing a new version walks only its thread local objects (count
     > 0, created or copied by f): each gets `INT64_MIN | 1`. An object
     already shared but not frozen (count < 0 with bit 62 set: a value
     that crossed into the cell from a capture of `update`'s closure) is
     copied first, so no frozen object is ever reachable from a counted
     holder outside the cell. Freezing is done by generated `freeze`
     helpers per type, as decision 0058's `share` helpers are.

4. **Thaw on exit.** No frozen pointer outlives the read that obtained
   it:
   - When `read`'s or `update`'s closure returns, its result R is
     thawed: generated `thaw` helpers (per type, like `share`) walk R and
     copy every frozen object reachable from it into ordinary thread
     local memory. A result built from scratch (a count, a serialized
     Str or Bytes) has nothing frozen and costs one walk of R.
   - Inside the closure, every crossing that today marks a value shared
     (OpShare: a channel send, a `spawn` capture, a store into a Mutex,
     Atomic, or another Shared cell) thaws instead of marking when it
     meets a frozen object.
   - Closures capture by value and cannot assign a captured `var`
     (XO0502), so the closure cannot store a frozen pointer outside
     itself any other way.
   So reclamation needs to know only which reads are in progress, never
   where a pointer went. The cost of thawing is allocation in the
   reader's own heap (no atomic operation, no shared line), proportional
   to what escapes: the idiom for the hot path is to serialize inside the
   read (the third line of item 1's example), which thaws nothing.

5. **Reclamation (RCU with quiescent states).** A retired version is
   freed (its objects' k decremented, item 3) once every reader that may
   have seen it has finished:
   - **Workers' quiescent points.** Each worker bumps a quiescent counter
     of its own (a plain store to its own cache line) at every scheduling
     point where the fiber it runs is not inside a read: park, yield,
     preemption (decision 0286 step 3), and when it goes idle.
   - **A read in progress** sets a flag in its fiber (a plain store at
     entry and exit, no shared write). A fiber that parks, yields, or is
     preempted with the flag set is not quiescent: it adds itself to a
     per epoch count of suspended readers (an atomic operation only on
     that rare path) and removes itself when its read ends.
   - **The grace period.** A writer that retires a version stamps it with
     the current global epoch and advances the epoch. The runtime's
     monitor thread (pool.xo mon_scan, every 10 ms) frees a retired
     version once every worker's quiescent counter has moved since the
     stamp and the suspended reader count of its epoch and every older
     one is zero. A writer's retire never waits.
   - **Memory bound.** Retired versions wait at most one scheduling slice
     (10 ms with preemption on, decision 0286; a loop polls) plus the
     longest suspended read. To bound a read that parks for long (I/O
     inside `read`), a writer parks when the cell's retired bytes exceed
     `XO_SHARED_RETIRED_MAX` (default the larger of 64 MiB and twice the
     live version's size) until reclamation frees some: backpressure, as
     Linux's synchronize_srcu, never unbounded growth. A warning (vet
     code claimed at implementation) flags I/O and channel operations
     inside a `read` closure, the cause of long suspended reads.
   - Under `XO_PREEMPT=0` a worker running a long loop without polls
     delays every grace period until it reaches a scheduling point; the
     backpressure limit keeps memory bounded then too.

6. **Go backend.** `Shared[T]` is a struct of a `sync.Mutex` for writers
   and an `atomic.Pointer[T]`: read loads the pointer and calls f, update
   copies the value (the Go backend's value semantics), calls f on the
   copy, and stores the new pointer. Freeze and thaw are no-ops; the
   garbage collector reclaims old versions when no reader holds them.
   Programs print the same on every backend.

7. **Interaction with the existing primitives.** `Mutex[T]`'s `with`
   and `with_read` keep their semantics and costs; `Shared[T]` is for
   read mostly data, where it also removes the reader lock (the
   `with_read` reader grants were 4.6% of the Linux profile and, on macOS,
   one wake a granted reader). A value read in `s.read` and used in a
   later `s.update` gets XO0804 (split critical section) as for Mutex.
   Arenas (core 5.5) are unaffected: a thawed result is ordinary memory.

8. **Implementation stages**, each gated by program output goldens on
   every backend, the leak checker, TSan, and the fixed point:
   1. Runtime: the frozen count in llrt (freeze, thaw, retire walks), the
      generated `freeze` and `thaw` helpers in the arm64, amd64, and LLVM
      emitters, and an assertion that no hot path changed (the existing
      immortal branch); a golden that freezes, reads, and thaws.
   2. `Shared[T]` in std with the reclamation of item 5 in pool.xo
      (quiescent counters, fiber read flag, suspended reader counts,
      monitor reclamation, backpressure), the Go backend's type, spec
      core 5.4 and std.md.
   3. Checker (Xo Publish): item 2's type rule, XO0804 for Shared, the
      vet warning of item 5.
   4. Measurement: the CRUD server's store on `Shared` (a variant beside
      bench/perf/http/xo, so the Mutex version stays comparable with
      Go's), Linux in Docker at 64 and 1024 connections, and the 32-core
      CI run.
   5. Separately (its own decision): a persistent hash map (HAMT) so an
      update of one entry copies O(log n) nodes instead of the map's
      buffer (with a plain Map, an update copies the buffer's n pointers
      and adds 1 to n children's k).

## Consequences

- Reads of shared data do no atomic operation and take no lock; their
  cost no longer grows with cores. Expected payoff: the count helpers'
  65% of the 32-core server's CPU and the reader grants. The
  immortal-strings measurement (scale-rc-payoff.md, Xo Lang 2's notes:
  the same server built with every shared object made immortal, which is
  exactly the read cost of item 3 without the thaw) gives the measured
  upper bound; its figures are added here when the run completes.
- Updates cost more than with a Mutex: a copy of the changed path, a
  freeze walk of it, and later a reclamation walk; with a plain Map, O(n)
  pointer work an update until the persistent map (stage 5). Write
  heavy data should stay in a Mutex.
- A result that escapes a read is copied (thawed). Code that copies
  large parts of a Shared value out of every read pays allocation
  instead of contention; serializing or aggregating inside the read
  avoids it.
- The runtime gains per worker quiescent counters, a fiber flag, and
  monitor work; the hot retain and release paths are unchanged.
- Memory: retired versions live up to one grace period (about 10 ms)
  plus the longest suspended read, bounded by `XO_SHARED_RETIRED_MAX`
  with writer backpressure.
- A new env var `XO_SHARED_RETIRED_MAX` (classified in
  internal/xotool/codegen_test.go as runtime only), a checker error code
  and a vet code.

## Alternatives

- **Sticky counts (2b)**: an object becomes immortal when shared from
  many threads. No new syntax, but never freed: a long running server
  that shares short lived data grows without bound. Rejected by the
  owner (2026-10-21).
- **Make every shared object immortal**: the measurement hack; leaks
  everything shared. A bound, not a design.
- **Elide counts for loops over a borrowed collection** (considered
  first): sound, but the hot path's values escape the lock, so it does
  not apply here.
- **Biased reference counting** (Swift's research, CPython's free
  threading PEP 703): the owning thread counts without atomics, others
  atomically. Hot data read by all cores is owned by one, so the others
  still contend on one line.
- **Deferred reference counting with stack scanning**: references from
  stacks are not counted, a collector scans stacks before freeing. Needs
  precise stack maps the native backends do not have, and pauses.
- **`freeze(v)` and a standalone `Frozen[T]` value**: the handle needs a
  count (one contended atomic operation a copy) or escape tracking;
  deferred until a use needs it (item 1).
- **Reclamation without thaw** (RCU at safe points alone): unsound with
  value semantics, as a frozen pointer copied out of a read can outlive
  any safe point (Context, fact 2).
- **SRCU style per worker read counters** instead of quiescent states:
  sound without the fiber flag, but an atomic operation at every read's
  entry and exit (on a worker local line, cheap, but not zero), and two
  counter arrays per cell. Kept as the fallback if quiescent state
  tracking proves fragile.

## Addendum (2026-10-21, owner: option (b)): reads that touch no counts

**Measurement that changed the design.** Docker linux/arm64, 12 vCPUs
shared with the client; the CRUD server (bench/perf/http) under a
list-only load (1024 keep-alive connections each looping GET /users over
1,032 users), XO_PREEMPT=0, fresh XO_PROF profiles:

| server | requests in 8 s | drop Str | dup Str? | dropp User |
|---|---|---|---|---|
| main | 70,978 | 33% | 16% | 4% |
| the store's Mutex value frozen (stage 1 helpers, a local hack) | 48,566 | 29% | 17% | 9% |

The runtime sentinel did not reduce the count helpers' time: at this
core count the cost is reading each of ~3,100 string objects' count
words a list request (its cache line, one per object, after the
encoder's pass or before it), not the atomic write. A frozen count is
still read. (Not measured yet: 16 and more cores, where atomic writes
also move each line between cores; frozen counts stay clean.)

**Redesign.** Reads of a Shared cell touch no count at all, decided at
compile time:

1. Inside a `read` closure the view and every value reached from it
   (fields, payloads, elements by index or iteration, map lookups, and
   the values those are moved or copied into while they stay in the
   closure) are frozen by construction. The reference counting pass
   (compiler/ssa rc.xo) marks the closure's view parameter frozen; a
   retain or release of a value derived from it is omitted. A copy that
   leaves the closure is still thawed (item 4), so only escaping data
   costs anything.
2. Code the closure calls does not know its argument is frozen (a std
   function such as `Map.values` or `sort_by` retains what it copies):
   stage 1 of this addendum covers the closure body only; callees get
   the same treatment later by specializing a function for a frozen
   argument (a copy of it compiled with that parameter marked), the way
   generic functions are instantiated.
3. The runtime sentinel (item 3) stays: it makes the omitted operations
   safe for values that reach code that was not specialized (a callee's
   retain of a frozen object changes nothing), and thaw (item 4) and
   reclamation (item 5) use it.
4. The idiom that gets the whole benefit: serialize or aggregate inside
   the read (`s.read(fn(st) { json.encode(st.users) })`), so the result
   is one new string and nothing frozen escapes.

**Next.** A prototype of point 1 for one closure (the CRUD server's list
read, the store moved to a `Shared` cell in a variant beside
bench/perf/http/xo), measured on Docker against main and Go under the same
list-only load; then the 32-core CI A/B the owner approved.

## Addendum: Deferred (owner, 2026-10-21), the evidence

All three measurements use the CRUD server (bench/perf/http) under a
list-only load: every keep-alive connection loops GET /users over 1,032
users.

1. **The runtime sentinel alone (12 vCPUs, Docker).** The store's Mutex
   value frozen by stage 1's helpers: 48,566 requests against main's
   70,978; the count helpers' share did not fall (drop Str 29% against
   33%). The cost is reading ~3,100 scattered count words a list request,
   not contention, and a frozen count is still read.
2. **The (b) prototype (12 vCPUs, Docker; two runs each).** Shared with
   the frozen view's counts elided, GET /users encoded inside the read:
   119,802 and 126,956 requests; the same restructure on Mutex (encoded
   inside with_read): 141,996 and 140,830; main 50,736 and 52,652; Go
   52,330 and 49,178. The 2.7x is the restructure, available with Mutex;
   Shared was 10 to 15% behind it.
3. **32 cores (CI run 39622568915, 16 server cores; requests in a 15 s
   round, two rounds each; ~/xo-lead/notes/lead/ci-0308):**

| connections | main | Mutex, encoded inside | Shared + elision | Go |
|---|---|---|---|---|
| 256 | 209,419 / 207,592 | 1,273,570 / 1,275,515 | 1,276,735 / 1,282,688 | 462,321 / 451,498 |
| 1024 | 207,419 / 205,161 | 1,380,396 / 1,372,886 | 1,376,460 / 1,388,409 | 511,691 / 510,998 |
| 4096 | 208,692 / 209,036 | 1,365,766 / 1,372,654 | 1,374,959 / 1,369,583 | 546,032 / 526,268 |

   p99: Mutex-encode-inside and Shared alike (7 to 8 ms at 256, 30 to 31
   ms at 1024, 84 to 91 ms at 4096); Go 101 ms to 1.15 s. Shared is within
   1% of the Mutex restructure at every connection count: with nothing
   copied out of the lock, neither the read lock's cost nor the counts
   matter at 16 cores.

**What would reopen it:** a workload where the result cannot be built
inside the lock (values must outlive it, e.g. handed to slow consumers or
other tasks) and many cores read the same shared data at once, measured
on the 32-core runner against Mutex; or a profile where the RWMutex's
reader lock itself (its spin lock and grants) is a top cost.
