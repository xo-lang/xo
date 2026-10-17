# A polling watchdog in a scope costs its poll interval per run

Status: Observed (2026-10-15, `xo test --timeout`, decision 0235).

Symptom: after the native harness got a per-run time limit
(40c1d729), `xo test --no-gc cmd/xo/testdata/nogc/prop` went from
0.92 s to 20.6 s warm. Packages without property tests barely changed,
and the identical-output gates all passed: only the time moved.

Cause: each test process (and each shrinking replay of a serving test
binary) ran inside a `scope` with a reader task and a watchdog task. The
watchdog looped `clock.sleep(20ms)` and checked a done flag. A scope
waits for every task it spawned, so after the reader saw the end of the
output the scope still waited for the watchdog's sleep to end: up to
20 ms per run. nogc/prop makes about 900 replays.

Fix: the watchdog waits on events, not time slices: the reader sends
on a channel at each run's start and closes it at the end of the
output, and the watchdog is a `select` of a receive and `after limit`
(compiler/xo test.xo watchdog). A closed channel ends it at once.
TestNoGCShrinkOverhead fails when a warm run of the fixture takes more
than 5 s, so an overhead per run that grows tenfold is caught. In
general: a helper task that must outlive nothing should block on a
channel the main task closes, never poll with a sleep, and a timing
change belongs in a timing check because the output gates cannot see
it.
