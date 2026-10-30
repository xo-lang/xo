# Concurrent installs of the TLS/h2 ops table are a data race

Status: Observed (2026-10-30)

## Symptom

TSan TestGoldenTasks/http_h2c fails (3 of 3) with "data race: write of
size 8 by thread T9 in x.http.Server.serve, previous write by T8 in the
same function" (later in with_protocols, after the install moved there).
It failed before and after the Server.post change; only the frame names
changed.

## Cause

Each task of the golden builds its own server; every h2c or TLS server
calls srvh2c.install_h2c (or srvtls.install), which stored its entries
into the one shared table (hconn.tls_ops_table) with plain stores. The
stores wrote the same values, but two tasks writing the same words at once
is a race for TSan, and a task that lost the creation of the table (tls_ops_set)
could use the table before the winner filled it.

## Fix

The fills are guarded by once flags in the table itself
(hconn.TO_ONCE_H2, TO_ONCE_TLS): the first caller of srvh2c.begin_fill
stores the entries and calls end_fill; the others wait for the flag
and store nothing. A new group of entries gets its own flag and the same
begin/end pair.
