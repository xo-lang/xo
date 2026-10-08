# Notes: 10-log-pipeline (Xo 0.6)

## Design

- `Pipeline` holds handles only: `Chan[Str]` queue (capacity `queue_cap`,
  default 1000), `Mutex[Aggregates]`, a `Mutex[List[Str]]` spill slot, and
  `Atomic[Int]` counters. Copies captured by worker closures share it.
- `run` = `defer { finish }` + `scope` with N worker tasks, one background
  flusher, and the reader (`feed`) in the scope body. `feed` blocks in
  `queue.send` when the queue is full (backpressure, nothing dropped) and
  closes the queue in its `defer`.
- Shutdown: a signal cancels main's scope. Because cancellation is sticky
  (core 7.2), the drain work is done in shielded `defer` code:
  - the reader keeps the line it is sending in `in_hand`; if `send` unwinds,
    its `defer` moves that line to the spill list;
  - a worker processes each received line inside `defer { p.process(line) }`,
    so cancellation between `recv` and the aggregate update cannot lose it;
  - `finish` (run's defer, after the scope has waited for every task) processes
    the spill list and everything left in the queue with `try_recv`, writes the
    final flush, and prints the summary.
- Flush: snapshot under the mutex, render one JSON object per service per line,
  `create_temp` + `write` + `rename` in the output `Fs` sub capability; a
  `defer` removes the temp file if the rename never happened.
- Per service: count by level, min and max timestamp ("first and last"), total,
  and top 5 messages by count (ties by message text, via a `derive Ord` key).
  Flush files are cumulative snapshots named `flush-000001.json`, ...

## Spec gaps and guesses (all accepted by the toolchain)

- Draining on SIGINT/SIGTERM: the spec says the signal cancels main's root
  scope and cancellation is sticky, which seems to forbid "let workers finish
  what is queued". The only escape I found is shielded `defer` code. Using
  `defer { p.process(line) }` as a shield inside a loop is legal but reads
  oddly; a `shield { }` block or a "stop reading" signal distinct from
  cancellation (for example `os.signals()` as a channel) would be clearer.
- Is `Mutex.with` a cancellation point when uncontended? The spec says "while
  waiting"; I shielded the update regardless.
- `IoErr` is used unqualified in a non test file. The spec lists it under the
  Stdio capability but does not say it is predeclared. It resolved.
- `MemFs.fail_next` and `files()` paths relative to `os.fs` root while the code
  writes through `os.fs.sub("out")`: guessed the key is `out/flush-000002.json`.
  That was correct (the log shows the injected rename failure on flush 2).
- `create_temp(".", prefix)` in a sub capability: "." means the root (std 3.1).
- Whether `FakeStdio.read_line` returns `""` for a blank line in the middle of
  input: assumed yes and counted it as malformed; the test passed (9 read,
  6 malformed).
- `os.clock.sleep(1ms)` in a test as "wait until every other task is blocked":
  inferred from the auto advance rule (core 9.2); it worked for the
  backpressure test (queue length 3, reader count 4, then all 10 drained in
  order).
- `s.cancel()` in a test as the stand in for a signal on a pipeline fed from a
  channel. `FakeProc.signal_after` cannot interrupt a fake stdin that is fully
  available at once, so mid input shutdown is tested through `run` directly.
- Function types in parameter lists with `uses fx` followed by a comma: used a
  type alias in tests (`type ReadLine = fn() -> Str? ! IoErr`) to avoid the
  return position ambiguity described in core 15.

## Suspected compiler or runtime bugs

None observed. Check was clean and all 9 tests passed on the first run with
`--race`.

## Go or Rust habits caught while writing

- Wanted `ch.recv()` in a `for` loop with `ok` (Go); used `match` on `T?`.
- Wanted `atomic.AddInt64(...)` as a tail statement; in Xo the tail value of a
  `()` function matters, so I used `let _ = counter.add(1)` where it would be
  the last expression of a branch.
- Wanted to `return` the summary from `run`; on cancellation nothing after the
  blocking call runs, so the summary is printed in the deferred `finish`.
- Wanted `os.Rename` style atomic write helpers from the stdlib; wrote
  `write_atomic` with a deferred cleanup flag.
- Avoided tuple sort keys (Ord on tuples is unspecified); used a small
  `derive Ord` struct.

## Friction

- No flag parsing in std; wrote `parse_args` and `parse_duration` by hand.
- `std/log` field maps need `Map[Str, Debug]`; fine, but numbers and errors
  mixed in one map rely on the implicit interface conversion.
- No way to read the summary from `run` in the cancel path except through
  shared handles (`Pipeline.summary()`), which shaped the API.
