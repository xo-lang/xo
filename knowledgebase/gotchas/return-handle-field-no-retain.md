# Returning a native handle's Option field as is frees the handle

Status: Observed (2026-11-15, decision 0644 T12, std/native/tls.xo
`TlsConn.hh`); cause in the compiler not yet found

## Symptom

Every TLS connection crashed (SIGSEGV, native and LLVM alike) after
`TlsConn`'s methods read their handle through a new helper
`fn (t: TlsConn) hh() -> rthttp.TlsH? { t.h }` in place of `t.h`. lldb
stops in `xo_f_share_20List_5bInt_5d`; `xo dev symbolize` names
`obj.rl_share` (a list being marked shared holds a freed pointer).
Goldens that never read the handle that way (tls_close_write) passed.

## Cause

Not confirmed. `rthttp.TlsH` is an empty struct the native code makes
with `tasks.make_handle` and a free hook (tlsio.handle_new). Returning
the field `t.h` of the borrowed receiver unchanged gives the caller a
reference it later releases without the matching retain, so the handle
(and the engine it holds) is freed under the live connection. The same
shape with a `Mutex[List[Int]]?` field in user code does not crash.
Bisect (scratch copy of std, one change at a time): HEAD plus every new
function passes; replacing the nine `t.h` reads with `t.hh()` crashes;
the same `hh` rebuilt as `match t.h { Some(x) => Some(x) None => None }`
passes.

## Fix

Rebuild the Option from a match when returning a handle field (tls.xo
`hh`, and the same in a `Mutex.with` closure). For the compiler owner: a
repro is the T12 tree of 2026-11-15 with `hh` as `{ t.h }`, run
`internal/llvm/testdata/golden/tls_resume` on arm64.
