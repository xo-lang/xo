# Go call site forms (decision 0441): cost on the zap and chi workflows

Measured 2026-10-30 on the linux/arm64 offload host (12 CPUs, the host
exclusive), the Go backend, base against the 0441 branch: the generated
Go lines (the program's own packages, and the whole tree the backend
hands `go build`), the edit loop's rebuild wall (the bin and backend
caches off, GOCACHE warm, 10 alternating rounds after 2 warmups a side,
the median), the binary size, and the zap workflow's recorded caller.
Script: measure102host2.sh (the lead's notes, xo-lang-6).

## Result (multi-line site literals, 24e6e944d against 6f25ffe83)

| workflow | side | Go lines (program) | Go lines (tree) | site literals | build wall median | b/a | binary | zap caller |
|---|---|---|---|---|---|---|---|---|
| zap | base | 200 | 128419 | 0 | 232 ms | | 6095010 | main.xo:21 (with the skip) |
| zap | 0441 | 389 | 130822 | 12 | 230 ms | 0.991 | 6095010 | main.xo:19 (the Xo call) |
| chi | base | 264 | 126336 | 0 | 238 ms | | 7930018 | |
| chi | 0441 | 538 | 128623 | 14 | 238 ms | 1.004 | 7930018 | |

Rounds (ms): zap base 233 232 228 229 234 227 234 227 231 232, 0441 226
230 230 229 227 229 229 231 232 237; chi base 241 239 234 239 238 237 237
233 238 237, 0441 238 237 238 239 236 241 242 239 240 236. The build
wall is within noise both ways; the program's generated Go grows by one
literal body per bound call (189 lines for zap's 12 sites, 274 for chi's
14, about 2% of the tree `go build` compiles); the binaries are the same
size. Both workflows pass on both sides; zap records the Xo line with no
caller skip.

## First run (one-line literals, 47409c140 against 1e4f0f213; superseded)

gofmt reflowed the one-line literal, so only its first line was under
the Xo call's //line directive and zap recorded main.xo:25 (the
statement's line in the formatted Go), not 19; the second run has the
escaped multi-line literals. The cost columns stand.

| workflow | side | Go lines (program) | Go lines (tree) | site literals | build wall median | b/a | binary |
|---|---|---|---|---|---|---|---|
| zap | base | 200 | 128402 | 0 | 230 ms | | 6095010 |
| zap | 0441 | 296 | 130712 | 12 | 232 ms | 1.007 | 6095010 |
| chi | base | 264 | 126319 | 0 | 238 ms | | 7930018 |
| chi | 0441 | 403 | 128471 | 14 | 239 ms | 1.004 | 7930018 |

The binary is byte for byte the same size: the literals inline what the
adapters held, and the adapters stay in rt for interface calls and
generics (the linker's dead code elimination drops the unused ones
either way).
