# Editor and edit loop, before and after (2026-11-05)

Item B of the lead's queue (owner via the lead): the numbers behind the
day's editor and edit-loop work, each with its machine, commit and
method, for the blog. Earlier context: [edit-loop-2026-11-05.md](edit-loop-2026-11-05.md).

## Method

- LSP: a scripted client (Go, ~/xo-lead/notes/xo-lang-4/lspbench, not in
  the repository) starts `xo lsp` in the checkout, opens
  compiler/ssa/build.xo, waits for its first diagnostics, then asks
  hover, definition and references at `module_scans(c: check` (line
  2600; medians of 5), makes a one-line edit and waits for diagnostics
  (median of 3), and reads the server's RSS. Each round is a new server.
  Mac (darwin/arm64, 24 GB), under heavy.sh's gate class.
- CLI lookups: one process per command, `XO_NO_SERVE=1` for cold, through
  a running `xo serve` for served (first a miss, then hits).
- Edit rebuild: `go run ./scripts/xobuild` after a one-line comment edit
  of compiler/ssa/build.xo, the builder being the revision's own clean
  build, on the linux/arm64 offload host ("host 1"), alone on it.

## LSP (Mac)

| | first diagnostics | edit to diagnostics | hover, definition | references | RSS |
|---|---:|---:|---:|---:|---:|
| main 9b1973a4a (before) | 468 to 544 ms | 708 to 712 ms | 1 ms | 622 to 733 ms | 1.64 GB |
| + references in place (b7edfe4cf) | 470 to 476 ms | 712 to 726 ms | 1 ms | 142 to 148 ms | 1.64 GB |
| + module check cache (decision 0526, f38f9ae53), A/B: cache off | 537 to 758 ms | 770 to 809 ms | 1 ms | 160 to 166 ms | 1.64 GB |
| + module check cache (decision 0526, f38f9ae53), A/B: cache on | 355 to 399 ms | 583 to 613 ms | 1 to 2 ms | 159 to 206 ms | 1.51 GB |

The first two rows ran on a quiet Mac; the A/B pair ran with a load of 11
to 55 from other jobs, one binary with `XO_CHK_CACHE=off` against the
default, interleaved off, on, off, on with 3 rounds each, so compare the
pair with each other, not with the rows above. References: the session
resolved every identifier on a copy of its tables (the whole checker)
and wrote it back (gotchas/element-copy-store-back-quadratic.md); inc and
drop were 41% of the CPU of a references-heavy profile.

## CLI lookups (Mac, main 9b1973a4a; process start and socket included)

| command | cold, 3 runs | served: miss, hit, hit |
|---|---|---|
| def Checker.unparse_bodies --dir=compiler | 0.56, 0.47, 0.38 s | 0.44, 0.03, 0.04 s |
| refs unparse_bodies --dir=compiler | 0.91, 0.90, 0.94 s | 0.99, 0.03, 0.03 s |
| type compiler/ssa/build.xo:2600:8 | 0.68, 0.68, 0.69 s | 0.93, 0.03, 0.03 s |
| doc Checker --dir=compiler | 0.60, 0.72, 0.65 s | 0.70, 0.04, 0.04 s |
| symbols --dir=compiler/ssa | 0.27, 0.28, 0.25 s | 0.29, 0.03, 0.04 s |

`xo serve` RSS 8 MB (decision 0283 stage 1, e849730f5). Host 1: 0.20 to
0.74 s cold, 0.00 s served, server 16 MB
([edit-loop-2026-11-05.md](edit-loop-2026-11-05.md)).

## Warm compiler edit rebuild (host 1, xobuild)

| | build | wall | note |
|---|---:|---:|---|
| before (main of 2026-11-05 morning) | 15.7 to 15.9 s | 17.5 to 17.6 s | edit-loop-2026-11-05.md w2, w3 |
| + cc object keys (fix 1, bccbae048, reverted in a7217f35d) | 9.4 to 10.3 s | about 11 s | 121 to 123 of 126 objects reused; identity: warm == cold |
| + one check pass (fix 2, feaeba094) | 6.96 to 7.39 s | 8.4 to 8.8 s | no module check cache fallback; ran with fix 1 of bccbae048 |

### Relanded (ac2664c08, both fixes on main)

Fix 1 relanded as ac2664c08 (8184fcb08 before pushq's rebase), with
the weak references of a reused object read from the object itself.
Measured at ac2664c08 on host 1, 2026-11-06, the same edit loop
(objkey3-remote.sh: a one-line comment edit in compiler/ssa/build.xo,
xobuild with XO_XOBUILD_TIMES=1, w2 and w3 after e0 and w1):

| | build | wall | note |
|---|---:|---:|---|
| main at ac2664c08 (fixes 1 and 2) | 6.81 to 6.90 s | 8.23 to 8.34 s | 121 of 127 objects and 120 of 126 parts reused; identity: warm == cold (w5) |

Log: ~/xo-lead/notes/xo-lang-4/editloop/objkey3-ac2664c08.log. The
program has 127 objects now (126 parts and one more object since the
rows above). The six objects rebuilt are the edited part's and those
whose part changed with it; assembling them is 2.3 s of the build.

Fix 1 was reverted (a7217f35d: a reused object's stub .s hid its weak
references from a darwin cc link) and relanded with that fixed; the
rows were measured before the revert. The fix 1 row ran the branch at 9d1f8dd23 (with diagnostic counters, since
dropped; the landed code is bccbae048); the fix 2 row ran a9f0f726e, fixes
1 and 2 with the same counters (landed as bccbae048 and feaeba094). Logs:
~/xo-lead/notes/xo-lang-4/editloop/objkey2-9d1f8dd23.log and
festats-a9f0f726e.log (w1 to w3).

Fix 1: the cc assemble path keyed objects on the build's random work
directory, so no object was ever reused on Linux
(gotchas/cc-object-key-random-dir.md). Fix 2: since decision 0517 every
warm build of a program with dependency modules checked the program
twice (gotchas/module-check-cache-skips-whole-program-passes.md).

Decision 0526 (accepted by the owner, 2026-11-05; f38f9ae53) puts the
module check cache under `xo lsp`; the A/B rows above ran on its binary
before the gate (the code that landed).

## Summary for the blog

- References in the editor: 622 to 733 ms, now 142 to 148 ms (4.5x).
- First diagnostics and diagnostics after an edit: about a third and a
  quarter less with the module check cache (A/B above), RSS 8% lower.
- CLI lookups through `xo serve`: 0.25 to 0.94 s cold, 0.03 s served.
- A warm one-line compiler edit through xobuild on linux/arm64: 17.5 s,
  now 8.2 to 8.3 s wall (build 15.7 to 15.9 s, now 6.8 to 6.9 s), on main
  at ac2664c08 (the "Relanded" table).
