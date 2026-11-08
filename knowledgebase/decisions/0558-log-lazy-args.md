# 0558: Log calls below the level do not evaluate their arguments

Status: Accepted as opt-in (owner, 2026-11-07; built 2026-11-07 as 87542c788: the guard is keyed on the std/log callees, since Xo has no attributes, and the warning is XO0438; Xo Lang 3, 2026-11-07; raised by Xo Lang 1's xo-vs-go survey, row 7, log/filtered 1.79x Go)

## Context

`log.debug("cache miss", {"key": "user:${i}", "n": i})` below the configured level writes nothing, but the caller builds
the fields `Map`, interpolates `"user:${i}"` and boxes every value as a `Debug` before `log.debug` is entered and its first
instruction (the level check) throws them away. bench/perf/log `filtered` (100,000 calls with two fields): Xo 13.4 ms against Go's
log/slog 7.5 ms (1.79x, survey 2026-11-07 row 7; profile: `maph.putbody[Map[Str, xo.dyn]]`, the interpolation and the `xo.dyn`
boxes). Go builds its `[]any` of arguments before slog's level check too, but a slice of boxed ints is cheaper than a
`Map` with hashing; Go's own advice for a hot debug call is `if logger.Enabled(ctx, slog.LevelDebug) { ... }`, which Xo
has no equivalent of: the level lives in the `log` effect's state, not in a value the caller can ask.

Production services log at Info and leave Debug calls in their request paths, so this cost is paid on every request.

## Decision (proposed)

1. **Call sites guard themselves.** A call to `log.debug`, `log.info`, `log.warn`, `log.error` (and the same methods of a
   `Logger`) is compiled as `if log.enabled(Level.X) { call }`: when the level is below the configured one the arguments
   (the message, the fields map and everything inside them) are not evaluated. `log.enabled(level) -> Bool` is a new `pub fn` of
   std/log (it performs `log`), also usable by hand for work that is not an argument (a costly dump).
2. **What changes for a program.** An argument with a side effect or an effect of its own (`log.debug("x", {"n": next_id()})`,
   a call that `uses` another capability) is not run when the call is filtered out. Go evaluates arguments always, so this is a
   deliberate difference; it is the same rule as `assert` messages and `debug_assert`. The checker warns (new code, warning
   only) when a log argument contains a call to a function that performs an effect other than `log` or that is not `pure`,
   since that is the case where the skipped evaluation could surprise: the message names the call and says it only runs when the
   record is written.
3. **Where it lives.** The lowering is in the SSA builder where a call to a function marked `@lazy_args` is built (the
   attribute applied to the four functions and the four methods in std/log's declaration file); no other function can take it
   in this decision, so no user visible `lazy` keyword is added.
4. **Not covered.** `Logger.with(fields)` still builds its map (once, not per record); a record the handler writes is
   unchanged, byte for byte, and `log.capture` in tests sees exactly the records that were enabled.

## Consequences

- `log/filtered` goes from a `Map` and boxes per call to one load and compare; expected below Go's row (Go still boxes its
  arguments). To be measured with `--ab` on the first linux/arm64 host before the decision is built, as a prototype on a
  branch.
- Enabled calls pay one extra compare against the level state, which the callee already did; the callee's own check stays
  for calls through a function value.
- A program that relied on an argument's side effect running with the log filtered out changes behavior. The warning of 2
  names these calls.
- Cost: the attribute and its lowering (a few dozen lines in compiler/ssa), `log.enabled` and the warning in std and the
  checker, spec/core.md 6.1 and std.md 6 text, tests for the skipped evaluation.

## Alternatives

- **Do nothing** (accept 1.79x): the cost is bounded (about 70 ns a filtered call with two fields) but it is on every
  request path of a service that logs at Info.
- **A closure form** (`log.debug_with("x", fn() -> Map[Str, Debug] { ... })`): rejected as the only API, it makes every call
  site noisier; it stays available to a program as `if log.enabled(Level.Debug) { log.debug(...) }`.
- **A lazy value type** (`Lazy[Map[Str, Debug]]`): rejected, a new type for one use, and the evaluation order would not be
  visible at the call.
- **Make the Map cheaper** (a small inline Map for up to 4 string keys): helps every Map use, not only logs, but does not
  remove the interpolation and the boxes; worth doing separately and compatible with this decision.
