# A prebuilt xo binary keeps the std/native it was built with

Status: Observed (2026-10-13, decision 0195, bench/perf/glob)

Symptom: after editing std/native (a new runtime module, a changed
import in testing.xo), measurements and differential runs through a
scratch `xo` binary show no change: the "Xo" and the "C" builds of
bench/perf/glob retired the same instructions, and both binaries
contained `xo_test_glob`.

Cause: std/native and std/llrt are embedded in the xo binary
(`std.NativeFiles`, decision 0157's packs); a binary built before the
edit compiles the old runtime. In-process tests (internal/llvm
`build.Build`) read the tree as it is, so they do not show it.

Fix: rebuild the binary (`go build -o <scratch>/xo ./cmd/xo`) after
every edit under std/ before a measurement or a run through it, and
check the binary for the symbol the change removes (`nm <bin> | grep
xo_test_glob`) when a C path and an Xo path measure the same.
