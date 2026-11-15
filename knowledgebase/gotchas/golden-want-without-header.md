# Native golden want.txt files written as bare stdout

Status: Observed (2026-10-12), fixed for env_program, map_remove_bytes,
and unix_kill.

Symptom: internal/llvm TestGolden (and TestGoldenTasks for unix_kill)
failed on every platform for three golden programs added on 2026-10-12:
the want text was the program's stdout alone, the got text started with
`exit 0` and `--- stdout` and ended with `--- stderr`.

Cause: the files were written by hand instead of with `-golden-update`,
which writes runResult.String(): `exit N`, `--- stdout`, stdout,
`--- stderr`, stable stderr.

Fix: regenerate with `go test -run 'TestGolden$/^NAME$' -golden-update
./internal/llvm` and review the diff (for these three it only added the
header and the stderr marker). Write new native goldens that way.

Hit again (2026-10-12, decision 0193): text_table and str_ops_table were
written from a run of the program (stdout only); TestGolden and
TestGoldenRun failed until `exit 0` and `--- stdout` led the file. Write
a new golden with `-golden-update`, or copy the header of a neighbor.

Hit again (2026-11-15, decision 0644 B1 and B2): eight TLS goldens
(conn_close_write, tls_close_write, tls_conn_state, tls_dial, tls_prefs,
tls_server, tls_ekm, tls_verify) were written from a run of `xo run`
whose stdout matched Go's, and failed TestGoldenRun on every backend.
Comparing stdout with Go's program is the right check for the content;
the file still needs the `exit 0` / `--- stdout` header and the trailing
`--- stderr` line.
