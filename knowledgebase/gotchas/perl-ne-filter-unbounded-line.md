# A perl -ne output filter grows with the longest line

Status: Observed (2026-11-18, alert 56402, #390)

Symptom: offload.sh's output filter grew to a 20 GB phys_footprint on the
Mac during a long amd64 job (Lang 1's s2c-356).

Cause: `perl -ne` (like `while (<>)`) reads a whole line before running the
body. A job that prints a long line with no newline, or a progress stream
of `\r` with no `\n`, keeps that one line in memory, unbounded. The filter
stripped `\r` only after reading the line, so carriage returns did not end
it either.

Fix: read fixed chunks (`sysread(STDIN, $c, 65536)`), act on each complete
line, and pass a buffer longer than 64 KiB in pieces (offload.sh
`off_filter_pl`). Any line filter in a long-running pipe should bound its
buffer the same way. Test: scripts/lead/offload_filter_test.sh (200 MB with
no newline stays at 4 MB resident; the old filter held 196 MB).
