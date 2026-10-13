# grep finds nothing in a heavy.sh log that holds a NUL byte

Status: Observed (2026-10-13)

Symptom: after a golden run failed, `grep -n "FAIL" agents/<name>-cmd.log`
printed nothing, though the log had `--- FAIL` lines.

Cause: the failing golden (show_table) prints the Rune U+0000, so the
log held a NUL byte; grep then treats the file as binary and prints
nothing for matching lines unless it is told otherwise.

Fix: `grep -a` (or `--text`) on logs of test runs.
