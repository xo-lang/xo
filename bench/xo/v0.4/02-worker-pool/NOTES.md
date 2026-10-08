# Notes: 02-worker-pool (Xo 0.4, with checker)

Files: `xo.mod`, `pool.xo` (the pool), `main.xo` (the URL fetcher), `pool_test.xo` (tests).
The checker reported 0 errors on the first run. A deliberate probe (run 3) confirmed that
test blocks in `_test.xo` files are checked.

## Design summary

- `run_pool[T, R, E, fx: Effects](clock, jobs: List[Job[T]], workers, timeout, f: fn(Job[T]) -> R ! E uses fx) -> Report[R, E] uses (clock, fx)`.
- Job indices are pushed into a buffered `Chan[Int]` that is then closed. `min(workers, jobs.len())`
  tasks are spawned in a `scope` that runs inside `task.with_timeout`. Each worker loops on
  `queue.recv()` until it gets `None`.
- Outcomes go into a `Mutex[Map[Int, Result[R, E]]]` keyed by job index. After the timeout scope
  ends, the report is built by walking `jobs` in order: `Ok` goes to `results`, `Err(e)` becomes
  `(id, Timed.Failed(e))`, and a missing entry becomes `(id, Timed.TimedOut)`. Order is preserved
  by construction, whatever order the jobs finish in.
- The failure error type is `task.Timed[E]`, the std enum, rather than a custom one.

## Spec gaps and guesses

1. **The task signature has no clock.** The task says `run_pool(jobs, workers, timeout, f)`, but a
   timeout and an elapsed time both need `clock`, and capabilities only come from parameters
   (core 6.3). I added `clock: Clock` as the first parameter. This is a forced change, not
   a choice.
2. **Cancelled jobs that return an error.** Core 7.2 says a cancelled blocking call that returns a
   `Result` reports `Canceled`, and `f` may map that to its own `E`. Without a guard, a job that
   was killed by the timeout could be stored as `Failed(SomeCanceledErr)` instead of `TimedOut`.
   I guarded the store with `if !task.is_canceled()`. The spec does not say whether an
   uncontended `Mutex.with` is a cancellation point (7.2 says "`Mutex.with` while waiting"), so
   the guard is also my way of avoiding that question.
3. **`task.with_timeout` throws away partial work.** It returns `T ! Timed[E]`, so on expiry
   nothing the closure computed comes back. `task.map_concurrent` has the same problem when it
   is wrapped in `with_timeout`. That is why the pool records results through a captured
   `Mutex` instead of using `map_concurrent` (see friction 1).
4. **Elapsed time under the fake clock.** std 8.2 says "Time does not move while cancelled tasks
   unwind", so I assert `report.elapsed == 5s` exactly in the timeout test. If unwinding
   advanced time, this would need `>=`.
5. **Exhaustiveness with `E = Never`.** The `with_timeout` closure cannot fail, so I matched only
   `Ok(())` and `Err(TimedOut)` and left out `Failed`, relying on core 2.3 and the note in
   std 7. The checker accepted it.
6. **Bare variant names from another module's enum.** Core 2.3 says enums from other modules are
   qualified (`task.Timed.TimedOut`), but also that the prefix may be omitted when the expected
   type is known. I wrote bare `TimedOut` and `Failed(inner)` in match arms and the qualified
   form in construction (`task.Timed.Failed(e)`). The checker accepted both. The two rules
   could be read as conflicting. My reading is that the omission rule wins in arms.
7. **`testing.run_main(main, os)`.** I assumed `main` can be passed as a function value whose
   inferred effects instantiate `fx`, and that `os.stdio.output().lines()` gives one element
   per printed line. I did not assume anything about a trailing empty element.
8. **Display text of `http.ClientErr`.** It is not specified (only that it implements
   `Display`), so the program test only checks the `"<url> error: "` prefix for the
   unreachable host.
9. **Reading the whole map out of a read only `with`.** `done.with(fn(m) { m })` returns the
   protected value itself. Core 5.4 allows a read only view. I assumed returning it is
   allowed because it is an immutable persistent map. The checker accepted it.

## Suspected checker bugs

None found. Every construct I was unsure about (items 5, 6, and 9 above) was accepted, and
in each case my reading is that the spec allows it. The checker does reject what it should:
the probe in run 3 produced the expected XO0110 with a machine applicable fix.

## Go or Rust habits caught while writing

- **Rust `while let Some(i) = queue.recv()`.** This is not in the grammar (only `if let`). I wrote
  `while { let idx = match queue.recv() { Some(n) => n  None => break } ... }`.
- **Go `for idx := range queue`.** The spec does not list `Chan` as something `for` can iterate,
  so I used the explicit `recv` loop.
- **Go `sync.WaitGroup` plus `context.WithTimeout`.** Replaced by `scope` (which waits for its
  tasks) inside `task.with_timeout`. There is no `ctx` to pass.
- **Treating logging as a global.** I first used `log.warn` without `use std/log`. Logging is
  ambient (no capability), but it is still a module that has to be imported. I caught this
  before the first run.
- **Shadowing in a match arm.** I almost wrote `let i = match queue.recv() { Some(i) => i ... }`.
  Core 3.1 allows it in a nested block, but I renamed the arm binding to `n` for clarity.
- **Decrementing an atomic.** There is no `sub` or `fetch_max`, so I used `active.add(-1)` and a
  `compare_and_swap` loop for the peak.

## Friction (rules that made the task harder)

1. **There is no "run with a deadline and keep what finished" primitive.** `with_timeout`
   returns only `Err(TimedOut)` on expiry, and `map_concurrent` has no deadline. The obvious
   one liner does not meet the requirement:
   ```
   // loses every completed result when the timeout fires
   let rs = task.with_timeout(clock, timeout, fn() { task.map_concurrent(jobs, workers, f) })
   ```
   So the pool hand rolls a channel, a scope, N workers, and a `Mutex[Map[Int, Result[R, E]]]`
   to collect partial results. This is about 25 lines for something std almost provides.
2. **Capabilities change the API shape.** The required signature cannot be implemented as
   written, because timeout and elapsed time need a `Clock` passed in:
   ```
   pub fn run_pool[T, R, E, fx: Effects](clock: Clock, jobs: List[Job[T]], workers: Int,
       timeout: Duration, f: fn(Job[T]) -> R ! E uses fx) -> Report[R, E] uses (clock, fx)
   ```
   This is good for testing (the fake clock makes the timeout test instant and exact), but
   every caller has to carry the clock.
3. **Telling "timed out" apart from "failed because it was cancelled".** Because a cancelled
   effect call can surface as an ordinary `Err` that `f` maps into its own `E`, the pool has to
   check `task.is_canceled()` before recording, or a timed out job is misreported as a normal
   failure:
   ```
   let outcome = f(jobs[idx])
   if !task.is_canceled() {
     done.with(fn(var m) { m.put(idx, outcome) })
   }
   ```
   Nothing in the type system points this out. I found it only by reading 7.2 closely.
4. **Worker loops are verbose without `while let` or channel iteration**, as shown in the
   habits section.
5. **Per item output needs ids carried through `R`.** `Report.results` is a plain `List[R]` in
   job order, with failed jobs removed, so `main` cannot zip it with the URLs. The program
   puts the id and URL into `Fetched` and rebuilds the lines through a `Map[Int, Str]`. This
   comes from the task's report shape rather than from Xo, but value semantics and the lack of
   index based failure info make it slightly more work.
