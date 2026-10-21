# 0316: an input-keyed program cache (XO_PROG_CACHE)

Status: Superseded by 0236's run cache for `xo test` (2026-10-21, xo-fa);
not pursued for `xo build`.

The proposal: key a finished program on its inputs before the check, so
an unchanged warm build skips the check, SSA passes and link (the warm
floor of `xo test --std-module=M` was 0.55 to 0.83 s against Go's 0.07 to
0.4 s). Decision 0236's run cache already does this for `xo test`, with
a key and an input list that cover what this draft listed (the command
line, XO_ environment, toolchain, every std file and module directory
listing, xo.mod and xo.lock). Only `--std-module` runs had been excluded,
which the 0236 addendum of the same day lifts. `xo build` has no warm
case that measured as a problem, so no second cache is added.
