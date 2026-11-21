# A cached --race build on macOS left no ThreadSanitizer runtime beside the binary

Status: Observed (2026-11-21, task #608, tier 922's darwin llvm step)

Symptom: `TestRaceBinaryOutlivesToolchain/arm64` failed with "a cached
link left no copy of the runtime beside the binary". By hand: `xo build
--race -o out/prog DIR` then `xo build --race -o out2/prog DIR` of the
same program left `out2` with `prog` alone. That binary loads
`libclang_rt.tsan_osx_dynamic.dylib` by @rpath, beside itself first and
then from the toolchain's directory, so it ran only while the toolchain
stayed in place.

Cause: the link copies the runtime beside its output (compiler/xo ld.xo
race_rt_beside, and again after a binary cache hit in main.xo). The run
cache (decision 0360) answers before any of that, and since 530cddfba
(2026-11-15, before v0.54.0) its entry no longer depends on the -o
directory, so the second build was a run cache hit that copied the
binary and nothing else. A private, cold cache hides it when the run
cache is off or misses: `scripts/bisect.sh` passed the test at v0.54.0
and at main, while the test binary's shared cache failed it.

Fix: a run cache hit of a `--race` build copies the runtime beside each
darwin output (main.xo, after whole_build_hit).

Avoid: a file a build writes beside its output is part of the output:
every path that delivers a cached binary (the binary cache, the run
cache) has to write it too. Check a new side file with a second build
into another directory, caches warm.
