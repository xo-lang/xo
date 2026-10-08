# Planted safety bugs in Xo, measured (2026-10-07)

The 30 X class bugs from the Go planted benchmark (tasks 01 to 05) were
translated to Xo by one agent (Sonnet), choosing realism without running any
checks, into `bench/planted-xo/<task>/variants/`. Each variant ran through
`xo check`, then `xo test --race` (`bench/planted-xo/run_detection_xo.py`,
raw data in `bench/planted-xo/results.jsonl`). This replaces the estimate in
`planted-and-harder-2026-10-07.md` Part 2.

## Results

| Class | Inexpressible in Xo | Caught by `xo check` | Caught by `xo test` | Escaped | Go plain caught | Go strict caught |
|---|---|---|---|---|---|---|
| X1 ignored error | 0 | 5 (see note) | 0 | 0 | 1/5 | 1/5 |
| X2 nil | 5 | 0 | 0 | 0 | 4/5 | 4/5 |
| X3 missing case | 0 | 5 (see note) | 0 | 0 | 1/5 | 2/5 |
| X4 data race | 4 | 1 (XO0501) | 0 | 0 | 0/5 | 2/5 |
| X5 leak | 3 | 0 | 1 (hang) | 1 | 0/5 | 0/5 |
| X6 lost update | 0 | 3 (XO0520, XO0110, XO0502) | 2 | 0 | 3/5 | 4/5 |
| **Total** | **12** | **14** | **3** | **1** | **9/30** | **13/30** |

Raw: 29 of 30 prevented or caught in Xo, against 13 of 30 for strict Go.

## Honest adjustments

- **X1 faithfulness.** All 5 Go bugs were explicit `_` discards. Four Xo
  variants were written as implicit drops (a bare statement), which XO0201
  rejects but which are not the same mistake. The faithful Xo analog is an
  explicit discard (`.ok() ?? default` or `.ignore_err("...")`), which
  compiles. The one variant that tried the shortcut `r ?? []` on a `Result`
  was rejected (XO0406): Xo removes the silent shortcut but not a deliberate
  explicit discard. Counting X1 as 0/5 for faithful explicit discards:
  **25 of 30**.
- **X3 faithfulness.** Two Go X3 bugs were removed `select` cases, not
  switches; Xo's `select` has no exhaustiveness. The Xo variants became
  `match` arm removals. Counting those 2 as escaped: **23 of 30**.
- **X5.** Two variants are hangs rather than leaks (structured concurrency
  turns a leak into a scope that never exits). One hang failed a test; the
  other (`spawn` instead of `spawn_background` for the evictor, task 05)
  hangs only at shutdown, which no test exercises, so it escaped.
- **03 X6** is a loose analog (a clamp on an unused copy) and was caught by
  contract fuzzing.

**Conservative result: Xo prevents or catches 23 of 30 planted safety bugs,
against 13 for strict Go and 9 for plain Go.** The upper bound is 29.

## Also found

- `??` binds loosest, so `x ?? [].map(f)` applies `map` to `[]`. The planting
  agent fell into it. Worth an `xo vet` hint or a precedence note in core 15.
- Checker: the right side of a failed `??` is now inferred with a bad hint to
  avoid cascades (no behavior change otherwise).

## Update: shutdown helper

`os.proc.signal_after(d)` (std 8.4) now delivers SIGTERM to a program run by
`testing.run_main`. With it, the "escaped" variant 05 X5 (`spawn` instead of
`spawn_background` for the evictor) shuts down cleanly, exactly like the
original: SIGTERM cancels the whole root scope, including foreground tasks, so
the evictor stops. That variant is not a defect in Xo. Revised conservative
result: **24 of 30 prevented or caught, 0 escaped among faithful variants that
are real defects**; the 6 not counted are the X1 explicit discards and X3
`select` cases discussed above. Regression test:
`internal/gen/testdata/golden/test_signal_after`.

## Logic bug control (N1 to N4)

20 logic bugs translated to Xo by a separate agent; all type check, so only
tests can catch them (raw data: `bench/planted-xo/results-N.jsonl`).

| Class | Xo tests caught | Go tests caught |
|---|---|---|
| N1 off by one | 4/5 | 5/5 |
| N2 wrong status or kind | 3/5 | 5/5 |
| N3 wrong arithmetic | 4/5 | 3/5 |
| N4 wrong condition | 5/5 | 4/5 |
| **Total** | **16/20 (80%)** | **17/20 (85%)** |

As expected, Xo gives no advantage on logic bugs; its tests catch them at
about the same rate as Go's. The safety bug gap is therefore not explained by
Xo solutions simply having better tests. Three variants were loose analogues
(01 N3, 02 N1/N2, 04 N3; see the manifests).

## Independent replant (second author)

A second agent, without seeing the first author's variants, planted the same
30 classes into the same Xo solutions, told to prefer subtle, deliberate
looking forms (`bench/planted-xo2/`, raw data in `results.jsonl` there).

| | Author 1 | Author 2 |
|---|---|---|
| Inexpressible | 12 | 6 |
| Caught by `xo check` | 14 | 4 |
| Caught by `xo test` | 3 | 11 |
| Escaped | 1 (not a defect) | 9 (8 real defects; 05 X5 is the same non defect) |
| **Prevented or caught** | **24 to 29** | **21** |

Escapes from author 2, by shape:

| Shape | Escaped | Note |
|---|---|---|
| `m[k]` on a missing key faults | 01 X2, 02 X2 (2 more caught by tests) | Xo's form of a nil dereference: a crash, not a type error |
| Check then act across two `Mutex.with` sections | 01 X4, 05 X4 (02 X4 caught by a test) | Xo prevents data races, not atomicity bugs |
| `_` arm swallowing a case | 04 X3 (3 more caught by tests) | defeats exhaustiveness |
| `.ignore_err("...")` discard | 04 X1, 05 X1 (2 more caught by tests) | explicit by design |
| Channel never closed | 02 X5 | shutdown waits for the timeout; slow, not stuck |

**Combined conclusion:** across two independent authors, Xo prevents or
catches 21 to 24 of 30 planted safety bugs (author dependent), against 13 for
strict Go and 9 for plain Go. The second author's escapes point at the
remaining gaps.

## Design candidates from the replant (for spec 0.6)

1. Map read indexing `m[k]` is a fault on a missing key. Make it a compile
   error for `Map` (use `get` or a named `get_or_fault("reason")`), keeping
   `xs[i]` for lists.
2. Warn (`xo vet`, or a checker warning) on `_` arms in a `match` over an enum
   declared in the same module.
3. Atomicity lint: data read inside one `Mutex.with` and written in a later
   `with` on the same mutex in the same function (check then act).

## Update: spec 0.6 rules (2026-10-08)

The three design candidates were implemented (decision 0038) and both planted
sets rerun. Warnings already present in the unmodified solutions are
subtracted.

| Set | Before | After: caught | After: caught or warned | Remaining escapes |
|---|---|---|---|---|
| Author 1 (X) | 29/30 | 29/30 | 29/30 | 05 X5 (not a defect) |
| Author 2 (X) | 21/30 | 23/30 | 25/30 | 04 X1, 05 X1 (`ignore_err`, by design), 04 X3 (`_` on prelude `target.os`, by design), 02 X5 (slow shutdown), 05 X5 (not a defect) |
| Author 1 (N, control) | 16/20 | 16/20 | 16/20 | unchanged, no false detections |

All four `m[k]` crashes are now compile errors (XO0410). XO0804 flags all
three check then act bugs, including one that is a branch on a stale read.
XO0305 flags the two `_` arms on own enums.

## Next experiments

1. A shutdown test helper (`testing.run_main` with a delivered SIGTERM) so
   "hangs at shutdown" bugs like 05 X5 fail a test.
2. Plant the N class (logic) bugs in Xo too, to confirm Xo's tests catch them
   at a rate similar to Go's 85%.
3. Rerun the planting with a second, independent agent to measure how much
   variant authoring choices move the numbers.
