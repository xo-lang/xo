# 0488: a write to a held-once shared container stays in place and shares what it stores

Status: Accepted (owner, 2026-11-04; proposed by Xo Lang 2 the same day).

## Context

Native counts: 1 is thread local, a negative count is shared (decision
0334's `share`), -1 a shared object with one holder. A list write on an
object that is not thread local goes through `reuse` (std/native/llrt
obj.xo rl_copy): `rc.claim` turns -1 into 1 after an acquire load and
the write happens in place; any other count copies.

`Mutex.with` hands its value back through `share` (`rt.mutex_unlock_put`,
decision 0482). `share` of a thread local container marks it shared and
then every element it holds (rc.mark returns 1, the caller marks the
children), because a thread local container may hold thread local
elements. So a `with` that writes one element of a value held in a Mutex
pays a claim (O(1)) and then a walk of the whole container (O(size)).

Measured on the Mac, 2026-11-04 (a `Mutex[List[Str]]`, one element
written per `with`, 20,000 times, three runs each):

| elements | 0.36.0 | main at eda09dfa0 |
|---|---|---|
| 1,000 | 0.07 to 0.08 s | 0.02 to 0.03 s |
| 100,000 | 8.23 to 8.94 s | 2.55 to 3.28 s |

about 127 us an unlock at 100,000 elements. Maps escaped it on main only
by going to the log form on their second shared write (maphyb.xo
maph_shared tests a count of 1 only); a change that let a map in a
Mutex take the lists' in-place write (Xo Lang 2's l2-cjfix, held) made
cookiejar 0.66 to 0.71 of its wall time and an HTTP server's 1,000 entry
session store 1.369 of it (host 1, n=5), the same walk.

## Decision (proposed)

A write to a container whose count is -1 (shared, one holder) does not
claim it. It writes in place and shares the element it stores (a write
barrier: `share` of the new element, O(element), usually a count store
on an element just made), so the container stays shared and the
unlock's `share` finds a negative count and returns at once (O(1)).

- The acquire that rc.claim does stays, once per write: the holder must
  see the writes of the thread that held the container before
  (TSan: the same sanhook.acquire rl_copy reports).
- Only -1 qualifies: a count below -1 has other holders and still copies.
- Where it applies: the list writes that go through reuse (set, push,
  pop, insert, remove, swap), the hybrid map's lists (maph_shared's -1
  case, l2-cjfix fb5c3eece), and a struct field set on a -1 struct. Not
  the map log's chunks: those are shared between a table's forks, so one
  holder of the directory is not one reader of a chunk (TestTSanSharedMaps
  failed with a claim there, 2026-11-04).
- Reads are unchanged.

## Consequences

- A `with` that writes k elements of a value of n costs O(k), not O(n).
- Every store into a -1 container shares the stored value: a store of a
  thread local Str marks its count negative, so its later drops are
  atomic. A loop that builds a large value inside one `with` pays an
  atomic count per element it stores (the cost the unlock's walk paid
  once per element before); measured before acceptance.
- The compiler's ownership passes must know a -1 container's element
  stores share (insert_rc), and the TSan and free check modes must
  agree (TestTSanSharedMaps, TestGoldenTasks under TSan, XO_FREE_CHECK=1
  goldens).

## Measurement plan

`xo bench --ab` on host 1: bench/perf/httpsession store, store100k and
plain; bench/perf/cookiejar both, get and set; a Mutex[List[Str]] row at
1,000 and 100,000 elements (bench/perf to add); and a build-inside-with
row (a `with` that pushes 100,000 elements once) for the write barrier's
cost. Self build wall and max RSS (xo builds with Mutexes).

## Alternatives

- Keep claiming, make `share` lazy (mark only the container, mark an
  element when another thread first reaches it): every read on another
  thread would test and mark, the cost moved to the readers.
- Keep claiming and skip the walk when the container's elements are
  already shared: needs a per container "all children shared" bit kept
  exact through every write, which is the write barrier above.
- Leave maps in the log form (main today): costs cookiejar 30%, and
  lists in a Mutex keep the O(size) unlock.
