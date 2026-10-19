# A build that is not a release build already has debug information

Status: Observed (2026-10-19, Xo lang 4)

## Symptom

TestDebugInfoSameOutputLLVM, moved from release builds to -O1 builds to
save time, built each golden program without flags and with
`--debug-info` and compared the outputs. It passed, but the clang
cache showed only 24 new entries for about 390 builds: the two builds
of each -O1 program were the same build.

## Cause

compiler/xo main.xo: `debug_info = !strip && (!release || o.debug_info)`.
Only a release build leaves out debug information by default, so
`--debug-info` changes nothing for any other build; `-s` is what removes
it.

## Fix

Compare `-s` against a plain build for builds that are not release
builds, and `--release` against `--release --debug-info` for release
builds (internal/llvm debugvars_test.go). To check that a test's two
builds differ, count what a content keyed cache stores for them (the
clang cache's `clang-<key>` entries in XO_CC_CACHE).
