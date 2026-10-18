# Test packages filled the disk through their XO_BUILD_DIR

Status: Observed and fixed (Xo Lang 2, 2026-10-18).

## Symptom

`XO_COMPILE_PROPS=3000 XO_COMPILE_PROP_LLVM=1 go test ./internal/gen -run
TestCompileProperty` wrote about 1 GB a minute into the package's
temporary directory (`$TMPDIR/xo-gen-test-*`): 18 GB in 17 minutes,
and free disk fell to 23 GB before it was stopped. The same day the
lead's full suite run of internal/llvm reached 11 GB
(`xo-llvm-test-*/build`: 769 program directories of about 15 MB, the
LLVM objects of each).

## Cause

The TestMains of internal/gen, internal/llvm, internal/arm64, cmd/xo
and internal/selfhost set XO_BUILD_DIR to `<tmp>/build` for every build
of the package. xo keeps a build directory per program there (the
generated Go tree, objects, the binary), and it never prunes a base the
user chose (compiler/xo cache.xo build_dir). Every generated program is a
new directory, so the base grows with the seed count until the package
exits. The property tests also fill the shared binary cache
(`<user cache>/xo/testbin`) and backend cache, which do prune, at 20 GB.

## Fix

- internal/xotool Build (buildScratch) gives each `xo build` of a package whose TestMain sets XO_TEST_BUILD_SCRATCH=1 (internal/gen, internal/llvm, internal/arm64; cmd/xo tests read their build directories) a fresh
  directory under the package's XO_BUILD_DIR and removes it after the
  build; the binary is at the build's `-o` path, outside it. Not when
  the caller sets XO_BUILD_DIR in Options.Env, for ModeTest, or with
  XO_TEST_KEEP_BUILD=1 (to look at a build's files). 30 compile and 8
  defer fault programs at -parallel 2 peaked at 43 MB.
- For long property runs, also set `XO_BIN_CACHE=off
  XO_BACKEND_CACHE=off` (those caches prune, but at 20 GB), run in
  batches (XO_COMPILE_PROP_FIRST, XO_ARITH_PROP_FIRST), and check free
  disk between batches.
