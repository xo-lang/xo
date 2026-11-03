# The Go backend aliased a copy of an opaque std struct

Status: Observed (2026-11-03), fixed in compiler/gen/own.xo

**Symptom.** On the Go backend, `var b = a` of a `deque.Deque[Int]` shared the ring with `a`: after
`b.push_back(9)` and `a.push_back(7)`, `b.get(2)` was 7, and `a.pop_front()` emptied `b.front()`. The native
backends printed the right values. Found while porting std/heap, whose copy test failed only on go.

**Cause.** The Go backend marks the lists a value holds when the value is copied (rt.Own, `XoMark`), and decides
that from the declared fields (own.xo `holds_list`). A std struct whose declaration file is empty
(`struct Deque[T] {}`, "opaque") has no declared fields, so it was never marked, although its hand written Go type
(std/rt/deque.go) holds a List and has an `XoMark` method. The copy then wrote into the shared block. Deque's
corpus test passed only because its copy grew the ring (a fresh block).

**Fix.** `holds_list` is true for an opaque struct of a std module outside the prelude, and `mark_stmts` marks it
with `rt.MarkP(&x)` (a no-op for a type without `XoMark`); `go run ./scripts/stdgen` then adds `XoMark` to the
generated structs that contain one (Os, testing, multipart, template, x509). Regression test:
internal/gen/testdata/golden/std_opaque_copy.

**Avoid.** A new std type whose Go side holds a List needs an `XoMark` method; with the fix it is marked on copy.
