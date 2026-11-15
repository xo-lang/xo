# An engine address read from a handle outlives the handle

Status: Observed (2026-11-15, decision 0644 T12, std/native/tls.xo
`TlsConn.hh`; diagnosed by Lang 1, ~/xo-lead/notes/lang1/hret/diagnosis.md)

## Symptom

Every TLS connection crashed (SIGSEGV, native and LLVM alike) after
`TlsConn`'s methods read their handle through a new helper
`fn (t: TlsConn) hh() -> rthttp.TlsH? { t.h }`. The backtrace runs
`obj.rl_share` < `tlsio.save` < `tlsio.close_notify` <
`TlsConn.close`. Rebuilding the Option with a match "fixed" it, which
looked like a missing retain in the compiler. It is not a miscompile:
every SSA pass off and the backend cache off still crash, and the RC
stage verifies.

## Cause

`{ t.h }` gives the receiver's field away, so `hh`'s receiver becomes
owned, and `close` (whose caller drops `t`) moves its last references
into it. `tlsio.close_notify(tlsio.handle_conn(x))` then reads the
engine's address (an Int) from the handle, the handle's last use, so the
RC stage releases it right there, its free hook frees the engine, and
`close_notify` runs on freed memory. The match form only kept the
receiver borrowed, so `t.h` happened to keep the handle alive. Same class
as obj-addr-handle-released-early.md: an Int taken from a handle carries
no reference.

## Fix

Never carry `tlsio.handle_conn(x)`'s Int past `x`'s last use. tlsio's
accessors std/tls calls take the handle (`close_notify_h`, `alpn_h`,
`peer_ders_h`, `negotiated_h`, `version_h`), and `read` and `write` pass
the handle as a parameter to `read_on` and `write_on`, which the caller
holds until the call returns. With that, `hh` returns `t.h` again and
tls_resume passes on arm64.

A vet or checker rule for this class (an Int read from a handle with
`rt.obj_addr` or `mem_load` and used after the handle's last use) is
proposed to Lang 1 (~/xo-lead/notes/lang3/handle-addr-vet.md).
