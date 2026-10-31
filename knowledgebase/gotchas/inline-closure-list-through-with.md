# A List of closures in a request field broke arm64's inlined std store

Status: Observed (2026-10-30, Xo lang 5)

## Symptom

The main check's internal/arm64 TestStdStoreBuilds failed with
`xo dev stdobj exit 1: xo: the inlined arm64 module does not verify (74
errors; the first: http.ReverseProxy.serve: b100: v392.0 is held 1 times
at ret (leak or imbalance))` (XO_INLINE_STRICT=1). Every golden passed:
whole program builds of the HTTP goldens inline less of std than the
std store does (`xo dev stdcache --conc=0`), so only the store build met
it.

## Cause

82a509844 added `BodyState.closers: List[fn() -> ()]` (inside the
BodyReader's Mutex) and `Request.src_close`, filled by
`with_body_reader` from `b.st.with_read(fn(s) { s.closers })`. Once that
list flowed into ReverseProxy.serve's inlined body, the inlined module
held one reference too many at return; the module without inlining
verified. Every shape that kept a list read out of the Mutex failed: the
`with` field, an assignment after it, a copied list, and capturing it in
the source closure (69 errors); leaving it empty passed. A small program
with the same shapes (a struct field, `with`, `with_read` of a closure
list, an inlined caller) verified, so the trigger needs more of std's
context; the std variant path below reproduces it.

## Fix

The closers moved out of the Mutex: `BodyReader.close`, set once by
`http.pipe()` (`conn_body(...) with {close: [...]}`) and read with no
lock; `with_body_reader` captures it in the source function, which
closes the reader when called with a negative max, and the client's
close_src calls each `req.src` with -1. No `Request.src_close` field.

A trap on the way: closing through `b.st.with_read` at close time
deadlocked (the arm64 and LLVM goldens timed out at 600 s):
`BodyReader.read` holds `st` for the whole read, and the sender task
waits inside it on the pipe. A closer must not take a lock that a
blocked read holds.

How to find it: bisect std, not xo. One xo and each revision's std
(`git archive REV std`, `XO_STD=DIR XO_INLINE_STRICT=1
XO_BACKEND_CACHE=on XO_BACKEND_CACHE_DIR=... xo dev stdcache --conc=0`)
takes seconds a revision; std variants patched by a script isolate the
construct. Check the exit status, not only the verifier message: a
variant that does not compile also lacks that message.
