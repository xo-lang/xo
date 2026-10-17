# A function's cached text must not depend on its module

Status: Observed (2026-10-17)

## Symptom

`XO_BACKEND_CACHE=verify xo asm --backend=arm64 --targets=linux/arm64 .`
failed ("3 functions differ from their cache entries, the first
pool.fiber_new") after the same xo had built other programs; the cached
text had `.weak __errno_location` lines a fresh no-libc generation does
not write. With the cache off the output was right.

## Cause

The per-function back-end cache (decision 0220 addendum stage B) keys a
function's canonical text by its SSA, its callees' signatures, the types
it reaches, the target, and the compiler. Two module facts reached the
text without being in the key: weak_ref wrote `.weak sym` only when no
function of the module exported sym (a no-libc build's cabi_libc.xo
exports __errno_location), and c_sym_addr addresses a C data symbol
directly in a no-libc build and through the GOT otherwise (m.nolibc). A
libc build's entries were then placed in a no-libc build.

## Fix

weak_ref always writes the directive and drop_own_weak removes the
module's own symbols' from the module's text (gen.xo, parallel.xo), so
the function text no longer reads the module; m.nolibc is in fn_key
(cache version v5). internal/arm64 TestBackendCacheModuleFacts shares
one cache between a libc and a no-libc build, in both orders, and checks
the text against the cache off and with verify. Anything new a
function's text reads from ssa.Module (not its own Func) goes into
fn_key or out of the function's text.
