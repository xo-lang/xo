# Build paths in shipped runtimes

Status: Observed (2026-10-27)

## Symptom

The seeds' ThreadSanitizer runtime (xo/lib/xo/race/darwin-arm64/
libclang_rt.tsan_osx_dynamic.dylib, since at least 0.29) held 88 absolute
paths under the seed cutter's home directory, such as
`/Users/<name>/.../seed-0.32.0-work/race/src/llvm-project-23.1.3.src/
compiler-rt/lib/sanitizer_common/...`. `strings -a` shows them. The xo
binary and std held none.

## Cause

compiler-rt's sources assert and check with `__FILE__`, and its debug
info records the build directory. scripts/race-runtime.sh compiled them
under WORKDIR with no prefix map, so every such string named WORKDIR.

## Fix

race-runtime.sh passes `-ffile-prefix-map=$work=. -fdebug-prefix-map=$work=.`
in the C and C++ flags of every platform (and the Windows build's
EXTRA_CFLAGS). The rebuilt darwin-arm64 dylib has no `/Users/` string.
seed-pack.sh refuses a pack when a packed file contains a path
`XO_SEED_FORBID` names, and seed-cut.sh sets it to its WORKDIR, so a new
build path in any packed file stops the cut instead of shipping.

Do not test the check with a made-up path such as `/nonexistent`: the xo
binary contains that string, so the refusal proves nothing about the
path you meant.
