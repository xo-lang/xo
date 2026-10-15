# cabi_tls.xo's exports cannot call framework functions directly

Status: Observed (2026-10-14, B30)

Symptom: after std/native/llrt/tlscert.xo declared CoreFoundation and
Security functions in sys_darwin.xo, programs that import std/http but
use neither the client nor the server (TestGoldenSplit/request_eq) failed
to link on macOS: `"_SecPolicyCreateSSL", referenced from:
_xo_x_tls_apple_verify ... ld: symbol(s) not found`.

Cause: every top level function of a cabi module is a C ABI export, so
cabi_tls.xo's exports and everything they call are in every program that
imports std/http. The xo build links Mbed TLS and the Security and
CoreFoundation frameworks only when the generated code names the
client, the server, or an `xo_tls_` symbol (selfhost/xo/tls.xo
needs_tls); the `xo_x_tls_` exports do not count.

Fix: reach a library that only TLS programs link through addresses the
C fills in (xo_tls.c's tls_svc, compiled only with -DXO_TLS), and call
them with rt.call_c2 or the record's call4 trampoline (tlscert.xo sec).
sys_darwin.xo and sys_windows.xo declarations are for libSystem and the
DLLs every program links (crypt32 is one: xo_win.c's pragma).
