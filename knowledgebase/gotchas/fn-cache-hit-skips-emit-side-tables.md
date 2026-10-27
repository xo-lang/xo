# A per-function cache hit skips what emitting the function records

Status: Observed (2026-10-26, xo-lang-4)

**Symptom.** With the per-function cache turned on for every amd64
target, a windows/amd64 build through a warm cache failed to link:
`prog-windows.0.obj: undefined symbol xo_w64_WaitOnAddress`
(TestBackendCacheEditIdentity's windows/amd64 row, the second build on).
Its cold build, and linux/amd64 and darwin/amd64 through the same cache,
were fine.

**Cause.** The Windows emitter calls a C function through a thunk: the
call names `xo_w64_SYM` and records SYM and its signature in `g.w64`
(compiler/arm64 x86win.xo w64_sym), and gen.xo's w64_thunks writes a
thunk for each recorded name after every function is emitted. A function
read from the cache (arm64.generate_cached) is never emitted, so it
records nothing, and its thunks are missing. The cache entry carries the
text, not the side tables the text depends on.

**Fix.** Until 2026-10-27 the per-function and part cache stayed off for
windows. Since then each function's entry (compiler/arm64 fcache.xo
FnRec) and each part's (partcache.xo) carry the g.w64 thunks and the
g.wsyms weak symbols (COFF `/alternatename`, calls.xo weak_ref) their
functions recorded, as an optional tail (put_w64) written only when one
is non-empty, so other targets' entries and keys are unchanged; a hit
adds them back (place_text, gen_part's merge). The other per-function
tables an emitter fills are the ref flags, wants, strings and positions,
which the entry already carried. The general rule: before serving a function's text from a cache, list every
table its emission writes outside its text.
