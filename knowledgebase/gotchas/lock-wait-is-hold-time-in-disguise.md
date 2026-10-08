# Same lock design, far longer waits: the Xo critical sections were longer

Status: Observed (2026-10-08, bench/perf/http task 01, 32 workers)

Symptom: the Go and Xo servers of bench/perf/http use the same lock
design (an RWMutex, reads shared, the list sorted inside the read
section), yet the store lock was 61 percent of Xo's blocked time and
acquiring it waited 44 to 95 us on average against 5 to 20 us in Go.
The CPU profile put under 1 us of CPU per store operation, so the
critical sections looked short.

Cause: measured directly (`XO_LOCKSTATS`, `HOLDSTATS`, see
`infrastructure/toolchain.md`), Xo held the lock 8 to 30 times longer
than Go (create and update p50 5.6 us against 0.2 us, get 1.3 us against
0.17 us), and lock waits grow much faster than linearly with hold time
under 32 workers. The CPU profile understated the hold about fivefold;
only wall clock timestamps around the section show it. Three costs:

1. Runtime bookkeeping inside the hold: `Mutex.with` walked 14 stack
   frames (`runtime.Callers`) for the holding site, inserted into the
   task's held map, and took the lock's internal `l.mu` a second time to
   record the owner, after the permit was taken. `l.mu` is what every
   waiter and releaser hammers, so the holder queued for it inside its
   own critical section. About 1.2 us of every hold.
2. `st.users.values().any(...)` built a list of every user (6.5 KB copy)
   to scan it, and `Map.each` on a log with overwrites and tombstones did
   a hash lookup per entry.
3. A closure that takes `&v` for `MarkP` moves `v` to the heap on every
   call, even on the branch that does not take the address: the first
   in place map iterator allocated once per element (128 allocations per
   scan).

The Xo store's `email_taken` scan itself (Go keeps an email index) is a
difference in the benchmark's design, not in the language, and stays.

Fix: bookkeeping before the wait and owner recording in the same `l.mu`
section as the permit (`holdSite`, `acquireMode(..., site)`); loops over
`m.values()`/`m.keys()` iterate in place (`rt.Map_values_seq`); a
linear `Map.eachRoot`; `a.to_lower() == b.to_lower()` lowers to
`rt.Str_eq_lower`; iterators keep separate closures for types that need
marking. Hold p50 fell to 0.2 to 0.4 us for get and delete and about 2 us
for create and update; wait p50 fell from 16 to 51 us to 0.3 us
(`bench/results/perf-throughput-2026-10-08.md`, "Lock hold time"). To
check: build the server, run it with `XO_LOCKSTATS=<file>`, load it,
stop it with `kill <its PID>`.
