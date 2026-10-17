# A broken xo becomes the seed of the next rebuild

Status: Observed (2026-10-14, B13 stage 2, branch b13-stage2).

## Symptom

After a runtime change in std/native/llrt/obj.xo, `go run
./scripts/xobuild` succeeded, but the xo it produced crashed at once on
every `xo build` (EXC_BAD_ACCESS in parser.Parser.parse_file: a list
pointer of 0). The next `go run ./scripts/xobuild` would have used that
binary to build the fix.

## Cause

xobuild builds compiler/xo with an earlier xo, in this order
(internal/xotool xotool.go seed candidates): `$XO_SEED`, the checkout's
`bin/xo`, the newest build in `<user cache>/xo/checkout/*/xo`, `xo` on
PATH, the prebuilt seed. A std change compiles into the new xo's own
code, so a runtime bug there breaks the new xo; that binary is then both
the checkout's bin/xo and the newest cached build, the first two
candidates of every later rebuild (in any worktree, for the cache).
The bug itself: obj.xo rl_grow_n fell through to growing the immortal
empty list when its fast allocation failed (elements too large for the
small classes); the benchmarks never took that path, the compiler did.

## Fix

- Move the broken `bin/xo` aside and rebuild with `XO_SEED` set to the
  last good build (`ls -lt ~/Library/Caches/xo/checkout/`; test it with
  a small `xo build` first).
- Delete the broken build's cache directory (only one your session
  made: the xobuild log names it, "into .../checkout/<key>/xo"), so no
  other worktree's xobuild takes it as "the newest earlier build".
- Before measuring a runtime change, build something real with the new
  xo (a self build, or `xo dev goldens`), not only the benchmarks: they
  cover the fast paths, not the fallbacks. The golden
  internal/llvm/testdata/golden/list_objects covers this one.
