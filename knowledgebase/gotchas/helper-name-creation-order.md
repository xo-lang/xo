# A helper name that depends on creation order poisons the function caches

Status: Observed (2026-10-22)

Symptom: cmd/xo TestNoGCTests failed 3 of 3 on main at test_fakes:
on LLVM, clang rejected the IR (`instruction forward referenced with
type %"e.Str?"`: `invoke %"e.Bytes?" @"x.xo.maph.logget[Map[Str, Str]]"`
in http.fields_in), and the Go backend's run warned that the inlined
arm64 module does not verify (`v117: Str? = call
xo.maph.logget[Map[Str, Str]] ... result type does not match the
callee`). The same program built by hand from the repository root
passed on every backend.

Cause: Map[K, Bytes] lowers to Map[K, Str], so a get or remove helper of
both map types is named after `Map[K, Str]`, but returns `Str?` for one
and `Bytes?` for the other. compiler/ssa named the helper after the map
type and added `[Bytes?]` only when a helper of that name with another
result type already existed in the build: the second instance created
got the suffix. Which is second depends on the order a build reaches the
functions (the directory it runs from, what the caches skip), and a
function body cached under one order calls the plain name for a result
type that another build gave the other instance. maph.logget,
maph.slowrem, maph.remrest, map.remove and mlog.remove all did this
(decision 0291, f3d132327; on by default since d9c5bccae).

Fix: the name says which result type the helper returns from the types
alone (ssa map.xo opt_suffix: "" for the map value's own option, else
`[Bytes?]`), never from what the build already made. Any helper name
derived from a type that lowers to a shared one must carry its own
distinguishing types; a name chosen by checking `b.fidx(name)` first
reads as an order bug. The build-time layout check (owner-approved
2026-10-22) is meant to fail such a mismatch at link time.
