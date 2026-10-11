# Self hosted arm64 emitter, 2026-10-11

Decision 0158. The arm64 emitter in Xo (`selfhost/arm64`, about 9,400
lines of Xo) against internal/arm64 (Go), compared through the new
hidden `xo asmdump` piece by piece: every function with its call site
table, every helper, every literal, and xo_entry, with the labels made
from program wide counters renamed (TestSelfhostArm64). The input of
both is the module after the SSA passes and reference counting:
internal/ssa's for the Go emitter, selfhost/ssa's for the Xo emitter,
on the runs both SSA builders build completely (decision 0150 stage 8:
identical after every pass on those runs).

Go mode (goasm.go, gosyntax.go: Go assembler for the Go backend's
hybrid builds) and the layout instrument (align.go, `XO_ARM64_LAYOUT`)
are not ported: the self hosted compiler emits native programs with the
default layout.

# Stage 1: the core, 2026-10-11

Everything but closures, boxes, aggregate equality, `share`, `reuse`
(stage 2), parameters by reference (stage 3), and inlining (stage 4):
value layout, instruction selection of arithmetic, compares,
conversions, aggregates, Option, lists, strings, string equality,
direct and runtime calls, inline counting and the counting helpers of
every type, cancelled retain and release pairs, the memos and value
ranges, strength reduction and rotation, every MIR pass, register
allocation with spills and far frames, frames, fault stubs, landing pads
and their call site tables, the text peepholes, and the literals. Both
emitters run with `--no-inline`.

TestSelfhostArm64, go build of the Xo emitter, every repository run but
the five heaviest (arm64Heavy: selfhost/check, parser, ssa, arm64, and
huge_frame):

| runs (642) | count |
|---|---|
| compared | 302 |
| rejected by both checkers | 278 |
| not built completely by both SSA builders | 62 |
| byte identical as a whole file | 76 |

| functions (of the Go emitter's 7,910 in the runs compared) | count |
|---|---|
| identical, with their call site tables | 5,237 |
| skipped by the Xo emitter (a later stage) | 2,673 |
| different | 0 |

All 2,902 helpers the Xo emitter emitted are identical to the Go
emitter's; every literal it emitted is one of the Go emitter's.
TestSelfhostArm64Mutations, 100 code mutations (seed 20261011): 77
compared, 23 rejected by both, 1,164 of 1,866 functions identical, 702
skipped, 0 differences.

A development comparison over every run with the heaviest included
(.tools/acmp.py, the prototype of the test): 10,974 functions, 6,139
identical, 4,835 skipped, 0 different. Why functions were skipped:

| reason | functions |
|---|---|
| stage 3, parameters by reference | 2,121 |
| stage 2, function values | 1,704 |
| stage 2, share and reuse | 514 |
| stage 2, function values and boxes counted inline | 216 |
| stage 2, equality helpers | 101 |
| stage 2, boxes | 95 |
| stage 3, calls by reference | 84 |

Speed, the emitter alone (`--bench=5`, ms for 5 generations, Apple M4
Pro, another agent's test suite running), go build of the Xo emitter;
the Xo emitter skips the functions of later stages, so lines emitted are
shown:

| input | Go emitter | Xo emitter, go backend |
|---|---|---|
| selfhost/lexer | 141 ms, 140,345 lines | 183 ms, 89,125 lines |
| selfhost/parser | 2,322 ms, 2,415,560 lines | 284 ms, 229,835 lines |
| bench/perf/xo | 39 ms, 39,245 lines | 47 ms, 23,935 lines |

Per line emitted, the Xo emitter takes about twice the Go emitter's time
on selfhost/lexer (2.04 against 1.00 microseconds per line).

Found in stage 1: a local `var` may shadow a parameter of the same name
without a diagnostic (gotchas/xo-local-shadows-parameter.md); the first
run faulted on it. No difference in the emitted code needed a second
attempt: the first full comparison found only differences of the
comparison itself (call site numbers, function names holding `: `,
the alignment line after a literal).

# Stage 2: closures, boxes, equality, sharing, 2026-10-11

Closures and their environments (with their drop and share functions),
captures, calls through function values, function addresses, rebinds,
boxes, function values and boxes retained and released inline, the
equality helpers of structs, tuples, enums, lists, and boxes, `share`,
and `reuse`. Both emitters still run with `--no-inline`.

Development comparison over every repository run (.tools/acmp.py; 306
runs compared, 278 rejected by both, 62 not built completely by both SSA
builders):

| functions (of the Go emitter's 10,974) | stage 1 | stage 2 |
|---|---|---|
| identical | 6,139 | 8,699 |
| skipped (stage 3: parameters by reference, calls by reference) | 4,835 | 2,275 (2,121 and 154) |
| different | 0 | 0 |
| runs byte identical as a whole file | 76 | 219 |
| helpers emitted by the Xo emitter, all identical | 3,305 | 9,950 |

TestSelfhostArm64 in the merge tier (every second run, the five heaviest
left out): 321 runs, 149 compared, 111 byte identical; 3,836 of 4,013
functions identical, 177 skipped, 0 differences; 4,340 helpers
identical. TestSelfhostArm64Mutations (30 mutations): 481 of 492
functions identical, 11 skipped, 0 differences.

Every stage 2 construct was identical on its first full comparison.
