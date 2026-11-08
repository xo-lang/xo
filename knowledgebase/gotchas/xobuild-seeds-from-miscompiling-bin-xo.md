# xobuild seeds from the checkout's own bin/xo, so a codegen bug poisons the next build

Status: Observed (2026-10-16, an arm64 register allocator experiment).

Symptom: after a change to the arm64 emitter that miscompiles, `bin/xo`
of the worktree crashes on everything, even `xo version` (SIGSEGV, exit
139), and `go run ./scripts/xobuild` keeps reporting it up to date after
the bug is fixed.

Cause: xobuild builds bin/xo with an earlier xo, and the checkout's
earlier bin/xo comes before the seed in its order (toolchain.md). When
that bin/xo already carries the broken code generator, it miscompiles
the new compiler. The result is cached by source key in
`<user cache>/xo/checkout/<key>`, so the same sources fetch the broken
build again.

Fix: when an experiment touches code generation and bin/xo misbehaves,
build the compiler with a known good xo instead of xobuild:
`XO_STD=std /path/to/main/bin/xo build --backend=arm64 -o bin/xo
compiler/xo`, and point XO_CLI at it. The fixed point
(scripts/bootstrap-xo.sh) then checks the self rebuild. The cache entry
for that source key stays broken until the sources change or the entry
is removed (ask before deleting caches).

Observed again (2026-11-08, decision 0564 step 2, Xo Lang 1): a WIP commit
whose native code generation segfaulted four goldens was built by
xobuild and cached; the next commit's xobuild took that build as its
seed (the newest cached build of an ancestor) and failed. Setting
`XO_SEED` to a known good xo for the rebuild worked (XO_SEED comes first
in xobuild's order), after removing the bad entry by its literal path
under `<user cache>/xo/checkout/`. A WIP commit that changes code
generation is built with `XO_SEED` set from the start.
