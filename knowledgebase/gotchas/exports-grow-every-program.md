# C ABI exports are built into every native program

Status: Observed (2026-10-13, decision 0207)

Symptom: after the first exports (fault texts, number text, UTF-8,
address policy) the SSA golden of fib grew from 9 functions to 111 and
`go test -short ./internal/ssa` from about 9 s to about 90 s (the
goldens of every program carry the exports' closure); a hello world on
arm64 grew by 39 KB before the TLS and zone exports moved to feature
modules, 20,720 bytes after (bench/results/raw-memory-2026-10-13.md).
LLVM builds are unchanged in size (LTO drops what the C runtime's
reachable code does not call).

Cause: the C runtime is one object linked whole, so every symbol it
names must be defined, whether or not the code that names it runs; the
exports' closure (Dragonbox Float text for the Int conversion fault,
Duration and Time text, Net.only's policy) is built in every program.

Fix: put an export only the C of one feature calls in a
`cabi_<feature>.xo` module imported by that feature's implementation
(rooted only when its code is built, gotchas/std-testing-loads-http-everywhere.md);
keep cabi.xo for what xo_rt.c and xo_std.c always compile. The next
reduction is moving the always compiled C callers themselves (the
Float to Int fault, Float32 text, the http log time) to the generated
code or Xo, so their exports follow use.
