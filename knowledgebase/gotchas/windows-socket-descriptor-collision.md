# Windows sockets and C runtime descriptors share numbers

Status: Observed (2026-10-12), fixed in std/llrt/xo_win.c (XO_W_FD).

Symptom: a native program on Windows with a few loopback connections
opened and closed about 550 or more files; afterwards some connections
failed with `write tcp: not a socket` (0 of 8 working after 1000 files),
and removing the files' directory failed with `Denied` (some files were
never closed).

Cause: the runtime numbers both kinds as ints. A socket is its SOCKET
handle's value (a multiple of 4: 572 to 676 in that run) and a C runtime
descriptor counts up from 3, so past a few hundred open files the same
number named a live file and a live socket. `close` tried `closesocket`
first: closing the file closed the socket, and the file stayed open. A
set of open sockets consulted by `close` does not help, because the
number is both at once.

Fix: C runtime descriptors above 2 carry a tag bit (0x40000000) from the
shims that make them (`open`, `pipe`, `_open_osfhandle` in xo_proc.c),
and every shim that hands one to the C runtime takes it off (`read`,
`write`, `close`, `fstat`, `flock`, `_get_osfhandle`, `_fdopen`). New
code that gets a descriptor from the C runtime directly must tag it with
`xo_w_fd`.
