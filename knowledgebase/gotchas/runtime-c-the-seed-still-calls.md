# Runtime C that the seed still calls cannot be deleted yet

Status: Observed (2026-10-14, B08 of the zero-C audit). Every shim
deleted with seed 0.11.0 (below). Resolved the same day for B08: seed 0.10.0-23ce8ce2 has B08, and the seed compatibility
block (xo_crypto.c's xo_rand_*, xo_win.c's xo_w_entropy) is deleted.
Applies again to a new intrinsic: decision 0222's `rt.ct_barrier`,
which seed 0.10.0-23ce8ce2 lowers as a call of `xo_ct_barrier`, keeps
that function in xo_crypto.c until a seed has 0222: selfhost/xo
reaches Ed25519 through sumdb's ed25519_verify, and the seed built it
(2026-10-14, 37 s and 44 s on the Mac) with that function present.
Again for zero-C audit B13 (2026-10-14): the stack trace's padding
moved from `rt.str_rune_count` to the native prelude's str_rune_count,
so xo_std.c's `xo_str_rune_count` stays, labeled, until a seed has B13.

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

Open seed shims: none. Seed 0.11.0-ae2587fd (2026-10-14) has B02, B03,
B09, and B13, and the shims were deleted that day: xo_sys.c
`xo_static_word`, xo_crypto.c (`xo_ct_barrier`, the whole file),
xo_rt.c `xo_println`, `xo_eprint`, `xo_eprintln`, `xo_env_get`,
`xo_env_vars`, xo_bytes.c `xo_env_args`, `xo_env_program`, xo_std.c
`xo_exit`, `xo_str_rune_count`, with what only they used (cabi.xo's
print, println, eprint, eprintln, and exit exports; the test world's C
tw_write and tw_env_* forwarders and cabi_tw.xo's exports for them).
Checked by building selfhost/xo with the 0.11.0 seed itself
(`XO_SEED=<unpacked seed>/xo/bin/xo go run ./scripts/xobuild`, which
would fail to link on any call it still makes) and with nm on the
result.

`xo_print` was not only a shim: building with the seed found the native
prelude's stdio_write (`Stdio.write`) still calling `rt.print`, the C
`xo_print`, which B03 had missed. It now writes through stdio.xo's out,
and rt.print is gone. A shim's removal is checked by building with the
seed, not by reading the compiler: the std's own calls of `rt.<name>`
count too.
