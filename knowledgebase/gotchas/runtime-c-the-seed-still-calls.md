# Runtime C that the seed still calls cannot be deleted yet

Status: Observed (2026-10-14, B08 of the zero-C audit). Resolved the
same day: seed 0.10.0-23ce8ce2 has B08, and the seed compatibility
block (xo_crypto.c's xo_rand_*, xo_win.c's xo_w_entropy) is deleted.
Applies again to a new intrinsic: decision 0222's `rt.ct_barrier`,
which seed 0.10.0-23ce8ce2 lowers as a call of `xo_ct_barrier`, keeps
that function in xo_crypto.c until a seed has 0222: selfhost/xo
reaches Ed25519 through sumdb's ed25519_verify, and the seed built it
(2026-10-14, 37 s and 44 s on the Mac) with that function present.

Symptom: after Rand moved to std/native/llrt/rand.xo (the SSA builders
call the native prelude's llrt_rand_* instead of rt.rand_*) and
xo_crypto.c's xo_rand_int, xo_rand_bytes, xo_rand_float, and
xo_rand_bool were deleted, `go run ./scripts/xobuild` failed: every
seed (the prebuilt 0.9.0-5e21dc31 and the newest cached build) linked
the new selfhost/xo with `Undefined symbols: _xo_rand_bytes,
_xo_rand_int` (selfhost/xo calls `os.rand.bytes`).

Cause: the seed is an older compiler, but it builds the checkout's
compiler against the checkout's std and runtime C (decision 0216). Its
SSA builder still lowers a runtime operation to the C function it knew,
so a C entry point that only a newer compiler stops calling is still
needed by the build of that compiler.

Fix: when a port changes what the compiler emits for an operation (a
call of the native prelude in place of an `rt.<name>` C function), keep
the C function, labeled as seed compatibility, until the seed is a
release with the change; delete it then. A port that keeps the
operation's lowering (C that only other C calls, such as SHA-512 for
Ed25519) can delete the C at once. Check with `go run
./scripts/xobuild` (it builds with the seed) before deleting runtime
C that generated code calls.
