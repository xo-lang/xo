# A runtime symbol named xo_tls_* turns on the TLS library

Status: Observed (2026-10-13, decision 0212) Obsolete since 2026-10-15: Mbed TLS removed (decision 0202).

Symptom: after adding thread local intrinsics named `rt.tls_get` and
`rt.tls_set` (C symbols `xo_tls_get`, `xo_tls_set`), the C only runtime
tests (TestRuntimeTasks, TestRuntimeEventLoop) failed to link with
undefined `xo_x_pem_next`, `xo_x_tls_is_ip`, and other exports of
std/native/llrt/cabi_tls.xo, although no program used std/http.

Cause: internal/build's `needsTLS` (and selfhost/xo's copy) decides
that a link needs Mbed TLS, and compiles the runtime with `-DXO_TLS`,
when the generated code names a symbol starting with `xo_tls_` (the
TLS functions of std/http). The empty program's IR named `xo_tls_get`,
so xo_tls.c's real code was compiled, and it calls cabi_tls exports that
only programs importing std/http build.

Fix: the thread local intrinsics are `rt.tlocal_get`, `rt.tlocal_set`,
and `rt.tlocal_addr` (`xo_tlocal_*`, the table `xo_tlocal`). Any new
runtime symbol must avoid the prefixes in `tlsSymbols`
(internal/build/tls.go: `xo_cl_do`, `xo_srv_new`, `xo_tls_`).
