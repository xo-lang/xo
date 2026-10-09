# Native Mutex.with copied the whole protected value on every write

Status: Observed (2026-10-09, bench/perf/http task 01 on `--backend=arm64`
and `--backend=llvm`)

Symptom: the native servers ran at half the Go backend's throughput
(64k against 134k req/s) with 65 us of CPU per request. Profiles
(`sample <pid>`) showed `xo_rl_copy`, `xo.dupp.User`, `xo.drop.User`,
`xo.drop.State`, and `map.values` under the store's `Mutex.with`
closures, and `XO_RTSTATS=1` counted 12 fiber parks and 4 contended
runtime locks per request.

Causes, each worth a commit (bench/results/perf-throughput-2026-10-08.md,
"Native backends"):

1. Every release woke every waiter (`wl_wake`); 32 request tasks parked
   and woke about 12 times per request and convoyed on the mutex's own
   lock, putting worker threads to sleep in the kernel. Fix: wake only
   the oldest writer, or all readers when no writer waits (decision 0082).
2. `with(fn(var st) ...)` passed the value out of the mutex's list while
   the list kept it, so the closure's first change found a count of 2 and
   copied every list of the map, with an atomic count update per string
   of every user (values shared between tasks have atomic counts), and
   the old copy was dropped at unlock. Fix: move the value into the
   closure; the closure hands back the old value when it fails or unwinds
   (decision 0082), so Perceus keeps a second reference only when a
   failure can follow a change.
3. `Map.remove` wrote zero values into the key and value lists, so a
   remove that may be followed by `return Err` (the store's delete) still
   copied the values. Fix: clear only the live flag (decision 0082).
4. A woken waiter went to the waker's `next` slot, which thieves leave
   alone for 50 us, while the waker finished its request: the lock sat
   free. Fix: mutex wake ups go to the ring.

How to spot it again: `XO_RTSTATS=1` (fiber_park, lock_slow per request)
and `xo_rl_copy` under a `with` closure in `sample <pid> 5`.
