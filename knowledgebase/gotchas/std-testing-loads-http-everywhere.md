# Every native program loads std/http: module loading is not a usage signal

Status: Observed (2026-10-13, decision 0207)

Symptom: the C ABI exports of the TLS runtime (cabi_tls.xo) were
imported by std/native/http.xo so that only programs using std/http
would carry them, yet a hello world built them too (17 tlsname.xo
functions, about 4,000 arm64 instructions).

Cause: the checker loads std/testing for every program (the predeclared
`testing` of test blocks), std/testing imports std/http, and loadNative
loads the implementation module of every std module loaded
(internal/check load.go). So std/native/http.xo, and every module it
imports, is loaded into every native program; the SSA builders only
build what is reachable, which hid this until code was rooted by module.

Fix: root a feature's exports by what the program builds, not by what
it loads: the SSA builders (internal/ssa declareExports, compiler/ssa
declare_exports) build cabi_<feature> only when a function of a module
that imports it was declared, after the program's own code is built.
Use the same rule for anything else that should follow a feature.
