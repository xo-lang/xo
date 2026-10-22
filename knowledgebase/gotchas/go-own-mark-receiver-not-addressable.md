# An own-marked receiver cannot call a std type's pointer method in Go

Status: Observed (2026-10-22), fixed the same day.

## Symptom

XO_TEST_GO_PARITY=1 TestGoldenGoParity failed for http_client_ip,
embed_dir and embed_dir_fs: the generated Go did not compile, `cannot
call pointer method M_client_ip on rt.Http_Request` and `cannot call
pointer method M_len/M_paths/M_read_text/M_sub on rt.Embed_Dir`.

## Cause

Decision 0335 (817e4b019) made a Map count as holding a list for the Go
backend's ownership marks, so a by-value read of a value whose type holds
a Map is wrapped as `func(v T) T { v.XoMark(); return v }(x)`. A
Request and an Embed.Dir hold Maps, so their receivers got the wrapper;
the call result is not addressable, and std/rt implements these methods
on pointer receivers (`func (r *Http_Request) M_client_ip`). Go calls
a pointer method only on an addressable value. The per-push gates of
0335 ran the Go goldens it touched, not every golden calling a std
method on such a type.

## Fix

compiler/gen call.xo go_recv: a Go method's receiver that is an
own-marked copy goes through `rt.Addr(...)` (std/rt list.go: a pointer
to a copy), which calls pointer and value methods alike and keeps both
the mark and the order of evaluation. Any new wrapper form around a
receiver needs the same: Go method calls want an addressable receiver.
