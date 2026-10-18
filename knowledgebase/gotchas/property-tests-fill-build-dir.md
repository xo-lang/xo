# Large property test runs filled the disk through XO_BUILD_DIR

Status: Observed and fixed (Xo Lang 2, 2026-10-18).

## Symptom

`XO_COMPILE_PROPS=3000 XO_COMPILE_PROP_LLVM=1 go test ./internal/gen -run
TestCompileProperty` wrote about 1 GB a minute into the package's
temporary directory (`$TMPDIR/xo-gen-test-*`): 18 GB in 17 minutes,
and free disk fell to 23 GB before it was stopped.

## Cause

internal/gen's TestMain sets XO_BUILD_DIR to `<tmp>/build` for every
build of the package. xo keeps a build directory per program there (the
generated Go tree, objects, the binary), and it never prunes a base the
user chose (compiler/xo cache.xo build_dir). Every generated program is a
new directory, so the base grows with the seed count until the package
exits. The property tests also fill the shared binary cache
(`<user cache>/xo/testbin`) and backend cache, which do prune, at 20 GB.

## Fix

- The property tests (compile, arith, defer fault) build each program
  with XO_BUILD_DIR under its own t.TempDir() (propEnv in
  internal/gen/compileprop_test.go), removed when the subtest ends. 30
  programs at -parallel 2 peaked at 87 MB.
- For long runs, also set `XO_BIN_CACHE=off XO_BACKEND_CACHE=off`, run in
  batches (XO_COMPILE_PROP_FIRST, XO_ARITH_PROP_FIRST), and check free disk
  between batches.
