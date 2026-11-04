# `xo asm` is not the built binary's code

Status: Observed (2026-11-04)

**Symptom.** Checking whether a compiler change removed a retain in
ByteBuffer.read_until, `xo asm --backend=arm64` of bench/perf/bytebuf
showed read_until inlined into `buf` and the same calls before and after;
the built binary had read_until as its own function and `buf` inlined into
`main`. `XO_DUMP_FN=buf` then said no such function reached the backend.

**Cause.** `xo asm` lays the program out on its own path (its inlining and
which functions stay separate differ from `xo build`'s module inliner), so
its listing answers "what code does this function get" only approximately.
A build that falls back from inlining (the module does not verify) differs
again.

**Fix.** Read what runs: `otool -tV BINARY` (darwin) or `objdump -d`
(linux) for the function's calls, and `XO_BACKEND_CACHE=off
XO_DUMP_FN=qualified.name XO_DUMP_DIR=DIR xo build` for its post-rc SSA
(a std function by its full name, e.g. `prelude.bytebuffer_read_until`;
the cache must be off or a cached function writes no dump). Build with
`XO_INLINE_STRICT=1` so a fallback fails instead of changing the layout.
