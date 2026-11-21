# TestRaceStacksArm64 wanted a symbol the inliner had removed

Status: Observed (2026-11-21, task #608, tier 922's darwin llvm step)

Symptom: `TestRaceStacksArm64` and `TestRaceDefaultNative` (darwin/arm64,
`xo build --race` with the native backend) failed with `report lacks
"xo_f_touch main.xo:"` and "no race reported", while the report was
there and complete: `#0 xo_f_internal_2ecrypto__op internal.xo:103`,
`#1 touch main.xo:17`, `#2 xo_f_main main.xo:36`.

Cause: the tests looked for the symbol of testdata/race_stacks' `touch`.
Since 4bf60e8eb (decision 0716, 2026-11-18, before v0.54.0) the inliner
takes a small function with landing pads at up to 8 sites, `touch`
among them, so it has no symbol and no entry hook of its own; the
report names it as an inline frame from the debug information, without
the `xo_f_` prefix. `scripts/bisect.sh` from v0.53.0 named the commit.
The step had no result in three tiers (memory stops), so the stale
expectation went unseen from 2026-11-18 to tier 922.

Fix: both tests look for `touch main.xo:`, which a frame of either kind
prints.

Avoid: a test of a report's stack names the source function and line,
not the linker symbol: whether a small function keeps a symbol is the
inliner's choice.
