# A std closure stored in a pure function field needs rt.launder

Status: Observed (2026-11-12, std/native/http.xo Client.with_jar, decision 0617 B)

Symptom: arm64 and LLVM built the program, but the wasm goldens failed
with `XO0701 the expected function type fn(Str) -> Str is pure, but
performs clock, CookieStore.fx` at the closure that with_jar stored in
the Client.

Cause: the native backends' std build did not report it, but the wasm
front end checks std's effects in full. A closure that calls effectful
code (a Clock, an interface with an effect parameter) has that effect,
and a struct field of a pure function type (`List[fn(Str) -> Str]`, how
http1.client_do takes its hooks) cannot hold it.

Fix: when the installer pays (spec/core.md 6.3: the installing call
performs the installed code's effects), wrap the closure with
`rt.launder1` / `rt.launder2` (std/native/rt.xo), which perform the
closure's effects at the call that wraps it and return a pure function
type. Run the wasm goldens for any std change that stores a closure.
