# A std module named like an llrt module calls the wrong function

Status: Observed (2026-10-29)

## Symptom

Adding std/x509 (std/x509.xo and std/native/x509.xo) while
std/native/llrt/x509.xo existed failed every native build that used it:

```
error XO1510: a function disagrees with this build's definitions (a stale
cached piece; decision 0342): x509.parse: b0: v1: native.x509.Parsed = call
x509.parse, v0: result type does not match the callee
```

and with the backend cache off, `XO_INLINE_STRICT=1`'s "the inlined arm64
module does not verify". Nothing was stale: the message's cache advice
misleads.

## Cause

std/native/x509.xo imported the llrt module under an alias
(`use native/llrt/x509 as lx`) and called `lx.parse(der)`, while std/x509
declares its own `parse`. The backend names functions by the module's last
path segment, so both were `x509.parse`; the call bound to the public one
and returned a `Certificate ! CertErr` where llrt's `Parsed` was expected.
An import alias changes only how the source names the module.

## Fix

Give the llrt module another basename: std/native/llrt/x509.xo is now
x509cert.xo, imported `as x509` by the TLS files so their bodies did not
change. Before adding a std module, check that no std/native/llrt file has
its basename and a function name in common (std/trace and llrt/trace share
a basename but no function names). A compiler fix would mangle by the full
module path.
