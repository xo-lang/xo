# Runtime C that the seed still calls cannot be deleted yet

Status: Observed (2026-10-14, B08 of the zero-C audit). Resolved the
same day: seed 0.10.0-23ce8ce2 has B08, and the seed compatibility
block (xo_crypto.c's xo_rand_*, xo_win.c's xo_w_entropy) is deleted.

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

Open seed shims (delete each with the first seed that has its item):
xo_sys.c `xo_static_word` (B02); xo_rt.c `xo_print`, `xo_println`,
`xo_eprint`, `xo_eprintln` (B03 stage 1, 2026-10-14: one line forwards
to cabi.xo's exports, since seed 0.10.0 lowers Stdio.print and the SSA
builder's rt.eprintln to them); xo_std.c `xo_exit` (B03 stage 2, a one
line forward); xo_rt.c `xo_env_get`, `xo_env_vars` and xo_bytes.c
`xo_env_args`, `xo_env_program` (B03 stage 3, whole: the seed lowers
Env.args and the prelude's rt.env_get to them). Before deleting, check
with nm that a program built by the new xo references none of them.
