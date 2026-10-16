# A C test harness of the runtime faults at run time once its feature's C moved to Xo exports

Status: Obsolete (2026-10-16): the C test harnesses of the runtime are Xo programs (decision 0241); was Anticipated (2026-10-14, B12 of the zero-C audit; avoided, not hit)

Symptom (expected): a C program under std/llrt/testdata that includes
xo_rt.c (http_diff.c, http_fuzz.c, crypto_diff.c) links fine but, at
the first response or signature, faults with "HTTP without std/http's
exports" (or "SHA-512 without std/crypto's exports"), or compares wrong
output if the fault is caught.

Cause: when a feature's C moves to Xo through a feature marker module
(cabi_http.xo, cabi_crypto.xo), every program still links the C that
calls it, so the C has a weak definition that faults. A harness compiled
with cOnly links the exports of an empty program, which does not carry
the feature's exports, so the weak fault stub wins: no link error warns.

Fix: compile the harness with cOnlyWith and a program that uses the
feature (internal/llvm `cOnlyHTTP` for the HTTP harnesses, the
std/crypto program in TestCryptoDiff). Check with `nm` that the binary's
`xo_x_<name>` is the Xo export (a 4 byte branch to `xo_f_cabi__...`),
not the C stub.
