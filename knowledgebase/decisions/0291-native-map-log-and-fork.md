# 0291: A native Map whose copies cost O(1) (the Go backend's log and fork)

Status: Accepted (owner, 2026-10-20); switched on (owner, 2026-10-21):
native Maps and Sets are the hybrid of the addendum below by default,
`XO_MAP_LOG=0` builds the lists alone. Built behind a flag first; every
measurement in "Cost" is taken before the switch. No interim change to
`Mutex.with` (the owner, the same day): this decision is the fix.

Progress: stage 1 landed 2026-10-21 (f00c1950): the log representation
behind XO_MAP_LOG=1, off by default; goldens map_log_model and
map_log_sets match on arm64, LLVM, wasm and Go. Measured on the Mac
(--release arm64, n = 200,000, flag on against off): get 7 to 8 ns
against 4, iteration under 1 ns per entry for both, copy then put 75 ns
against about 100 us, put under `Mutex.with` 70 to 80 ns at any size
against 23 to 85 us. A table of 8 or fewer entries with no index keeps
no hashes (lazy hashing, eedeb18b4). HTTP callgrind (arm64, plain,
bench/perf/http callgrind-docker.sh with PRELOAD and XO_MAP_LOG, main
495fd9830, 3 runs each, instructions a request), flag on against off:
8 preloaded users 23,864 against 24,048 (-0.8%), 10,000 users 513,052
against 1,172,360 (-56.2%). The earlier figures (+4% at 8 users) were
taken before lazy hashing and on a build with an arm64 miscompile
(87059821 to e879670d) and are withdrawn. Next: the fixed point with
the flag on, the Mac measurement (collections, json, xo's self build),
then the switch.

## Context

The spec calls `Map[K, V]` an immutable persistent hash map with value
semantics (core 2.1). On the native backends (decision 0150 stage 3) a
Map is a struct of copy-on-write lists (keys, values, live flags, an
open-addressing table): a write to a Map that another value still shares
copies all of it, O(n).

That made the large-heap comparison (owner-approved 2026-10-19) never
finish: with `Mutex.with`, the packed closure keeps the value it was given
(ssa build.xo pack_orig) so that an error or a fault hands it back
(decision 0085), the Map is shared while the body writes it, and every
`put` copies the table. Measured 2026-10-20 (--release arm64, Mac), per
put inside `Mutex.with(fn(var m) { m.put(..) })`: 23 us at 10,000
entries, 170 us at 40,000 (a plain put: 38 to 52 ns at any size). The
bench store preloading users (after its own email scan was fixed,
d294c50b): 98 us per insert at 10,000 users, 280 us at 30,000; Go's
server 24 to 42 us flat. The Go backend's `with` stays flat (about 700 ns
per put at 10,000 and 40,000 entries).

The same cost hits any shared Map write: a Map copied into a struct and
changed, a Map captured by a closure and changed, a Map read by
`Mutex.with_read` while a writer changes it.

Today's costs (n = 200,000, Int keys, --release, two runs):

| | put | get | iterate, per entry | copy, then put | put, Str keys |
|---|---|---|---|---|---|
| native (lists) | 33 to 34 ns | 5 to 6 ns | under 1 ns | 110 us | 92 to 99 ns |
| Go backend (log and fork) | 42 to 51 ns | 14 to 15 ns | 1 ns | 93 to 98 ns | 104 to 106 ns |

## Decision (proposed)

Port the Go backend's Map representation (std/rt/map.go) to the native
backends, in place of the four lists:

- **A value is a header (table, view length, live count).** A table is an
  append-only log of entries (key, value, the previous entry of the same
  key, the origin that gives the key its insertion position; a removal is
  a tombstone) plus a hash index from each key to its newest entry.
- **Ownership by claim, as List's backing array:** a put through a header
  whose length is the table's claimed length takes the next position with
  a compare and swap and appends in place (amortized O(1), no copy). A
  stale header (another copy appended first) forks: a new table whose
  parent is the stale view, O(1); lookups search the fork's index, then
  the parent view by the prev links. Forks nest a bounded depth; deeper,
  or when overwritten entries and tombstones outnumber live keys, the view
  is flattened into a fresh table in O(n), amortized against the appends
  that made the garbage.
- **Soundness** is std/rt/map.go's argument (entries are never written
  after they are appended; the index is not part of any view; at most one
  value appends at a position), and its model tests (std/rt
  map_model_test.go: random operations on copies against a reference) are
  ported to a private native test.
- **RC interplay:** a header is a counted record like a List's; entries
  hold counted keys and values, released when a table's last header goes
  (a fork holds its parent view). `Mutex.with` keeps its rollback
  snapshot unchanged: holding the old header now costs O(1), and the
  body's puts append (the first one forks only if the snapshot header
  claimed the log, which it does not: the moved value claims).
- **Codegen:** the xo.map.* functions generated per type (compiler/ssa
  map.xo) are rewritten against the new layout; the hash functions per
  type stay. JSON, Debug, equality, iteration order and every Map method's
  results are unchanged (golden parity with the Go backend, which already
  has this representation, is the behavior test).

## Native design (2026-10-20, Xo Lang 3; as built)

The Go backend's tables are mutated in place while other headers read
them, and Go's collector keeps an outgrown array alive for a reader that
still holds it. The native port keeps the same views and claims, built
entirely in SSA (compiler/ssa maplog.xo) from Lists, with no new runtime
object:

- **A Map value is a struct of eight fields**: dir (List[List[E]], the
  entry log in chunks), idx (List[Int], the index), used (List[Int], one
  slot: the table's claimed length), base, n (the view's length), len
  (live keys), keys (the table's key count, for index growth), parents
  (List[Level], the views this table forked from, newest first). An entry
  E is xo.MapEntry {h, prev, org, k, v}; a removal is an entry with org
  -1. Copying a Map counts its Lists only.
- **Chunks never move.** Chunk c holds 8 << c entries; the directory is
  made with room for 40 chunks and never grows, and both are shared by
  every value of the table: a reader of a shorter view reads entries the
  owner appends, and an index slot may name an entry past a view (its key
  and prev are read, as in Go). The one new backend operation is
  `log.push` (arm64, LLVM, wasm): append into a List's spare capacity in
  place, never reallocating, so other holders see the element.
- **Claim and index are atomic**: rt.slot_cas (acquire/release) on used
  claims the next position; index slots are written with slot_store
  (release) and read with slot_load (acquire), so ThreadSanitizer sees
  the publication. The index is open addressing over positions + 1,
  built at 9 entries (twice the keys, at least 16 slots) and rebuilt
  when it fills.
- **Forks and flattening as in Go**: a stale header forks a fresh table
  whose parents are its view's levels (each a Level {dir, idx, base,
  len}); a lookup probes its own table, then each level, following prev
  below the level's length. At depth 8, or when garbage passes the live
  keys, the view is flattened into a fresh table.
- **Iteration** (xo.mlog.each): a root table with no garbage lists its
  entries in order; with garbage, Go's eachRoot (an entry is current when
  no later entry's prev names it, ordered by origin); a forked view walks
  the levels oldest first.

The flag (XO_MAP_LOG=1, Module.map_log, in the fe and fn cache keys)
lets goldens, the tests and the benchmarks run both representations
from one xo until the switch. Test programs (scratch, to land as goldens
with the switch): the model test (Go's map_model_test ported: 40 seeds
x 3000 random put, update, remove and copy steps over four values
against a list reference), a deep-fork test (1000 alternating forks,
gets unchanged at 10 ns), and a two-task race test under --race (no
TSan reports, each copy sees only its own writes).

## Cost

Estimated from the Go backend's figures and the native ones: a put gains
the claim's compare and swap and the log append (about 33 to 45 ns), a
get on an unforked table stays one index probe plus one entry load (5 to
8 ns), a get through a fork follows prev links (bounded by the fork
depth), iteration walks the log skipping overwritten entries and
tombstones (under 2 ns per entry while garbage stays under live keys).
Copying a Map and writing it goes from O(n) to O(1). To be measured on a
prototype before the switch: bench/perf/compute and the json benches
(Map-heavy decode), callgrind's CRUD scenarios, xo's own build (the
compiler is a heavy Map user: a regression there shows in every build),
and the cpuburst and large-heap runs.

## Consequences

- Shared-Map writes stop being O(n): `Mutex.with` over a large Map, Maps
  in structs that are copied, closures capturing Maps. The large-heap
  comparison then measures the runtime.
- One representation on every backend (Go and native), one soundness
  argument and one model test, kept in step.
- A Map's memory holds overwritten entries until it is flattened (at
  most about twice its live entries, as on the Go backend).
- The compiler itself changes speed (it uses Maps throughout); the
  fixed point and the cold-build measurement decide whether the switch
  lands as is or with a faster path for unshared Maps.

## Addendum (2026-10-21, Xo Lang 3): the hybrid representation

**Measured before the switch** (Mac, bench/perf collection workloads,
ms, best of 5; main 495fd983 lists against the log Maps of branch
mswitch): putget 23 -> 99, get 8 -> 21, update 23 -> 95, copymut 3 ->
26, iter about 1 (json `maps` +5% instructions). An unshared fast path
for the log (`log.set`: the table's only holder overwrites in place)
gave putget 50, get 19, update 45, copymut 22.

**Why fast paths cannot close it.** A log get follows index slot ->
chunk directory -> chunk -> entry and decodes the position twice (find,
then the value); a copy then write pays a flatten that rebuilds entries
and index where the lists copy one block. With a fused get and a flat
root chunk the floor is still about 1.6x on get.

**Design (map.xo, maphyb.xo; behind XO_MAP_LOG=1).** A Map or Set value
is the lists' struct (keys, vals, live, slots, n) with one more field,
`log`: in log mode a list holding one log table (xo.MapLog[...]) whose
n field 4 mirrors; in lists mode empty, the shared immortal empty list
until the value's lists copied on write once, then an empty list of its
own (the mark: six fields keep a put's map, key, and value in arm64
registers, where a seventh put the value on the stack). In lists mode
every read and every write by the only holder is the lists' code. A
write to a value whose written list (values for a put, live flags for a
remove) is shared copies on write as the lists do and marks the result;
the second such write converts the value to a log table (one pass over
its live entries into mlog_table: a flatten's cost, once) and writes
through it from then on. So a Map written while shared again and again
(a store under `Mutex.with`, a snapshot per request) gets the log's
O(1) copies, and code that copies a Map and writes the copy keeps the
lists' speed. A log value never goes back. The log paths, the
conversion, and the log lookup stay out of line (ssa.cold_fn); a
lookup tries the lists first (a log value's lists are empty, so they
miss at once) and tags a log position (MAPH_LOG) so the value read
needs no second test.

**Prototype measurements** (branch mhybrid, 2026-10-21):

- Collections, instructions retired (deterministic), lists -> hybrid:
  putget 1.23x, get 1.19x, update 1.19x, copymut 1.24x, branch 1.20x,
  iter 1.00x. Wall time (min of 9, interleaved, a loaded machine):
  putget about 1.5x, get about 1.5x, update 1.28x, copymut 1.47x,
  branch 1.14x.
- HTTP CRUD callgrind (arm64, plain, 3 runs, instructions a request),
  lists -> hybrid: 8 users 23,606 -> 23,405 (-0.9%); 10,000 users
  1,185,600 -> 534,805 (-54.9%), the log's win kept.
- Correctness: the Map goldens (map_log_model's random model,
  map_log_sets, maps, map_small, set_hofs...) in both modes, an aliasing
  check (kept copies, small maps, heap values, iteration while a copy is
  written, update) identical to the Go backend, no leaks.

**Concurrent use (2026-10-21, later).** Under concurrent HTTP load the
hybrid crashed: entries stored into chunks already marked shared were
never marked (gotchas/map-log-store-into-shared-chunk.md). With the fix
(mlog_share_into) and TestTSanSharedMaps, the CRUD server's mixed load at
1024 connections (Docker linux/arm64, servers on 6 CPUs, medians of 3
rounds of 10 s): lists 22,476 req/s, hybrid 75,412 (3.4x), Go 89,846
(Go's rounds 83k to 120k). With the lists, each write copies the users
Map under Mutex.with (about 40% of the server's CPU, all inside the
exclusive section: the server used about 1.2 of its 6 cores); the
hybrid's copy is O(1) and the server uses about 1.9.

**What remains is code generation**, not the Map: the never-taken call
to the out-of-line log lookup in a get loop makes arm64's register
allocator keep the loop's values in callee-saved registers and stop
hoisting its invariants (constants and base addresses rebuilt each
iteration; gotchas/arm64-cold-call-in-loop.md). The lists' loop has no
call. Allocating around cold calls would close most of the wall-time
gap; the owner decides the switch on these numbers.

## Alternatives

- **A HAMT** (hash array mapped trie, with an insertion-order structure
  beside it): O(log n) copies and writes, but every get becomes several
  dependent loads, insertion order needs a second persistent structure,
  and it would differ from the Go backend's proven design.
- **Drop the rollback snapshot in `Mutex.with`** (decision 0085's
  fault behavior; an owner choice the lead has taken to the owner as a
  possible interim): fixes this pattern only; every other shared-Map
  write stays O(n).
- **Keep the lists and document the cost**: contradicts the spec's
  persistent map and leaves a quadratic trap in the most common service
  shape (a store under a Mutex).

## The switch (2026-10-21, owner)

The owner's bar: put then get, get, and update within about 1.2x of the
lists in cycles, copy then mutate not worse, the HTTP gain kept. Two
changes met it after Xo lang 4's cold-call register allocation
(aa1fa095): put and remove test only whether the list they write is held
once and then run the lists' code (9753943a; the rest in the cold
xo.maph.putrest, remrest), and Map.get reads the lists' value on a hit
with no tag test (f3d13232: a tagged position cost two taken branches a
get, 1.50x the cycles at 1.05x the instructions in a get loop).

Collection workloads, hybrid against the lists (Mac, min over 11
interleaved rounds, both built by the same xo; the machine was loaded,
load average about 30):

| workload | cycles before | cycles after | instructions after |
|---|---|---|---|
| putget | 1.29x | 1.02x | 1.04x |
| get | 1.24x | 0.98x | 1.02x |
| update | 1.17x | 1.09x | 1.06x |
| copymut | 1.17x | 0.99x | 1.04x |
| branch | 1.18x | 1.17x | 1.20x |
| iter | 0.99x | 0.98x | 1.00x |

("before": the hybrid with the concurrency fix ae4306dc and the cold-call
allocation, ahead of the two changes.)

The CRUD server's mixed load (loadgen, 1024 connections, Docker
linux/arm64, servers on 6 CPUs, medians of 3 rounds of 10 s; Docker's
run to run spread is large, so compare within a run):

| run | lists | hybrid | Go |
|---|---|---|---|
| after the concurrency fix | 22,476 | 75,412 | 89,846 |
| after 9753943a | 22,170 | 69,715 | 74,478 |
| after f3d13232 | 17,982 | 83,000 | 107,522 |

req/s; the hybrid is 3.1x to 4.6x the lists.

The wasm backend keeps the lists for now (compiler/xo main.xo: map_log
is off for `--backend=wasm`, and the wasm tool and playground never set
it). Two wasm problems showed when the switch was tested there: the
count read of the hybrid's checks took the native Int word, where the
wasm runtime keeps an i32 count beside the list's i32 length (fixed:
maplog.xo obj_count), and an overwrite after a copy on write and an
iteration of the map was lost (repro: a literal map copied, a new key
put into the copy, the copy iterated, then an existing key overwritten;
native correct). The second is open; wasm switches when it is fixed.

Wasm switched the same day (lead, relaying the owner): both problems
were the hybrid's assumptions about the native layout, not wasm code
generation. The mark (copied on write once) was "the log field is not
the immortal empty list", but wasm's list.new allocates, so every new
map looked marked; and the lists' put fast path took "values held
once" for lists mode, but a log value's empty values list is held once
on wasm. The mark is now an empty list of capacity 5 (list_cap), and
the fast path also needs a non-empty list. Every backend now builds the
hybrid by default (`XO_MAP_LOG=0` the lists); TestWasmMapHybrid runs
the Map goldens on wasm in both forms and TestSelfhostWasm every golden
with the hybrid. HTTP CRUD instructions per
request (callgrind, arm64): -0.9% at 8 users, -54.9% at 10,000.

Addendum (2026-10-21, xo-fa; reviewed by Xo Lang 3 and Xo Lang 2): a
`for` loop over a hybrid Map or Set in log mode on a root table walks the
table in place (compiler/ssa map.xo for_mlog_root). `xo.mlog.eachpos[L]`
gives the own positions of the live keys' current entries in insertion
order (mlog_each's root walk and order, as positions: the view's len,
field 5; origin order; positions 0..n-1 only when the table has no
garbage), and the loop binds the key and value to fields of each entry,
borrowed. Before, the loop's lists came from xo.maph.logkvl, which copied
every entry (a dup of each key and value, dropped after the loop): a
store under a lock walked on every GET had its strings' counts fought
over by the cores. A forked view (parents) and a loop body with a closure
or a defer keep the lists' loop (the body is built once per loop:
check.twice_ok: at most 64 nodes).
- Soundness: mlog_put's unshared fast path (maplog.xo) replaces an entry
  in place when the directory and claim cell are sole; the loop holds its
  table's directory counted for the whole walk (insert_rc retains the
  leaf when the body's write consumes the map), so the directory is never
  sole during a walk and a put in the body appends. Entries are otherwise
  append only, a flatten builds a new table, and a reader reads positions
  below its view's len (find's argument).
- Measured: a single-thread walk loop over a log map, user 3.6 to 3.9 s
  to 1.21 s; Xo Lang 3's CRUD profile (Docker arm64, 1024 connections):
  drop Str, dup Str?, xo.mlog.each and logkvl gone from GET /users, CPU
  per request on 6 cores -9% and -44% in two rounds; throughput within the
  run-to-run noise (sockets and the scheduler lead).
- Cost: the body is built twice (the walk beside the lists' loop): xo's
  own __text grew 0.44% (51,250,808 to 51,474,280 bytes; nearly every
  loop body is under the 64 node limit). A single loop choosing the
  element read per entry was tried and rejected: insert_rc owns the
  joined bindings (a retain on each edge, a release after use), a dup
  and drop per entry in both modes; it needs a borrowed join first.
- The root test also requires the table's base to be 0 (field 3).
- Tests: golden map_log_iter (puts and removes in the body, a flattening
  body, a return and a break inside the walk, a body over 64 nodes on
  the lists' loop, a converted set; want.txt is the XO_MAP_LOG=0
  build's output, and XO_LEAKCHECK shows 0 live in both), the Map
  goldens on arm64, LLVM and wasm, TestTSanSharedMaps with a walk under
  with_read.

## Addendum (2026-10-22, Xo Publish): branching, behind XO_MAP_LOG=branch

Status: prototype, off by default; the owner decides the default on the
numbers below.

**The gap.** bench/perf/xo `branch` (2,000 rounds of `var c = m;
c.put(KEYS + i, i)` from one 50,000-entry Map) took 63 ms native against
0.18 ms on the Go backend. XO_PROF put every sample in memmove under
xo.maph.putrest: the hybrid marks a value only after that value copied
on write, and the mark is stored on the copy, never on the base. So
every branch from an unmarked base copied its keys, values, live flags
and slots (about 1.6 MB), then grew the copied keys (a second memmove,
60% of the samples).

**Design (maphyb.xo maph_fork_lists, maph_goback; maplog.xo lists
levels).**
- A lists value of MAPH_BRANCH_MIN (1024) or more live entries, written
  while its lists are shared, becomes a log fork whose one parent level
  is its own lists. There is no copy, so the write is O(1).
- A Level gains a lists part: keys, values, live flags, slots and live
  count, read in place.
  - xo.mlog.lprobe is the lists' find on it.
  - mlog_lvl_at makes an entry from it (no hash, no prev; its own origin
    when live, else -1).
  - findx, entry_at and each read lists levels through these.
- A lists level is always the oldest level. Its lists stay shared, so the
  base never writes them in place under a fork; the base's own write
  while forks live forks too.
- A fork that its only holder writes MAPH_GOBACK (64) times goes back to
  the lists. It replays its puts after the lists level through the
  lists' put alone (maph_listput; the first copies on write), then
  writes in place. A fork with a removal stays a fork.
- So a copy written all over (copymut) pays the lists' one copy as
  before, plus 64 log puts.
- Smaller Maps, and values already in log mode, keep the hybrid as it
  was.
- Types and code are unchanged without the flag. The flag is keyed as
  `mapbr=` in the front end, fn and part caches.

**Measured.** `xo bench --ab --rounds=10 --warmup=1`, A main's build, B
the flag on, the same xo; Mac, a gate slot.

| workload | instructions B/A | wall B/A | max RSS B/A |
|---|---|---|---|
| branch | 0.039 | 0.071 (65.6 to 4.7 ms) | 0.733 |
| copymut | 1.006 | 0.998 (noise) | 0.992 |
| putget | 1.000 | 1.035; 1.019 (0.97 to 1.032, noise) in a 15-round rerun | 1.002 |
| get | 1.000 | 1.000 | 0.995 |
| update | 1.000 | 1.004 | 0.994 |
| iter | 1.000 | 1.038 (noise) | 0.995 |

In-program timer: branch takes 223 us native against 179 us on the Go
backend.

HTTP CRUD callgrind (arm64, plain, N=100, 3 runs, instructions a
request; bench/perf/http/callgrind-docker.sh, PRELOAD and XO_MAP_LOG),
hybrid against branch: PRELOAD=8: 24,702 to 25,040 against 24,687 to
24,896; PRELOAD=10,000: 486,685 to 490,459 against 485,699 to 491,766. Unchanged: the store's first shared write now forks its lists instead of copying them, and it stays a log value as before. (Logs: the lead's notes, xo-publish/mapbr-crud.)

**Tests.**
- Golden map_branch: one-key branches, a branch written all over, removes,
  nested forks, the base written after branches, keys and values, Str
  keys, Sets. want.txt is the Go backend's output.
- TestMapBranch runs it and six Map goldens on arm64 with the flag.
- TestTSanSharedMaps adds a map_log=branch run: its 1,600-entry Map forks
  under snapshots, with no reports.
- XO_LEAKCHECK shows 0 live on the golden and on the benchmark.
- The LLVM backend gives the same output.
- With the flag off, the output is unchanged: scripts/identity against the parent compared 154 programs (77, darwin and linux arm64), 0 differing.

**Rejected on the way.** A first prototype cached a log table built from
the lists in a cell on the base and forked that table. Branch dropped
to 0.047x of the instructions, but building the 50,000-entry table cost
copymut +8.4% instructions and +48% RSS, and its lists-mode `get` +2.9%
(a stricter log test). A list copy that keeps the source's capacity
(rl_copy, so the copied keys need no second copy to grow) gave branch
0.82x of the instructions and +15% RSS. Branching makes that moot, so it
was dropped.
