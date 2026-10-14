# 0227: Duration text accepted at arm64 1.112 with LLVM 0.925

Status: Accepted, implemented (owner, 2026-10-14)

## Context

Decision 0193 moved the runtime's number and time text to Xo
(std/native/llrt/text.xo, the default since 2026-10-12) and kept the C
path selectable (`XO_LLRT_C=text`, std/native/llrt/text_c.xo over 514
lines of std/llrt/xo_std.c under `-DXO_TEXT_C`) until bench/perf/text
met decision 0180's bar (within 5% of the C's instructions) on both
native backends. Every text workload met it except Duration text: 1.112
of the C's instructions on the arm64 backend, 0.925 on LLVM
(bench/results/arm64-codegen-2026-10-13.md, "Duration text"). The
module has one switch, so all 533 lines waited on that one row (work
item 9 of bench/results/zero-c-audit-refresh-2026-10-13.md). Nothing
ran the C path any more: text_table and every golden that prints a
Duration or Time run text.xo.

## Decision

The owner accepts Duration text at 1.112 of the C on arm64 together
with 0.925 on LLVM (2026-10-14). The C path is deleted: text_c.xo, the
`XO_TEXT_C` blocks of xo_std.c (514 lines: Int.parse, Duration text and
parse, Time text and parse, Debug quoting, the days_from_civil
forward), and the `-DXO_TEXT_C` selection in selfhost/xo link.xo and
internal/xotool cc.go. `XO_LLRT_C=text` now fails the build like the
other deleted names (decision 0198); net is the only C path left.

## Consequences

- std/llrt is 514 lines smaller; the (a) FALLBACK category of the audit
  holds only the sched path.
- The arm64 Duration text gap stays open as a codegen item (decision
  0194's ranked list on selfhost/arm64), not a gate: an improvement there
  needs no C comparison, since earlier results files keep the C numbers.
- rt.xo keeps the declarations `int_parse`, `duration_parse`,
  `rs_append_time`, `time_parse`, `rs_append_quote`: the wasm runtime
  (selfhost/wasm rt2.xo) implements them.

## Alternatives

- Work decision 0194's ranked list on selfhost/arm64 until Duration text
  is within 1.05: kept as optional codegen work, not a gate for the
  deletion.
- Keep the C path as a fallback indefinitely: it was untested by any run
  and blocked 514 lines of the zero-C audit.
