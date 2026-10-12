# xo1's copies of internal/build tables drift without a failing test

Status: Anticipated (found by reading, 2026-10-12, decision 0178).

## Symptom

xo1 (`selfhost/xo`) keeps its own copy of tables internal/build owns.
`io_flags` (link.xo), the symbols that turn on `-DXO_FS`, still had the
list from before decision 0172: `xo_fs_open`, `xo_fs_file_`,
`xo_fs_copy`, `xo_fs_perm`, `xo_fs_chmod`, `xo_fs_symlink`,
`xo_fs_read_link`, `xo_fs_lock`, `xo_fs_unlock` were missing. A program
whose only file operations are handles, copies, modes, links, or locks
would be compiled by xo1 without XO_FS, so the runtime's file code is
left out and the link fails (or a pack variant without it is chosen).
No golden caught it: every program using these also calls `xo_fs_stat`
or another symbol of the old list (internal/gen/testdata/golden/fs_files
builds and runs under xo1 with arm64 and LLVM either way).

## Cause

Decision 0174 duplicates internal/build's keys and tables in Xo and
checks the keys (`xo1 keys`, selfhost-samebin.py on the goldens); a
table that changes only the C flags for programs outside the goldens
is not covered by either.

## Fix

The list is in step again (decision 0178). Until the Go driver is
retired: a change to `build.IOFlags`, `tlsSymbols`, mbedtls.go's
configuration, or a runtime flag list must touch selfhost/xo in the
same change; grep selfhost/xo for the Go function's name, which each
port names in its doc comment. `TestXo1IOFlagsSymbols`
(internal/selfhost/xo1_tables_test.go, runs with -short) now compares
the symbol lists of `IOFlags` and `io_flags`; the other tables
(`tlsSymbols`, the rtpack flag variants) have no such guard yet.
