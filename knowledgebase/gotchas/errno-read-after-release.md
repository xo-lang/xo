# errno read after a release of the call's argument

Status: Anticipated (2026-10-13, decision 0213; found in the disassembly,
not in a failure)

Symptom (expected): a platform call from Xo that fails reports the wrong
error, now and then: `open` of a path fails with ENOENT but the message
names another errno, or a Windows CreateFileW failure maps the wrong
GetLastError value.

Cause: reference counting releases a value right after its last use.
In `ret(Int(sys_darwin.open(cpath(path), flags, mode)))` the path's
Bytes is last used by the call, so the arm64 code was `bl _open`, then
the release of the path (the allocator, which may call libSystem or
kernel32), then `bl ___error`. Any call between the failing call and
`__error()` or `GetLastError()` may change the error.

Fix: keep the call's buffers alive until the error is read
(darwin.xo `ret_keep`, windows.xo `keep(w, GetLastError())`); check the
order with `otool -tV` (`bl _open` then `bl ___error` with no call
between).
