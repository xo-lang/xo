# 0636: The inliner runs in waves on the task pool

Status: Accepted (lead, 2026-11-14: the first of the re-profile's three time cuts; gate:
byte-identical output across worker counts and repeated runs, and the fixed point). Compiler
internal: no language or output change.

## Context

The 2026-11-14 re-profile (host 1, linux/arm64, 12 CPUs, main a0c61aef6, every cache cold;
~/xo-lead/notes/lang1/retime-rank-2026-11-14.md) put xo's test build of gcp's aiplatform/apiv1
at 23.4 s wall against 8.1 s for Go's go test of google.golang.org/api/aiplatform/v1 (Go's build
cache empty, its module cache warm), with 4.7 of 12 CPUs busy on average. inline.apply was 3.8 s
of it (7.6 s of the self build's 39.8 s), and the CPU profile put 99% of arm64.inline_cached on
the main fiber: the inliner walked the module's functions one at a time in function order.

That order is part of the result. inline_one(fi) copies the bodies of the callees it inlines as
they are at that moment: a callee before fi already inlined into, a callee after fi still as the
SSA builder left it, and fi itself an empty function while it is being worked on (a self call
sees no body, no borrowed parameters and no results). A plain parallel loop would change which
body each call site copies, and so the output.

## Decision

The walk runs in waves with the order's result.

1. What inline_one reads (audited 2026-11-14): every pass it runs (devirtualize, the find_*
   searches, guarded_callee, merge_straight, fold_consts, fold_branches, unroll_small_loops,
   sink_retains, cancel_rc_pairs, view_lens, stack_small, unstack_params, stack_lists,
   hoist_fields, fold_tuple_fields, cse, take_in_place, the stack checks) takes the module read
   only. Of other functions they read bodies of inlinable callees (the name closure that
   inline_keys already keys the cache entry on, name_closure) and callee facts (borrowed,
   var_params, catches, results, indirect), which no inline pass changes. inline_one is now
   inline_body(m, st, fi, f) with m read only, behind the old inline_one.
2. inline_waves gives each function a wave: when one of two functions names the other in its
   closure, the one with the lower index goes to an earlier wave (longest path over those
   edges, in index order). So a function reads the inlined body of every earlier function it can
   reach (an earlier wave), the body before inlining of every later one (a later wave), and
   nothing of the functions in its own wave.
3. Each wave's functions are taken out of the module (their entries the empty function, as
   inline_one's own entry was), inlined at once on up to jobs tasks (inline_wave, the largest
   first), and put back in index order before the next wave starts. A wave of one function runs
   on the caller. A cache hit (inline_cached) is put in place in its wave.
4. jobs is module_jobs (XO_EMIT_JOBS); jobs 1 walks in function order as before. Both the cached
   walk (inline_cached) and the cache off and --release walk (inline_apply_jobs,
   inline_module_jobs) use the waves.
5. XO_DEBUG_INLINE_WAVES=1 prints the waves (count, functions and size after inlining per wave,
   an estimated speedup at 12 workers); =order walks the waves one function at a time, which must
   build the same binary (a check of point 2 without tasks).

## Results (2026-11-14)

- Waves: aiplatform/apiv1 31,828 functions in 232 waves, the self build 19,532 in 185; with the
  size after inlining as cost, the estimated speedup at 12 workers is 8.2x and 7.2x.
- Wave order without tasks (XO_DEBUG_INLINE_WAVES=order, host 1): the self build's xo is
  byte-identical to function order's (sha256 b2617f1e...).
- Determinism (host 2, every cache cold, all runs byte-identical to XO_EMIT_JOBS=1's function
  order): the self build (c329a2be...) and aiplatform/apiv1's test binary (6b0a02a7...), 10 runs
  each: XO_EMIT_JOBS=1, five default runs (12 jobs), XO_EMIT_JOBS=2 and 7, XO_THREADS=3,
  XO_MUTEX_YIELD=1. The scheduler has no shuffle mode; the thread and yield settings perturb it.
- Host 1 (a237352ad, every cache cold, two default runs each; the same output as XO_EMIT_JOBS=1's,
  sha256 ecceaacb... and 31464d06...): inline.apply 2.0 and 2.2 s for aiplatform/apiv1 (main's
  re-profile: 3.8 and 4.7 s), 3.2 and 3.9 s for the self build (7.6 and 9.6 s). A CPU profile of
  the self build puts 0.1 s of arm64.inline_cached on the main fiber (6.4 s before).
- `--ab` (host 1, the self build, main d1c7fbf12 against 81551b0d7, 7 rounds,
  ~/xo-lead/notes/lang1/pinl-ab-self.txt): wall 0.902 (0.895 to 0.903, better), CPU 1.023
  (1.018 to 1.023, worse: the second closure computation and the wave barriers), max RSS within
  noise (0.989). Five rounds ran at load 6 or more; the same slot ratios agree (0.904, 0.900).

## Steps

1. (Done) inline_body, inline_waves, inline_wave, the waves in inline_cached and inline_apply_jobs,
   XO_DEBUG_INLINE_WAVES, the determinism runs above.
2. (Done) Phase times and `--ab` on host 1, then the gate (host 2): codegen patterns, the arm64
   suite, cross, xounit, fp, the self build, the seed build; the amd64 sample (53 of 53).
3. Share the name closures with inline_keys (the CPU cost in the `--ab`).

## Consequences

- inline.apply is no longer serial; its wall time is the sum of each wave's largest function
  plus the wave barriers.
- A new read of another function's body or a module write inside the inline passes must keep to
  point 1 (the name closure, read only); otherwise the waves no longer give the order's result.
  The determinism runs and XO_DEBUG_INLINE_WAVES=order are the check.
- inline_waves computes every function's name closure again (inline_keys builds the same
  closures for the keys); sharing them is a later saving.

## Alternatives

- Inline every function against the bodies before inlining (no order): one parallel loop, but
  every result changes (callees inlined one level less), and the code size and speed effects
  would need their own measurement. Rejected for this step: the gate is byte-identical output.
- A dataflow schedule (start a function when its earlier closure members finish, not at wave
  boundaries): less waiting at barriers. Kept for later if the wave barriers show in the profile.
